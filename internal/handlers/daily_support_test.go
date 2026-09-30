package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/builtin"
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/support"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/require"
)

func supportTestSetup(t *testing.T) (*App, *models.WhatsAppAccount, *models.Contact, models.SupportJob) {
	t.Helper()
	a := newProcessorTestApp(t)
	a.Config = &config.Config{}
	org, account := createProcessorTestOrg(t, a)
	contact := testutil.CreateTestContact(t, a.DB, org.ID)
	require.NoError(t, a.DB.Create(&models.SupportPolicy{OrganizationID: org.ID, Enabled: true, EnabledAt: time.Now().Add(-time.Hour), BillingTimezone: "Asia/Karachi"}).Error)
	job := models.SupportJob{OrganizationID: org.ID, Account: account.Name, ContactID: contact.ID, Revision: 1, State: "ready", PendingFrom: time.Now().Add(-time.Hour), LastInboundAt: time.Now().Add(-time.Minute), DueAt: time.Now().Add(-time.Second), Answer: "A consolidated answer", RetryAt: time.Now().Add(-time.Second)}
	require.NoError(t, a.DB.Create(&job).Error)
	return a, account, contact, job
}

func TestSupportReservationConcurrentCustomer(t *testing.T) {
	a, account, contact, job := supportTestSetup(t)
	opts := ChatbotSendOptions()
	opts.supportJobID = job.ID
	opts.supportRevision = job.Revision
	req := OutgoingMessageRequest{Account: account, Contact: contact, Type: models.MessageTypeText, Content: "answer"}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := a.reserveSupportSend(req, opts, a.createOutgoingMessage(req, opts))
			if err == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, wins.Load())
	start, _ := billingStart(time.Now(), "Asia/Karachi")
	used, err := budgetUsage(a.DB, account.OrganizationID, account.PhoneID, start)
	require.NoError(t, err)
	require.EqualValues(t, 1, used)
}

func TestSupportBudgetConcurrentLastSlot(t *testing.T) {
	a, account, contact, _ := supportTestSetup(t)
	// Historical sends must count on activation; don't start the budget at zero.
	historical := make([]models.Message, 999)
	for i := range historical {
		historical[i] = models.Message{OrganizationID: account.OrganizationID, WhatsAppAccount: account.Name, ContactID: contact.ID, Direction: models.DirectionOutgoing, MessageType: models.MessageTypeText, Status: models.MessageStatusSent}
	}
	require.NoError(t, a.DB.CreateInBatches(historical, 100).Error)
	user := testutil.CreateTestUser(t, a.DB, account.OrganizationID)
	opts := DefaultSendOptions()
	opts.SentByUserID = &user.ID
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		c := testutil.CreateTestContact(t, a.DB, account.OrganizationID)
		require.NoError(t, a.DB.Create(&models.SupportJob{OrganizationID: account.OrganizationID, Account: account.Name, ContactID: c.ID, LastInboundAt: time.Now()}).Error)
		wg.Add(1)
		go func(c *models.Contact) {
			defer wg.Done()
			req := OutgoingMessageRequest{Account: account, Contact: c, Type: models.MessageTypeText, Content: "manual"}
			_, err := a.reserveSupportSend(req, opts, a.createOutgoingMessage(req, opts))
			if err == nil {
				wins.Add(1)
			}
		}(c)
	}
	wg.Wait()
	require.EqualValues(t, 1, wins.Load())
}

func TestSupportManualRejectionRestoresEligibility(t *testing.T) {
	a, account, contact, _ := supportTestSetup(t)
	user := testutil.CreateTestUser(t, a.DB, account.OrganizationID)
	opts := DefaultSendOptions()
	opts.SentByUserID = &user.ID
	req := OutgoingMessageRequest{Account: account, Contact: contact, Type: models.MessageTypeText, Content: "manual"}
	msg := a.createOutgoingMessage(req, opts)
	_, err := a.reserveSupportSend(req, opts, msg)
	require.NoError(t, err)
	a.DB.Model(msg).Update("status", models.MessageStatusFailed)
	a.finishSupportSend(msg, "", &whatsapp.RequestError{StatusCode: 400, Message: "rejected"})
	blocked, err := customerSuppressed(a.DB, account.OrganizationID, contact.ID, time.Now())
	require.NoError(t, err)
	require.False(t, blocked)
	start, _ := billingStart(time.Now(), "Asia/Karachi")
	used, err := budgetUsage(a.DB, account.OrganizationID, account.PhoneID, start)
	require.NoError(t, err)
	require.Zero(t, used)
}

func TestSupportUnknownOutcomeDoesNotReleaseBudget(t *testing.T) {
	a, account, contact, job := supportTestSetup(t)
	opts := ChatbotSendOptions()
	opts.supportJobID = job.ID
	opts.supportRevision = job.Revision
	req := OutgoingMessageRequest{Account: account, Contact: contact, Type: models.MessageTypeText}
	msg := a.createOutgoingMessage(req, opts)
	_, err := a.reserveSupportSend(req, opts, msg)
	require.NoError(t, err)
	a.DB.Model(msg).Update("status", models.MessageStatusFailed)
	a.finishSupportSend(msg, "", errors.New("connection lost"))
	blocked, err := customerSuppressed(a.DB, account.OrganizationID, contact.ID, time.Now())
	require.NoError(t, err)
	require.True(t, blocked)
	start, _ := billingStart(time.Now(), "Asia/Karachi")
	used, err := budgetUsage(a.DB, account.OrganizationID, account.PhoneID, start)
	require.NoError(t, err)
	require.EqualValues(t, 1, used)
}

func TestSupportRejectsStaleAnswerAndLegacyAutomation(t *testing.T) {
	a, account, contact, job := supportTestSetup(t)
	req := OutgoingMessageRequest{Account: account, Contact: contact, Type: models.MessageTypeText}
	opts := ChatbotSendOptions()
	_, err := a.reserveSupportSend(req, opts, a.createOutgoingMessage(req, opts))
	require.ErrorContains(t, err, "legacy")
	opts.supportJobID = job.ID
	opts.supportRevision = job.Revision - 1
	_, err = a.reserveSupportSend(req, opts, a.createOutgoingMessage(req, opts))
	require.ErrorContains(t, err, "changed")
}

func TestSupportDelayedInboundDoesNotExtendWindow(t *testing.T) {
	a, account, contact, job := supportTestSetup(t)
	original := job.LastInboundAt
	msg := &models.Message{BaseModel: models.BaseModel{ID: uuid.New(), CreatedAt: time.Now()}, OrganizationID: account.OrganizationID, WhatsAppAccount: account.Name, ContactID: contact.ID, Direction: models.DirectionIncoming, MessageType: models.MessageTypeText, Content: "question"}
	require.NoError(t, a.DB.Create(msg).Error)
	require.True(t, a.captureSupportIncoming(account, contact, msg, strconv.FormatInt(time.Now().Add(-2*time.Hour).Unix(), 10)))
	require.NoError(t, a.DB.First(&job, "id=?", job.ID).Error)
	require.WithinDuration(t, original, job.LastInboundAt, time.Millisecond)
	require.EqualValues(t, 2, job.Revision)
	require.Empty(t, job.Answer)
	// A stale AI worker cannot overwrite the new revision.
	stale := job
	stale.Revision--
	a.supportJobUpdate(stale, map[string]any{"answer": "stale", "state": "ready"})
	require.NoError(t, a.DB.First(&job, "id=?", job.ID).Error)
	require.Empty(t, job.Answer)
}

type supportTransport func(*http.Request) (*http.Response, error)

func (f supportTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSupportWorkerPreparesThenSendsOnce(t *testing.T) {
	a, account, contact, job := supportTestSetup(t)
	require.NoError(t, a.DB.Model(&job).Updates(map[string]any{"state": "pending", "answer": ""}).Error)
	msg := models.Message{OrganizationID: account.OrganizationID, WhatsAppAccount: account.Name, ContactID: contact.ID, Direction: models.DirectionIncoming, MessageType: models.MessageTypeText, Content: "Can I use Investify on my laptop?"}
	require.NoError(t, a.DB.Create(&msg).Error)
	require.NoError(t, a.DB.Create(&models.ChatbotSettings{OrganizationID: account.OrganizationID, IsEnabled: true, AI: models.AIConfig{Enabled: true, Provider: models.AIProviderOpenAI, APIKey: "test-only", Model: "gpt-4.1-nano"}}).Error)
	calls := 0
	a.HTTPClient = &http.Client{Transport: supportTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		answer := "Open https://www.investify.pk and sign in with the same account."
		if calls == 1 {
			d := support.Decision{Decision: "answer", Reason: "Web question", Language: "en", Summary: "Desktop access", Questions: []support.Question{{Category: "web", Question: "Use on laptop?", MessageIDs: []string{msg.ID.String()}, AttemptedSteps: []string{}}}}
			b, _ := json.Marshal(d)
			answer = string(b)
		}
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: make(http.Header)}, nil
	})}
	policy, _ := supportPolicy(a.DB, account.OrganizationID)
	require.NoError(t, a.DB.First(&job, "id=?", job.ID).Error)
	a.processSupportJob(context.Background(), policy, job)
	require.NoError(t, a.DB.First(&job, "id=?", job.ID).Error)
	require.Equal(t, "ready", job.State)
	require.Contains(t, job.Answer, "support@investify.pk")
	a.processSupportJob(context.Background(), policy, job)
	var sends int64
	require.NoError(t, a.DB.Model(&models.SupportSend{}).Where("organization_id=?", account.OrganizationID).Count(&sends).Error)
	require.EqualValues(t, 1, sends)
	a.processSupportJob(context.Background(), policy, job)
	require.NoError(t, a.DB.Model(&models.SupportSend{}).Where("organization_id=?", account.OrganizationID).Count(&sends).Error)
	require.EqualValues(t, 1, sends)
}

func TestSupportGeneratorStructuredContract(t *testing.T) {
	a := &App{HTTPClient: &http.Client{Transport: supportTransport(func(r *http.Request) (*http.Response, error) {
		var request map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		format := request["response_format"].(map[string]any)
		require.Equal(t, "json_schema", format["type"])
		schema := format["json_schema"].(map[string]any)
		require.Equal(t, true, schema["strict"])
		require.Equal(t, false, schema["schema"].(map[string]any)["additionalProperties"])
		require.Len(t, request["messages"], 2)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{}"},"finish_reason":"length"}]}`)), Header: make(http.Header)}, nil
	})}}
	_, err := a.supportGenerator(&models.ChatbotSettings{AI: models.AIConfig{Provider: models.AIProviderOpenAI, Model: "gpt-4.1-mini"}})(context.Background(), builtin.InvestifyQueuePrompt, `{"messages":[]}`)
	require.ErrorContains(t, err, "incomplete")
}
