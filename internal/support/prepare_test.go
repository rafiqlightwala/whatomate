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
		require.Contains(t, system, "APPROVED KNOWLEDGE")
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
