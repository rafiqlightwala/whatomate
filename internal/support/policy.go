// Package support prepares consolidated support replies. It deliberately has no
// network send capability: the persistent worker must authorize every send.
package support

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	MonthlyLimit  = 1000
	ExpiryMargin  = 8 * time.Minute
	MaxBodyRunes  = 3000
	MaxReplyRunes = 3500
)

var ErrExpired = errors.New("customer service window expired")

// PKT has no daylight saving time. A fixed zone also works in minimal images
// without depending on the host's local timezone or tzdata installation.
var PKT = time.FixedZone("Asia/Karachi", 5*60*60)

func CustomerMonth(t time.Time) string { return t.In(PKT).Format("2006-01") }

// DueAt returns the earlier of the next 09:00 PKT batch and eight minutes
// before the last verified customer event's 24-hour expiry. A delayed worker
// may catch up within the remaining window, but never after expiry. Call again
// immediately before dispatch, using the latest inbound timestamp.
func DueAt(now, lastInbound time.Time) (time.Time, error) {
	if lastInbound.IsZero() || lastInbound.After(now) {
		return time.Time{}, errors.New("invalid customer event timestamp")
	}
	expires := lastInbound.Add(24 * time.Hour)
	if !now.Before(expires) {
		return time.Time{}, ErrExpired
	}
	local := now.In(PKT)
	batch := time.Date(local.Year(), local.Month(), local.Day(), 9, 0, 0, 0, PKT)
	if batch.Before(now) {
		batch = batch.AddDate(0, 0, 1)
	}
	due := expires.Add(-ExpiryMargin)
	if batch.Before(due) {
		due = batch
	}
	if due.Before(now) {
		due = now
	}
	return due, nil
}

// Only exact empty introductions are stripped. An appended request must reach
// the classifier, including on the same line after the colon.
var emptyIntroduction = regexp.MustCompile(`(?i)^my investify user email is\s+\S+\s+and i have the following issues or feedback(?: about the (?:ios|android) app)?\s*:\s*$`)

// ObviousNoise is intentionally conservative. It does not decide that an issue
// is resolved, and must be applied to ALL pending messages, never just the last.
func ObviousNoise(text, media string) bool {
	if media != "" && media != "sticker" && media != "reaction" {
		return false
	}
	text = strings.TrimSpace(text)
	if emptyIntroduction.MatchString(text) {
		return true
	}
	normal := strings.ToLower(strings.TrimFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	}))
	switch normal {
	case "", "aoa", "a.o.a", "assalam o alaikum", "assalamualaikum", "salam", "salaam", "السلام علیکم", "hi", "hello", "hey", "ok", "okay", "thanks", "thank you", "shukriya", "شکریہ":
		return true
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return false
		}
	}
	return true
}
