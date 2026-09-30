package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/builtin"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/support"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

func supportPolicy(db *gorm.DB, org uuid.UUID) (models.SupportPolicy, error) {
	var p models.SupportPolicy
	err := db.Where("organization_id = ?", org).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return p, nil
	}
	return p, err
}

func supportLock(tx *gorm.DB, key string) error {
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", key).Error
}

// Recover the gap if the process stopped after saving an inbound message but
// before its job update. Source timestamps are persisted with the message.
func (a *App) reconcileSupportIncoming() {
	var messages []models.Message
	err := a.DB.Raw(`SELECT m.* FROM messages m JOIN support_policies p ON p.organization_id=m.organization_id AND p.enabled=true
 LEFT JOIN support_jobs j ON j.organization_id=m.organization_id AND j.account=m.whats_app_account AND j.contact_id=m.contact_id
 LEFT JOIN messages previous ON previous.id=j.last_message_id
 WHERE m.direction='incoming' AND m.source_at IS NOT NULL AND m.created_at>=p.enabled_at AND m.source_at>? AND m.deleted_at IS NULL
 AND (previous.id IS NULL OR m.created_at>previous.created_at)
 ORDER BY m.created_at ASC LIMIT 100`, time.Now().Add(-24*time.Hour)).Scan(&messages).Error
	if err != nil {
		a.Log.Error("Support queue reconciliation failed", "error", err)
		return
	}
	for i := range messages {
		m := &messages[i]
		var account models.WhatsAppAccount
		var contact models.Contact
		if a.DB.Where("organization_id=? AND name=?", m.OrganizationID, m.WhatsAppAccount).First(&account).Error != nil {
			continue
		}
		if a.DB.First(&contact, "id=?", m.ContactID).Error != nil {
			continue
		}
		a.captureSupportIncoming(&account, &contact, m, strconv.FormatInt(m.SourceAt.Unix(), 10))
	}
}

// captureSupportIncoming returns true whenever consolidated mode owns this
// message, including failures: a database error must not fall through to legacy replies.
func (a *App) captureSupportIncoming(account *models.WhatsAppAccount, contact *models.Contact, message *models.Message, timestamp string) bool {
	policy, err := supportPolicy(a.DB, account.OrganizationID)
	if err != nil {
		a.Log.Error("Support policy lookup failed", "error", err)
		return true
	}
	if !policy.Enabled {
		return false
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	now := time.Now()
	if err != nil || seconds <= 0 || time.Unix(seconds, 0).After(now.Add(time.Minute)) {
		a.Log.Error("Support inbound timestamp invalid", "message_id", message.ID)
		return true
	}
	source := time.Unix(seconds, 0)
	if source.After(now) {
		source = now
	}
	if err := a.DB.Model(message).Update("source_at", source).Error; err != nil {
		return true
	}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		if err := supportLock(tx, "support-customer:"+account.OrganizationID.String()+contact.ID.String()); err != nil {
			return err
		}
		var job models.SupportJob
		err := tx.Where("organization_id = ? AND account = ? AND contact_id = ?", account.OrganizationID, account.Name, contact.ID).First(&job).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if job.ID == uuid.Nil {
			job = models.SupportJob{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: account.OrganizationID, Account: account.Name, ContactID: contact.ID, PendingFrom: message.CreatedAt}
		}
		if job.LastMessageID == message.ID {
			return nil
		}
		// Reopen terminal work only for newly arriving messages; never replay a backlog on rollover.
		if (job.State == "sent" || job.State == "suppressed" || job.State == "expired" || job.State == "skipped") && message.CreatedAt.After(job.UpdatedAt) {
			job.PendingFrom = message.CreatedAt
		}
		if source.After(job.LastInboundAt) {
			job.LastInboundAt = source
		}
		job.LastMessageID = message.ID
		job.Revision++
		job.State = "pending"
		job.Answer = ""
		job.Decision = ""
		job.Attempts = 0
		job.LeaseUntil = nil
		job.RetryAt = now.Add(time.Minute)
		due, err := support.DueAt(now, job.LastInboundAt)
		if err != nil {
			job.State = "expired"
			job.Reason = "Customer service window closed"
			due = now
		}
		if due.Before(policy.FirstRunAt) {
			due = policy.FirstRunAt
		}
		// Expiry protection still applies during initial collection.
		safe := job.LastInboundAt.Add(24*time.Hour - support.ExpiryMargin)
		if safe.Before(due) && safe.After(now) {
			due = safe
		}
		job.DueAt = due
		return tx.Omit("Contact").Save(&job).Error
	})
	if err != nil {
		a.Log.Error("Support enqueue failed", "message_id", message.ID, "error", err)
	}
	return true
}

func customerStart(now time.Time) time.Time {
	p := now.In(support.PKT)
	return time.Date(p.Year(), p.Month(), 1, 0, 0, 0, 0, support.PKT)
}

func billingStart(now time.Time, zone string) (time.Time, error) {
	if zone == "" {
		zone = "Asia/Karachi"
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, err
	}
	p := now.In(loc)
	return time.Date(p.Year(), p.Month(), 1, 0, 0, 0, 0, loc), nil
}

// budgetUsage includes historical sends and all reservations, without double
// counting linked messages. Unknown outcomes stay charged conservatively.
func budgetUsage(db *gorm.DB, _ uuid.UUID, phone string, start time.Time) (int64, error) {
	var count int64
	err := db.Raw(`SELECT count(*) FROM (
 SELECT m.id FROM messages m JOIN whatsapp_accounts a ON a.organization_id=m.organization_id AND a.name=m.whats_app_account
 WHERE a.phone_id=? AND m.direction='outgoing' AND m.created_at>=? AND m.status <> 'failed'
 UNION SELECT message_id FROM support_sends WHERE phone_id=? AND created_at>=? AND state <> 'rejected'
 ) sends`, phone, start, phone, start).Scan(&count).Error
	return count, err
}

func customerSuppressed(db *gorm.DB, org, contact uuid.UUID, now time.Time) (bool, error) {
	var count int64
	err := db.Unscoped().Model(&models.Message{}).Where("organization_id=? AND contact_id=? AND direction='outgoing' AND created_at>=? AND status <> 'failed'", org, contact, customerStart(now)).Count(&count).Error
	if err != nil || count > 0 {
		return count > 0, err
	}
	err = db.Model(&models.SupportSend{}).Where("organization_id=? AND contact_id=? AND customer_month=? AND state <> 'rejected'", org, contact, support.CustomerMonth(now)).Count(&count).Error
	return count > 0, err
}

// reserveSupportSend atomically creates the message and its reservation. The
// caller cannot bypass this using the public API; automatic job fields are private.
func (a *App) reserveSupportSend(req OutgoingMessageRequest, opts MessageSendOptions, msg *models.Message) (bool, error) {
	policy, err := supportPolicy(a.DB, req.Account.OrganizationID)
	if err != nil {
		return true, err
	}
	if !policy.Enabled {
		return false, nil
	}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		// All guarded sends take locks in the same order.
		if err := supportLock(tx, "support-number:"+req.Account.PhoneID); err != nil {
			return err
		}
		if err := supportLock(tx, "support-customer:"+req.Account.OrganizationID.String()+req.Contact.ID.String()); err != nil {
			return err
		}
		now := time.Now()
		policy, err := supportPolicy(tx, req.Account.OrganizationID)
		if err != nil {
			return err
		}
		if !policy.Enabled {
			return errors.New("support policy changed; retry send")
		}
		if req.Type == models.MessageTypeTemplate || req.Type == models.MessageTypeFlow {
			return errors.New("templates and flows are disabled in consolidated support mode")
		}
		automatic := opts.supportJobID != uuid.Nil
		if !automatic && opts.SentByUserID == nil {
			return errors.New("legacy automation is disabled in consolidated support mode")
		}
		var job models.SupportJob
		err = tx.Where("organization_id=? AND account=? AND contact_id=?", req.Account.OrganizationID, req.Account.Name, req.Contact.ID).First(&job).Error
		if err != nil {
			if !automatic && errors.Is(err, gorm.ErrRecordNotFound) {
				var inbound models.Message
				if err := tx.Where("organization_id=? AND contact_id=? AND whats_app_account=? AND direction='incoming'", req.Account.OrganizationID, req.Contact.ID, req.Account.Name).Order("created_at DESC").First(&inbound).Error; err != nil {
					return errors.New("no customer service window")
				}
				job.LastInboundAt = inbound.CreatedAt
				if inbound.SourceAt != nil {
					job.LastInboundAt = *inbound.SourceAt
				}
			} else {
				return errors.New("no verified inbound support window for this conversation")
			}
		}
		if !now.Before(job.LastInboundAt.Add(24 * time.Hour)) {
			return support.ErrExpired
		}
		if automatic {
			if policy.Paused {
				return errors.New("support automation is paused")
			}
			if job.ID != opts.supportJobID || job.Revision != opts.supportRevision || job.State != "ready" || now.Before(job.DueAt) {
				return errors.New("support job changed or is not due")
			}
			var contact models.Contact
			if err := tx.First(&contact, "id=?", req.Contact.ID).Error; err != nil {
				return err
			}
			var transfers int64
			if err := tx.Model(&models.AgentTransfer{}).Where("organization_id=? AND contact_id=? AND status=?", req.Account.OrganizationID, contact.ID, models.TransferStatusActive).Count(&transfers).Error; err != nil {
				return err
			}
			if contact.AssignedUserID != nil || transfers > 0 {
				return errors.New("conversation is assigned to a human")
			}
			blocked, err := customerSuppressed(tx, req.Account.OrganizationID, req.Contact.ID, now)
			if err != nil {
				return err
			}
			if blocked {
				return errors.New("customer already replied to this month")
			}
		} else {
			var pending int64
			if err := tx.Model(&models.SupportSend{}).Where("organization_id=? AND contact_id=? AND state='reserved'", req.Account.OrganizationID, req.Contact.ID).Count(&pending).Error; err != nil {
				return err
			}
			if pending > 0 {
				return errors.New("a send is already in progress; wait for its outcome")
			}
		}
		start, err := billingStart(now, policy.BillingTimezone)
		if err != nil {
			return err
		}
		used, err := budgetUsage(tx, req.Account.OrganizationID, req.Account.PhoneID, start)
		if err != nil {
			return err
		}
		if used >= support.MonthlyLimit {
			return errors.New("monthly support limit reached; sending is paused until next month")
		}
		if err := tx.Create(msg).Error; err != nil {
			return err
		}
		reservation := models.SupportSend{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: req.Account.OrganizationID, PhoneID: req.Account.PhoneID, ContactID: req.Contact.ID, MessageID: msg.ID, CustomerMonth: support.CustomerMonth(now), Automatic: automatic, State: "reserved"}
		if automatic {
			reservation.JobID = &job.ID
			if err := tx.Model(&job).Update("state", "sending").Error; err != nil {
				return err
			}
		}
		return tx.Create(&reservation).Error
	})
	return true, err
}

func (a *App) finishSupportSend(msg *models.Message, wamid string, sendErr error) {
	var reservation models.SupportSend
	if err := a.DB.Where("message_id=?", msg.ID).First(&reservation).Error; err != nil {
		return
	}
	state := "accepted"
	jobState := "sent"
	reason := "WhatsApp accepted the reply"
	if sendErr != nil {
		state = "uncertain"
		jobState = "uncertain"
		reason = "Send outcome uncertain; no automatic retry"
		var rejection *whatsapp.RequestError
		if errors.As(sendErr, &rejection) && rejection.StatusCode >= 400 && rejection.StatusCode < 500 && rejection.StatusCode != 408 {
			state = "rejected"
			jobState = "error"
			reason = "WhatsApp rejected the send; automatic retry is bounded"
		}
	}
	if err := a.DB.Transaction(func(tx *gorm.DB) error {
		if err := supportLock(tx, "support-customer:"+reservation.OrganizationID.String()+reservation.ContactID.String()); err != nil {
			return err
		}
		if err := tx.First(&reservation, "id=?", reservation.ID).Error; err != nil {
			return err
		}
		if reservation.State == "rejected" {
			return nil
		}
		if err := tx.Model(&reservation).Updates(map[string]any{"state": state, "wamid": wamid}).Error; err != nil {
			return err
		}
		if !reservation.Automatic && state != "rejected" {
			jobState = "suppressed"
			reason = "Manual reply holds automation for this month"
		}
		updates := map[string]any{"state": jobState, "reason": reason, "lease_until": nil}
		if !reservation.Automatic && state != "rejected" {
			updates["revision"] = gorm.Expr("revision+1")
		}
		if state == "rejected" {
			updates["retry_at"] = time.Now().Add(2 * time.Minute)
			updates["attempts"] = gorm.Expr("attempts+1")
		}
		return tx.Model(&models.SupportJob{}).Where("organization_id=? AND contact_id=?", reservation.OrganizationID, reservation.ContactID).Updates(updates).Error
	}); err != nil {
		a.Log.Error("Support send reconciliation failed", "message_id", msg.ID, "error", err)
	}
}

// RunSupportWorker uses existing PostgreSQL and exits with the server context.
func (a *App) RunSupportWorker(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.supportTick(ctx)
		}
	}
}

func (a *App) supportTick(ctx context.Context) {
	now := time.Now()
	a.reconcileSupportIncoming()
	// A process crash after reservation is an ambiguous send, never a retry.
	a.DB.Model(&models.SupportSend{}).Where("state='reserved' AND created_at<?", now.Add(-3*time.Minute)).Update("state", "uncertain")
	a.DB.Model(&models.SupportJob{}).Where("state='sending' AND updated_at<?", now.Add(-3*time.Minute)).Updates(map[string]any{"state": "uncertain", "reason": "Worker stopped during dispatch; no duplicate retry"})
	var jobs []models.SupportJob
	err := a.DB.Where("state IN ? AND retry_at<=? AND (lease_until IS NULL OR lease_until<?)", []string{"pending", "preparing", "ready", "error", "budget_paused"}, now, now).Order("due_at ASC").Limit(50).Find(&jobs).Error
	if err != nil {
		a.Log.Error("Support queue read failed", "error", err)
		return
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		policy, err := supportPolicy(a.DB, job.OrganizationID)
		if err != nil || !policy.Enabled {
			continue
		}
		lease := time.Now().Add(2 * time.Minute)
		claim := a.DB.Model(&models.SupportJob{}).Where("id=? AND revision=? AND (lease_until IS NULL OR lease_until<?)", job.ID, job.Revision, time.Now()).Update("lease_until", lease)
		if claim.Error != nil || claim.RowsAffected != 1 {
			continue
		}
		a.processSupportJob(ctx, policy, job)
	}
}

func (a *App) supportJobUpdate(job models.SupportJob, updates map[string]any) {
	updates["lease_until"] = nil
	if err := a.DB.Model(&models.SupportJob{}).Where("id=? AND revision=?", job.ID, job.Revision).Updates(updates).Error; err != nil {
		a.Log.Error("Support queue update failed", "error", err)
	}
}

func (a *App) processSupportJob(ctx context.Context, policy models.SupportPolicy, job models.SupportJob) {
	now := time.Now()
	if !now.Before(job.LastInboundAt.Add(24 * time.Hour)) {
		a.supportJobUpdate(job, map[string]any{"state": "expired", "reason": "Service window closed"})
		return
	}
	eligibilityTime := now
	if job.DueAt.After(now) {
		eligibilityTime = job.DueAt
	}
	blocked, err := customerSuppressed(a.DB, job.OrganizationID, job.ContactID, eligibilityTime)
	if err != nil {
		a.supportJobUpdate(job, map[string]any{"state": "error", "retry_at": now.Add(time.Minute), "reason": "Eligibility lookup failed"})
		return
	}
	if blocked {
		a.supportJobUpdate(job, map[string]any{"state": "suppressed", "reason": "Customer already replied to or send outcome held this month"})
		return
	}
	var contact models.Contact
	if err := a.DB.First(&contact, "id=?", job.ContactID).Error; err != nil {
		return
	}
	if contact.AssignedUserID != nil || a.hasActiveAgentTransfer(job.OrganizationID, job.ContactID) {
		a.supportJobUpdate(job, map[string]any{"state": "suppressed", "reason": "Conversation assigned to a human"})
		return
	}
	var account models.WhatsAppAccount
	if err := a.DB.Where("organization_id=? AND name=?", job.OrganizationID, job.Account).First(&account).Error; err != nil {
		return
	}
	account.DecryptSecrets(a.Config.App.EncryptionKey)
	settings, err := a.getChatbotSettingsCached(job.OrganizationID, job.Account)
	if err != nil || !settings.IsEnabled || !settings.AI.Enabled {
		a.supportJobUpdate(job, map[string]any{"state": "error", "reason": "Chatbot or AI is disabled", "retry_at": now.Add(time.Minute)})
		return
	}
	if job.Attempts >= 3 {
		a.supportJobUpdate(job, map[string]any{"state": "skipped", "reason": "Stopped after three failed attempts"})
		return
	}
	if job.Answer == "" {
		if job.Attempts >= 3 {
			a.supportJobUpdate(job, map[string]any{"state": "skipped", "reason": "Classification unavailable after three attempts; no unverified reply sent"})
			return
		}
		transcript, err := a.supportTranscript(job)
		if err != nil {
			a.supportJobUpdate(job, map[string]any{"state": "error", "reason": err.Error(), "attempts": job.Attempts + 1, "retry_at": now.Add(2 * time.Minute)})
			return
		}
		prepCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
		prepared, err := support.Prepare(prepCtx, transcript, a.supportGenerator(settings))
		cancel()
		if err != nil {
			a.supportJobUpdate(job, map[string]any{"state": "error", "reason": "AI preparation failed; automatic retry scheduled", "attempts": job.Attempts + 1, "retry_at": now.Add(2 * time.Minute)})
			a.Log.Error("Support preparation failed", "job_id", job.ID, "error", err)
			return
		}
		decision, _ := json.Marshal(prepared.Decision)
		state := "ready"
		if prepared.Decision.Decision == "skip" {
			state = "skipped"
		}
		body := prepared.Body
		if body != "" {
			body += "\nhttps://wa.recubetech.com/support/email"
		}
		a.supportJobUpdate(job, map[string]any{"state": state, "decision": string(decision), "answer": body, "version": prepared.Version, "reason": prepared.Reason, "retry_at": now})
		return // The next tick rechecks revision and manual takeover before dispatch.
	}
	if policy.Paused {
		a.supportJobUpdate(job, map[string]any{"retry_at": now.Add(time.Minute)})
		return
	}
	if now.Before(job.DueAt) {
		a.supportJobUpdate(job, map[string]any{"retry_at": job.DueAt})
		return
	}
	start, err := billingStart(now, policy.BillingTimezone)
	if err != nil {
		return
	}
	used, err := budgetUsage(a.DB, job.OrganizationID, account.PhoneID, start)
	if err != nil {
		return
	}
	if used >= support.MonthlyLimit {
		a.supportJobUpdate(job, map[string]any{"state": "budget_paused", "reason": "Monthly number limit reached", "retry_at": start.AddDate(0, 1, 0)})
		return
	}
	if job.State != "ready" {
		a.supportJobUpdate(job, map[string]any{"state": "ready", "retry_at": now})
		return
	}
	opts := ChatbotSendOptions()
	opts.supportJobID = job.ID
	opts.supportRevision = job.Revision
	req := OutgoingMessageRequest{Account: &account, Contact: &contact, Type: models.MessageTypeText, Content: job.Answer}
	if utf8.RuneCountInString(job.Answer) <= 1024 {
		req.Type = models.MessageTypeInteractive
		req.InteractiveType = "cta_url"
		req.BodyText = job.Answer
		req.ButtonText = "Email support"
		req.URL = "https://wa.recubetech.com/support/email"
	}
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := a.SendOutgoingMessage(sendCtx, req, opts); err != nil {
		a.DB.Model(&models.SupportJob{}).Where("id=? AND revision=? AND state='ready'", job.ID, job.Revision).Updates(map[string]any{"state": "error", "reason": err.Error(), "retry_at": now.Add(time.Minute), "lease_until": nil})
	}
}

func (a *App) supportTranscript(job models.SupportJob) ([]support.Message, error) {
	var messages []models.Message
	if err := a.DB.Where("organization_id=? AND contact_id=? AND whats_app_account=? AND created_at>=?", job.OrganizationID, job.ContactID, job.Account, job.PendingFrom).Order("created_at ASC, id ASC").Limit(201).Find(&messages).Error; err != nil {
		return nil, err
	}
	if len(messages) > 200 {
		return nil, errors.New("Conversation exceeds preparation size; email fallback required")
	}
	transcript := make([]support.Message, 0, len(messages))
	for _, m := range messages {
		media := ""
		if m.MessageType != models.MessageTypeText {
			media = string(m.MessageType)
		}
		transcript = append(transcript, support.Message{ID: m.ID.String(), Direction: string(m.Direction), Text: m.Content, Media: media, Pending: m.Direction == models.DirectionIncoming})
	}
	return transcript, nil
}

// supportGenerator uses the existing encrypted provider configuration, a bounded
// HTTP request and no legacy session history. Provider errors never include keys.
func (a *App) supportGenerator(settings *models.ChatbotSettings) support.Generate {
	return func(ctx context.Context, system, input string) (string, error) {
		if settings.AI.Provider != models.AIProviderOpenAI {
			return "", errors.New("consolidated support currently requires the configured OpenAI provider")
		}
		payload := map[string]any{"model": settings.AI.Model, "max_tokens": 4000, "temperature": 0.2, "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": input}}}
		if system == builtin.InvestifyQueuePrompt {
			payload["response_format"] = map[string]any{"type": "json_object"}
		}
		raw, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(raw))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+settings.AI.APIKey)
		resp, err := a.HTTPClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("support AI returned HTTP %d", resp.StatusCode)
		}
		var result struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 128000)).Decode(&result); err != nil {
			return "", err
		}
		if len(result.Choices) != 1 || result.Choices[0].FinishReason != "stop" {
			return "", errors.New("support AI response incomplete")
		}
		return result.Choices[0].Message.Content, nil
	}
}

func (a *App) GetSupportQueue(r *fastglue.Request) error {
	org, _, err := a.requireAuth(r, "settings.chatbot", "read")
	if err != nil {
		return nil
	}
	policy, err := supportPolicy(a.DB, org)
	if err != nil {
		return r.SendErrorEnvelope(500, "Cannot load policy", nil, "")
	}
	var jobs []models.SupportJob
	page, _ := strconv.Atoi(string(r.RequestCtx.QueryArgs().Peek("page")))
	if page < 0 || page > 10000 {
		page = 0
	}
	query := a.DB.Preload("Contact").Where("organization_id=?", org)
	if state := string(r.RequestCtx.QueryArgs().Peek("state")); state != "" {
		query = query.Where("state=?", state)
	}
	if err := query.Order("updated_at DESC").Offset(page * 50).Limit(50).Find(&jobs).Error; err != nil {
		return r.SendErrorEnvelope(500, "Cannot load queue", nil, "")
	}
	type count struct {
		State string `json:"state"`
		Count int64  `json:"count"`
	}
	var counts []count
	if err := a.DB.Model(&models.SupportJob{}).Select("state,count(*) AS count").Where("organization_id=?", org).Group("state").Scan(&counts).Error; err != nil {
		return r.SendErrorEnvelope(500, "Cannot load queue counts", nil, "")
	}
	var accounts []models.WhatsAppAccount
	if err := a.DB.Where("organization_id=?", org).Find(&accounts).Error; err != nil {
		return r.SendErrorEnvelope(500, "Cannot load accounts", nil, "")
	}
	budgets := []map[string]any{}
	start, err := billingStart(time.Now(), policy.BillingTimezone)
	if err != nil {
		return r.SendErrorEnvelope(500, "Cannot determine billing month", nil, "")
	}
	for _, account := range accounts {
		used, err := budgetUsage(a.DB, org, account.PhoneID, start)
		if err != nil {
			return r.SendErrorEnvelope(500, "Cannot load message budget", nil, "")
		}
		budgets = append(budgets, map[string]any{"account": account.Name, "used": used, "limit": support.MonthlyLimit, "resets_at": start.AddDate(0, 1, 0)})
	}
	var monthly []struct {
		Month             string `json:"month"`
		Account           string `json:"account"`
		IncomingCustomers int64  `json:"incoming_customers"`
		RepliedCustomers  int64  `json:"replied_customers"`
		Sends             int64  `json:"sends"`
	}
	if err := a.DB.Raw(`SELECT to_char(created_at AT TIME ZONE 'Asia/Karachi','YYYY-MM') AS month,whats_app_account AS account,count(DISTINCT contact_id) FILTER(WHERE direction='incoming') AS incoming_customers,count(DISTINCT contact_id) FILTER(WHERE direction='outgoing' AND status IN ('sent','delivered','read')) AS replied_customers,count(*) FILTER(WHERE direction='outgoing' AND status IN ('sent','delivered','read')) AS sends FROM messages WHERE organization_id=? AND deleted_at IS NULL AND created_at>=? GROUP BY 1,2 ORDER BY 1 DESC`, org, customerStart(time.Now()).AddDate(0, -5, 0)).Scan(&monthly).Error; err != nil {
		return r.SendErrorEnvelope(500, "Cannot load monthly activity", nil, "")
	}
	return r.SendEnvelope(map[string]any{"policy": policy, "jobs": jobs, "counts": counts, "budgets": budgets, "monthly": monthly, "version": builtin.InvestifySupportVersion, "now": time.Now()})
}

func (a *App) UpdateSupportPolicy(r *fastglue.Request) error {
	org, _, err := a.requireAuth(r, "settings.chatbot", "write")
	if err != nil {
		return nil
	}
	var input struct {
		Enabled         bool   `json:"enabled"`
		BillingTimezone string `json:"billing_timezone"`
	}
	if err := json.Unmarshal(r.RequestCtx.PostBody(), &input); err != nil {
		return r.SendErrorEnvelope(400, "Invalid settings", nil, "")
	}
	if input.BillingTimezone == "" {
		input.BillingTimezone = "Asia/Karachi"
	}
	if _, err := time.LoadLocation(input.BillingTimezone); err != nil {
		return r.SendErrorEnvelope(400, "Invalid billing timezone", nil, "")
	}
	settings, err := a.getChatbotSettingsCached(org, "")
	if input.Enabled && (err != nil || !settings.IsEnabled || !settings.AI.Enabled || settings.AI.Provider != models.AIProviderOpenAI || settings.AI.APIKey == "") {
		return r.SendErrorEnvelope(400, "Enable the existing OpenAI chatbot configuration first", nil, "")
	}
	policy, err := supportPolicy(a.DB, org)
	if err != nil {
		return r.SendErrorEnvelope(500, "Cannot load policy", nil, "")
	}
	if policy.ID == uuid.Nil {
		policy = models.SupportPolicy{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org}
	}
	if policy.Enabled {
		input.BillingTimezone = policy.BillingTimezone
	}
	if input.Enabled && !policy.Enabled {
		// WABA month boundaries may differ from the PKT customer allowance.
		// Verified against Meta's official timezone ID reference.
		zones := map[string]string{"105": "Asia/Karachi", "1": "America/Los_Angeles", "71": "Asia/Kolkata", "474": "UTC"}
		var accounts []models.WhatsAppAccount
		if err := a.DB.Where("organization_id=? AND status='active'", org).Find(&accounts).Error; err != nil {
			return r.SendErrorEnvelope(500, "Cannot verify billing timezone", nil, "")
		}
		resolved := ""
		for _, account := range accounts {
			account.DecryptSecrets(a.Config.App.EncryptionKey)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			info, err := a.WhatsApp.GetBusinessAccountInfo(ctx, account.ToWAAccount())
			cancel()
			if err != nil {
				return r.SendErrorEnvelope(400, "Could not verify the WhatsApp billing timezone for "+account.Name, nil, "")
			}
			zone, ok := zones[info.TimezoneID]
			if !ok {
				return r.SendErrorEnvelope(400, "Unmapped Meta billing timezone ID: "+info.TimezoneID, nil, "")
			}
			if resolved != "" && resolved != zone {
				return r.SendErrorEnvelope(400, "Accounts have different billing timezones; configure separate account policies first", nil, "")
			}
			resolved = zone
		}
		if resolved == "" {
			return r.SendErrorEnvelope(400, "No active WhatsApp account to verify", nil, "")
		}
		input.BillingTimezone = resolved
		now := time.Now()
		local := now.In(support.PKT)
		policy.EnabledAt = now
		policy.FirstRunAt = time.Date(local.Year(), local.Month(), local.Day()+1, 9, 0, 0, 0, support.PKT)
	}
	policy.Paused = !input.Enabled
	policy.Enabled = policy.Enabled || input.Enabled
	policy.BillingTimezone = input.BillingTimezone
	if err := a.DB.Save(&policy).Error; err != nil {
		return r.SendErrorEnvelope(500, "Cannot save policy", nil, "")
	}
	return r.SendEnvelope(policy)
}

func (a *App) SupportEmailPage(r *fastglue.Request) error {
	r.RequestCtx.SetContentType("text/html; charset=utf-8")
	r.RequestCtx.SetBodyString(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Investify email support</title><body style="font-family:system-ui;max-width:38rem;margin:4rem auto;padding:1rem;line-height:1.6"><h1>Investify support</h1><p>For further help, email our support team.</p><p><a style="display:inline-block;background:#2563eb;color:white;padding:12px 20px;border-radius:8px" href="mailto:support@investify.pk?subject=Investify%20support">Open email app</a></p><p>If your email app does not open, copy this address:<br><strong>support@investify.pk</strong></p><p>Include your question, whether you use web, Android or iOS, and any relevant error or screenshot. For account issues, include your Investify email.</p><p>Please do not send passwords, verification codes or payment card details.</p></body></html>`)
	return nil
}

// PreviewSupport runs synthetic regression cases through the real configured
// model. It never enqueues or sends a WhatsApp message.
func (a *App) PreviewSupport(r *fastglue.Request) error {
	org, _, err := a.requireAuth(r, "settings.chatbot", "write")
	if err != nil {
		return nil
	}
	var input struct {
		Case string `json:"case"`
	}
	if err := json.Unmarshal(r.RequestCtx.PostBody(), &input); err != nil {
		return r.SendErrorEnvelope(400, "Invalid case", nil, "")
	}
	cases := map[string]struct {
		Texts     []string
		Decision  string
		Questions int
	}{
		"greeting":    {[]string{"AOA"}, "skip", 0},
		"boilerplate": {[]string{"My Investify user email is demo@example.test and I have the following issues or feedback about the iOS App:"}, "skip", 0},
		"login":       {[]string{"I'm having issues logging in"}, "answer", 1},
		"combined":    {[]string{"I forgot my password. I already checked spam for the reset email.", "Can I use my portfolio on my laptop?", "Thanks"}, "answer", 2},
		"resolved":    {[]string{"I cannot log in", "It is fixed now, I signed in successfully. No help needed, thanks."}, "skip", 0},
		"roman_urdu":  {[]string{"AOA, login nahi ho raha. Laptop par bhi Investify use kar sakta hoon?"}, "answer", 2},
	}
	fixture, ok := cases[input.Case]
	if !ok {
		return r.SendErrorEnvelope(400, "Unknown regression case", nil, "")
	}
	settings, err := a.getChatbotSettingsCached(org, "")
	if err != nil {
		return r.SendErrorEnvelope(400, "AI settings unavailable", nil, "")
	}
	var transcript []support.Message
	for i, text := range fixture.Texts {
		transcript = append(transcript, support.Message{ID: strconv.Itoa(i + 1), Direction: "incoming", Pending: true, Text: text})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	prepared, err := support.Prepare(ctx, transcript, a.supportGenerator(settings))
	if err != nil {
		return r.SendErrorEnvelope(502, "AI regression check failed", nil, "")
	}
	expectedLanguage := "en"
	if input.Case == "roman_urdu" {
		expectedLanguage = "roman_ur"
	}
	passed := prepared.Decision.Decision == fixture.Decision && len(prepared.Questions) == fixture.Questions && prepared.Language == expectedLanguage && !prepared.Fallback
	return r.SendEnvelope(map[string]any{"case": input.Case, "passed": passed, "prepared": prepared})
}
