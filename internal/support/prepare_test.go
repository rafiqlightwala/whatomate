package support

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func sampleMessages() []Message {
	return []Message{
		{ID: "old", Direction: "outgoing", Text: "Old bot said desktop was unavailable."},
		{ID: "one", Direction: "incoming", Text: "Can't log in. Already checked spam.", Pending: true},
		{ID: "two", Direction: "incoming", Text: "Can I use my portfolio on a laptop?", Pending: true},
		{ID: "three", Direction: "incoming", Text: "Thanks", Pending: true},
	}
}

func sampleDecision() Decision {
	return Decision{Decision: "answer", Reason: "Two unanswered requests", Language: "en", Summary: "Login recovery and desktop access", Questions: []Question{
		{Category: "account", Question: "How can I recover login?", MessageIDs: []string{"one"}, AttemptedSteps: []string{"Checked spam"}},
		{Category: "web", Question: "Can I use my portfolio on a laptop?", MessageIDs: []string{"two"}, AttemptedSteps: []string{}},
	}}
}

func decisionJSON(d Decision) string { b, _ := json.Marshal(d); return string(b) }

func TestPrepareCombinesQuestionsAndPreservesAttempts(t *testing.T) {
	calls := 0
	got, err := Prepare(context.Background(), sampleMessages(), func(_ context.Context, system, input string) (string, error) {
		calls++
		if calls == 1 {
			require.Contains(t, system, "classify")
			require.Contains(t, input, "Already checked spam")
			return decisionJSON(sampleDecision()), nil
		}
		require.Contains(t, system, "SUPPORT PLAYBOOK")
		require.Contains(t, system, "https://www.investify.pk")
		require.Contains(t, input, "Checked spam")
		require.Contains(t, input, "Can I use my portfolio on a laptop?")
		return "Login: use password recovery.\n\nWeb: open https://www.investify.pk using the same account.", nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Len(t, got.Questions, 2)
	require.Contains(t, got.Body, "support@investify.pk")
	require.False(t, got.Fallback)
	require.NotEmpty(t, got.Version)
}

func TestNoiseSkipsAIEntirely(t *testing.T) {
	got, err := Prepare(context.Background(), []Message{
		{ID: "old", Direction: "incoming", Text: "An old resolved question"},
		{ID: "new", Direction: "incoming", Text: "AOA", Pending: true},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "skip", got.Decision.Decision)
	require.Empty(t, got.Body)
}

func TestInvalidClassificationNeverGeneratesAnswer(t *testing.T) {
	for _, invalid := range []string{"not json", "```json\n{}\n```", decisionJSON(sampleDecision()) + "{}", strings.Replace(decisionJSON(sampleDecision()), `"one"`, `"invented"`, 1), strings.Replace(decisionJSON(sampleDecision()), `"one"`, `"old"`, 1), strings.Replace(decisionJSON(sampleDecision()), `"answer"`, `"send_now"`, 1), `{"decision":"skip","reason":"noise","language":"en","questions":[],"send":true}`} {
		calls := 0
		got, err := Prepare(context.Background(), sampleMessages(), func(context.Context, string, string) (string, error) {
			calls++
			return invalid, nil
		})
		require.Error(t, err)
		require.Equal(t, 1, calls)
		require.Empty(t, got.Body)
	}
}

func TestAnswerFailureUsesOneCompleteEmailHandoff(t *testing.T) {
	for _, language := range []string{"en", "roman_ur", "ur"} {
		for _, answer := range []string{"", strings.Repeat("ا", MaxBodyRunes+1), "provider error"} {
			calls := 0
			got, err := Prepare(context.Background(), sampleMessages(), func(context.Context, string, string) (string, error) {
				calls++
				if calls == 1 {
					d := sampleDecision()
					d.Language = language
					return decisionJSON(d), nil
				}
				if answer == "provider error" {
					return "", errors.New("unavailable")
				}
				return answer, nil
			})
			require.NoError(t, err)
			require.True(t, got.Fallback)
			require.Equal(t, 1, strings.Count(got.Body, "support@investify.pk"))
			require.LessOrEqual(t, utf8.RuneCountInString(got.Body), MaxReplyRunes)
		}
	}
}

func TestClassificationFailureReturnsErrorWithoutReply(t *testing.T) {
	got, err := Prepare(context.Background(), sampleMessages(), func(context.Context, string, string) (string, error) { return "", errors.New("offline") })
	require.Error(t, err)
	require.Empty(t, got.Body)
}

func TestCancelledPreparationCannotReturnSendableBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	got, err := Prepare(ctx, sampleMessages(), func(context.Context, string, string) (string, error) {
		calls++
		cancel()
		return decisionJSON(sampleDecision()), nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
	require.Empty(t, got.Body)
}

func TestOversizedTranscriptIsNotSilentlyTruncated(t *testing.T) {
	got, err := Prepare(context.Background(), []Message{{ID: "one", Direction: "incoming", Pending: true, Text: strings.Repeat("question ", 9000)}}, nil)
	require.ErrorContains(t, err, "compaction")
	require.Empty(t, got.Body)
}

func TestPrepareRemovesLegacyEmailURLFromModelOutput(t *testing.T) {
	calls := 0
	got, err := Prepare(context.Background(), sampleMessages(), func(context.Context, string, string) (string, error) {
		calls++
		if calls == 1 {
			return decisionJSON(sampleDecision()), nil
		}
		return "Open https://www.investify.pk on your laptop. For support use https://wa.recubetech.com/support/email", nil
	})
	require.NoError(t, err)
	require.NotContains(t, got.Body, "wa.recubetech.com/support/email")
	require.Contains(t, got.Body, "support@investify.pk")
	require.Contains(t, got.Body, "https://www.investify.pk")
}

func TestParseDecisionDropsContextEvidenceButNeedsPending(t *testing.T) {
	msgs := []Message{
		{ID: "old", Direction: "incoming", Text: "Portfolio not loading"},
		{ID: "reply", Direction: "outgoing", Text: "Log out and in"},
		{ID: "new", Direction: "incoming", Pending: true, Text: "Still not loading"},
	}
	d, err := ParseDecision(`{"decision":"answer","reason":"Follow-up","language":"en","summary":"Still failing","questions":[{"category":"portfolio","question":"Still not loading","message_ids":["old","new"],"attempted_steps":["logged out and in"]}]}`, msgs)
	require.NoError(t, err)
	require.Equal(t, []string{"new"}, d.Questions[0].MessageIDs)
	_, err = ParseDecision(`{"decision":"answer","reason":"Old","language":"en","summary":"Old request","questions":[{"category":"portfolio","question":"Not loading","message_ids":["old"],"attempted_steps":[]}]}`, msgs)
	require.Error(t, err, "a question backed only by context must not create work")
}

func TestFooterNeverMentionsAutomationAndSkipsDuplicateEmail(t *testing.T) {
	for _, lang := range []string{"en", "roman_ur", "ur"} {
		for _, follow := range []bool{false, true} {
			f := footer(lang, follow)
			require.Contains(t, f, "support@investify.pk")
			require.NotContains(t, strings.ToLower(f), "automat")
			require.NotContains(t, strings.ToLower(f), "limited")
		}
	}
	msgs := []Message{{ID: "1", Direction: "incoming", Pending: true, Text: "Can I export to Excel?"}}
	calls := 0
	gen := func(_ context.Context, _ string, _ string) (string, error) {
		calls++
		if calls == 1 {
			return `{"decision":"answer","reason":"Export","language":"en","summary":"Excel export","questions":[{"category":"portfolio","question":"Export to Excel?","message_ids":["1"],"attempted_steps":[]}]}`, nil
		}
		return "Excel export isn't available yet. For a copy of your data, email support@investify.pk.", nil
	}
	p, err := Prepare(context.Background(), msgs, gen)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(p.Body, "support@investify.pk"), "no second email line when the answer already gives it")
}
