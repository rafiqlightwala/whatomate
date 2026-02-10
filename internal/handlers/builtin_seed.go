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

const investifyBuiltinVersion = "builtin:investify:keywords:v1"

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
		// Ensure runtime uses freshly seeded rules and not stale Redis cache.
		a.InvalidateKeywordRulesCache(org.ID)
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
	// If called outside request flow, keep cache consistent.
	a.InvalidateKeywordRulesCache(orgID)
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
			Priority:        1000 - idx,
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
	if len(keywords) == 0 {
		return ""
	}
	if len(keywords) == 1 {
		return "(?i)" + keywords[0]
	}

	parts := make([]string, 0, len(keywords)+2)
	parts = append(parts, "(?is)")
	for _, kw := range keywords {
		parts = append(parts, "(?=.*"+kw+")")
	}
	parts = append(parts, ".*")
	return strings.Join(parts, "")
}
