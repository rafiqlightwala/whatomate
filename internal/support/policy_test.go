package support

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDueAt(t *testing.T) {
	for _, tc := range []struct{ name, now, inbound, want string }{
		{"evening waits for morning", "2026-10-01T18:00:00+05:00", "2026-10-01T17:59:00+05:00", "2026-10-02T09:00:00+05:00"},
		{"nine oclock expiry", "2026-10-02T08:00:00+05:00", "2026-10-01T09:00:00+05:00", "2026-10-02T08:52:00+05:00"},
		{"earlier expiry is not fixed 0852", "2026-10-02T07:00:00+05:00", "2026-10-01T08:15:00+05:00", "2026-10-02T08:07:00+05:00"},
		{"during batch", "2026-10-02T09:00:00+05:00", "2026-10-02T08:59:00+05:00", "2026-10-02T09:00:00+05:00"},
		{"new message after batch", "2026-10-02T09:01:00+05:00", "2026-10-02T09:01:00+05:00", "2026-10-03T08:53:00+05:00"},
		{"late worker catches up", "2026-10-02T08:55:00+05:00", "2026-10-01T09:00:00+05:00", "2026-10-02T08:55:00+05:00"},
		{"server uses UTC", "2026-10-01T13:00:00Z", "2026-10-01T12:59:00Z", "2026-10-02T09:00:00+05:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DueAt(at(tc.now), at(tc.inbound))
			require.NoError(t, err)
			require.True(t, got.Equal(at(tc.want)), "got %v want %s", got, tc.want)
		})
	}
	now := at("2026-10-02T09:00:00+05:00")
	_, err := DueAt(now, now.Add(-24*time.Hour))
	require.ErrorIs(t, err, ErrExpired)
	_, err = DueAt(now, now.Add(time.Second))
	require.Error(t, err)
	_, err = DueAt(now, time.Time{})
	require.Error(t, err)
}

func TestCustomerMonthUsesPKT(t *testing.T) {
	require.Equal(t, "2026-10", CustomerMonth(at("2026-09-30T19:00:00Z")))
	require.Equal(t, "2026-09", CustomerMonth(at("2026-09-30T18:59:59Z")))
}

func TestNoiseDoesNotDiscardRequests(t *testing.T) {
	for _, tc := range []struct {
		text, media string
		noise       bool
	}{
		{"AOA", "", true},
		{"Thanks!", "", true},
		{"👍", "", true},
		{"My Investify user email is example@example.test and I have the following issues or feedback about the iOS App:", "", true},
		{"My Investify user email is example@example.test and I have the following issues or feedback:", "", true},
		{"My Investify user email is example@example.test and I have the following issues or feedback: can't log in", "", false},
		{"My Investify user email is example@example.test and I have the following issues or feedback:\nLogin fails", "", false},
		{"I'm having issues logging in", "", false},
		{"AOA, login nahi ho raha", "", false},
		{"", "image", false},
		{"thanks", "document", false},
		{"", "sticker", true},
		{"web app?", "", false},
		{"1234", "", false},
	} {
		t.Run(tc.text+tc.media, func(t *testing.T) { require.Equal(t, tc.noise, ObviousNoise(tc.text, tc.media)) })
	}
}
