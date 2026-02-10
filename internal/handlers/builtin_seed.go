package handlers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/builtin"
	"github.com/shridarpatil/whatomate/internal/models"
	"gorm.io/gorm"
)

const investifyBuiltinVersion = "builtin:investify:keywords:v2"
const investifyBuiltinPriorityBase = 2000
const investifySeedGreetingMessage = "Thank you for contacting the Investify App Support Team. How can we assist you today?\nانویسٹیفائی ایپ سپورٹ ٹیم سے رابطہ کرنے کا شکریہ۔ ہم آپ کی کس طرح مدد کر سکتے ہیں؟"
const investifySeedAIContextName = "Investify Support FAQ (Built-in)"

// SeedBuiltinInvestifyForAllOrganizations ensures builtin Investify keyword rules
// exist for every organization. It is safe to run on every startup.
func (a *App) SeedBuiltinInvestifyForAllOrganizations() error {
	var orgs []models.Organization
	if err := a.DB.Select("id").Find(&orgs).Error; err != nil {
		return fmt.Errorf("list organizations: %w", err)
	}

	for _, org := range orgs {
		if _, err := a.seedBuiltinInvestifyKeywordRules(a.DB, org.ID); err != nil {
			return fmt.Errorf("seed org %s: %w", org.ID, err)
		}
		if err := a.seedBuiltinGreetingMessage(a.DB, org.ID); err != nil {
			return fmt.Errorf("seed org %s greeting: %w", org.ID, err)
		}
		if err := a.seedBuiltinInvestifyAIContext(a.DB, org.ID); err != nil {
			return fmt.Errorf("seed org %s ai context: %w", org.ID, err)
		}
		// Ensure runtime uses freshly seeded rules and not stale Redis cache.
		a.InvalidateKeywordRulesCache(org.ID)
		a.InvalidateChatbotSettingsCache(org.ID)
		a.InvalidateAIContextsCache(org.ID)
	}
	return nil
}

// SeedBuiltinInvestifyForOrganization seeds builtin Investify keyword rules for one org.
// If tx is nil, a.DB is used.
func (a *App) SeedBuiltinInvestifyForOrganization(tx *gorm.DB, orgID uuid.UUID) error {
	db := tx
	if db == nil {
		db = a.DB
	}
	_, err := a.seedBuiltinInvestifyKeywordRules(db, orgID)
	if err != nil {
		return err
	}
	if err := a.seedBuiltinGreetingMessage(db, orgID); err != nil {
		return err
	}
	if err := a.seedBuiltinInvestifyAIContext(db, orgID); err != nil {
		return err
	}
	// If called outside request flow, keep cache consistent.
	a.InvalidateKeywordRulesCache(orgID)
	a.InvalidateChatbotSettingsCache(orgID)
	a.InvalidateAIContextsCache(orgID)
	return nil
}

func (a *App) seedBuiltinInvestifyKeywordRules(db *gorm.DB, orgID uuid.UUID) (int, error) {
	replies, err := builtin.LoadInvestifyReplies()
	if err != nil {
		return 0, fmt.Errorf("load builtin replies: %w", err)
	}

	entries, err := builtin.LoadInvestifyKeywordEntries()
	if err != nil {
		return 0, fmt.Errorf("load builtin keyword entries: %w", err)
	}

	seeded := 0
	for idx, entry := range entries {
		replyByLang, ok := replies[entry.ReplyID]
		if !ok {
			continue
		}
		body := strings.TrimSpace(replyByLang[entry.Language])
		if body == "" {
			continue
		}

		compiledKeyword := compileKeywordPattern(entry.Keywords)
		if compiledKeyword == "" {
			continue
		}

		conditionTag := fmt.Sprintf("%s:%03d", investifyBuiltinVersion, idx+1)
		rule := models.KeywordRule{
			BaseModel:       models.BaseModel{ID: uuid.New()},
			OrganizationID:  orgID,
			WhatsAppAccount: "",
			Name:            fmt.Sprintf("Investify %s (%s) #%d", strings.ReplaceAll(entry.ReplyID, "_", " "), entry.Language, idx+1),
			IsEnabled:       true,
			Priority:        investifyBuiltinPriorityBase - idx,
			Keywords:        []string{compiledKeyword},
			MatchType:       models.MatchTypeRegex,
			CaseSensitive:   false,
			ResponseType:    models.ResponseTypeText,
			ResponseContent: models.JSONB{
				"body": body,
				"delay_range": models.JSONB{
					"min": entry.DelayRange.Min,
					"max": entry.DelayRange.Max,
				},
				"reply_id": entry.ReplyID,
				"language": entry.Language,
				"builtin":  "investify",
			},
			Conditions: conditionTag,
		}

		var existing models.KeywordRule
		err := db.Where("organization_id = ? AND conditions = ?", orgID, conditionTag).First(&existing).Error
		if err == nil {
			// Seed-once behavior: if builtin rule already exists, keep user edits intact.
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return seeded, err
		}
		if err := db.Create(&rule).Error; err != nil {
			return seeded, err
		}
		seeded++
	}

	return seeded, nil
}

func compileKeywordPattern(keywords []string) string {
	cleaned := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		trimmed := strings.TrimSpace(kw)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}

	if len(cleaned) == 0 {
		return ""
	}
	if len(cleaned) == 1 {
		return "(?i)" + cleaned[0]
	}

	// Go's regexp engine (RE2) doesn't support lookaheads. Build an
	// order-insensitive AND pattern by matching every keyword permutation.
	perms := keywordPermutations(cleaned)
	orderPatterns := make([]string, 0, len(perms))
	for _, perm := range perms {
		var b strings.Builder
		b.WriteString("(?s).*")
		for _, kw := range perm {
			b.WriteString("(")
			b.WriteString(kw)
			b.WriteString(").*")
		}
		orderPatterns = append(orderPatterns, b.String())
	}

	return "(?i)(?:" + strings.Join(orderPatterns, "|") + ")"
}

func keywordPermutations(items []string) [][]string {
	if len(items) == 0 {
		return nil
	}

	used := make([]bool, len(items))
	current := make([]string, 0, len(items))
	perms := make([][]string, 0)

	var build func()
	build = func() {
		if len(current) == len(items) {
			perm := append([]string(nil), current...)
			perms = append(perms, perm)
			return
		}
		for i := range items {
			if used[i] {
				continue
			}
			used[i] = true
			current = append(current, items[i])
			build()
			current = current[:len(current)-1]
			used[i] = false
		}
	}
	build()

	return perms
}

func (a *App) seedBuiltinGreetingMessage(db *gorm.DB, orgID uuid.UUID) error {
	var settings models.ChatbotSettings
	err := db.Where("organization_id = ? AND whats_app_account = ''", orgID).First(&settings).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		settings = models.ChatbotSettings{
			BaseModel:          models.BaseModel{ID: uuid.New()},
			OrganizationID:     orgID,
			WhatsAppAccount:    "",
			IsEnabled:          false,
			SessionTimeoutMins: 30,
		}
		if err := db.Create(&settings).Error; err != nil {
			return err
		}
	}

	if strings.TrimSpace(settings.DefaultResponse) == "" {
		return db.Model(&settings).Update("default_response", investifySeedGreetingMessage).Error
	}
	return nil
}

func (a *App) seedBuiltinInvestifyAIContext(db *gorm.DB, orgID uuid.UUID) error {
	staticContent := strings.TrimSpace(builtin.LoadInvestifyAIContextSummary())
	if staticContent == "" {
		return nil
	}

	updateFields := map[string]interface{}{
		"is_enabled":       true,
		"priority":         900,
		"context_type":     models.ContextTypeStatic,
		"trigger_keywords": models.StringArray{},
		"static_content":   staticContent,
	}

	var existing models.AIContext
	err := db.Where("organization_id = ? AND whats_app_account = '' AND name = ?", orgID, investifySeedAIContextName).
		First(&existing).Error
	if err == nil {
		// Always refresh builtin AI context on startup/build.
		return db.Model(&existing).Updates(updateFields).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	aiCtx := models.AIContext{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  orgID,
		WhatsAppAccount: "",
		Name:            investifySeedAIContextName,
		IsEnabled:       true,
		Priority:        900,
		ContextType:     models.ContextTypeStatic,
		TriggerKeywords: models.StringArray{},
		StaticContent:   staticContent,
	}
	return db.Create(&aiCtx).Error
}
