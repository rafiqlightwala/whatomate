package handlers

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/builtin"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileKeywordPattern_SingleKeyword(t *testing.T) {
	pattern := compileKeywordPattern([]string{`(info|information|infi)`})
	require.NotEmpty(t, pattern)
	assert.NotContains(t, pattern, "(?=.*")

	re, err := regexp.Compile(pattern)
	require.NoError(t, err)
	assert.True(t, re.MatchString("Information"))
	assert.False(t, re.MatchString("broker"))
}

func TestCompileKeywordPattern_MultiKeyword_AndAcrossOrder(t *testing.T) {
	pattern := compileKeywordPattern([]string{`(create)`, `(account)`})
	require.NotEmpty(t, pattern)
	assert.NotContains(t, pattern, "(?=.*")

	re, err := regexp.Compile(pattern)
	require.NoError(t, err)
	assert.True(t, re.MatchString("how to create account"))
	assert.True(t, re.MatchString("account create issue"))
	assert.False(t, re.MatchString("create only"))
	assert.False(t, re.MatchString("account only"))
}

func TestSeedBuiltinInvestifyAIContext_OverridesExistingContext(t *testing.T) {
	app := newProcessorTestApp(t)
	org, _ := createProcessorTestOrg(t, app)

	existing := models.AIContext{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		WhatsAppAccount: "",
		Name:            investifySeedAIContextName,
		IsEnabled:       false,
		Priority:        10,
		ContextType:     models.ContextTypeAPI,
		TriggerKeywords: models.StringArray{"custom"},
		StaticContent:   "custom content",
	}
	require.NoError(t, app.DB.Create(&existing).Error)

	require.NoError(t, app.seedBuiltinInvestifyAIContext(app.DB, org.ID))

	var updated models.AIContext
	require.NoError(t, app.DB.Where("id = ?", existing.ID).First(&updated).Error)
	assert.True(t, updated.IsEnabled)
	assert.Equal(t, 900, updated.Priority)
	assert.Equal(t, models.ContextTypeStatic, updated.ContextType)
	assert.Empty(t, updated.TriggerKeywords)
	assert.Equal(t, strings.TrimSpace(builtin.LoadInvestifyAIContextSummary()), strings.TrimSpace(updated.StaticContent))
}

func TestSeedBuiltinInvestifyKeywordRules_OverridesExistingRule(t *testing.T) {
	app := newProcessorTestApp(t)
	org, _ := createProcessorTestOrg(t, app)

	_, err := app.seedBuiltinInvestifyKeywordRules(app.DB, org.ID)
	require.NoError(t, err)

	entries, err := builtin.LoadInvestifyKeywordEntries()
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	replies, err := builtin.LoadInvestifyReplies()
	require.NoError(t, err)

	first := entries[0]
	conditionTag := fmt.Sprintf("%s:%03d", investifyBuiltinVersion, 1)

	var existing models.KeywordRule
	require.NoError(t, app.DB.Where("organization_id = ? AND conditions = ?", org.ID, conditionTag).First(&existing).Error)

	require.NoError(t, app.DB.Model(&existing).Updates(map[string]interface{}{
		"name":             "Custom Greeting",
		"is_enabled":       false,
		"priority":         1,
		"keywords":         models.StringArray{"(?i)custom"},
		"match_type":       models.MatchTypeContains,
		"response_type":    models.ResponseTypeText,
		"response_content": models.JSONB{"body": "custom body"},
	}).Error)

	_, err = app.seedBuiltinInvestifyKeywordRules(app.DB, org.ID)
	require.NoError(t, err)

	var updated models.KeywordRule
	require.NoError(t, app.DB.Where("id = ?", existing.ID).First(&updated).Error)

	assert.True(t, updated.IsEnabled)
	assert.Equal(t, investifyBuiltinPriorityBase, updated.Priority)
	assert.Equal(t, models.MatchTypeRegex, updated.MatchType)
	assert.False(t, updated.CaseSensitive)
	assert.Equal(t, models.StringArray{compileKeywordPattern(first.Keywords)}, updated.Keywords)
	assert.Equal(t, strings.TrimSpace(replies[first.ReplyID][first.Language]), strings.TrimSpace(fmt.Sprintf("%v", updated.ResponseContent["body"])))
	assert.Equal(t, conditionTag, updated.Conditions)
}
