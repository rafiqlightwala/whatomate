package support

import (
	"fmt"
	"math"
	"time"
)

// MaxAutomaticReplies is the hard ceiling of automated replies per customer per PKT month.
// The first reply is always allowed (subject to the number budget); the second only when
// SecondReplyPacing says the month's quota can afford it.
const MaxAutomaticReplies = 2

// Pacing tunables. The reserve protects first replies for the rest of the month: a projection
// of first-reply (and manual) demand, inflated by SurgeFactor, plus a fixed floor.
const (
	PacingSurgeFactor = 1.5
	PacingReserve     = 50
	pacingMinDays     = 1.0
)

// PacingInput describes the business number's month so far. Baseline demand counts only first
// automatic replies and manual sends; second replies are excluded so they cannot inflate the
// projection that decides whether more second replies are affordable.
type PacingInput struct {
	Now        time.Time
	MonthStart time.Time // billing month start
	MonthEnd   time.Time // next billing month start
	Limit      int       // e.g. 1000 sends per number per month
	Used       int       // every send charged this month, including reservations and second replies

	// BaselineDaily holds first-reply + manual sends per calendar day for the most recent days,
	// oldest first, ending with today (partial). It may reach into the previous month.
	BaselineDaily []int
	// BaselineMonthToDate is first-reply + manual sends since MonthStart.
	BaselineMonthToDate int
	// PreviousMonthDailyAverage is used while the current month has too little history.
	PreviousMonthDailyAverage float64

	SecondRepliesToday int
}

type PacingDecision struct {
	Allowed        bool    `json:"allowed"`
	Reason         string  `json:"reason"`
	DailyRate      float64 `json:"daily_rate"`       // projected baseline sends per day
	DaysRemaining  float64 `json:"days_remaining"`   // until the billing month resets
	Reserved       int     `json:"reserved"`         // capacity held back for first replies
	Spare          int     `json:"spare"`            // capacity left for second replies this month
	DailySecondCap int     `json:"daily_second_cap"` // second replies allowed today
}

// SecondReplyPacing decides whether one more second reply is affordable right now.
//
// It projects the rest of the month's first-reply demand from the busiest of three views of
// recent history (last 3 days, last 7 days, month to date, plus last month early on), so a
// sudden barrage of messages after an app or server problem raises the projection within a
// day and stops second replies before first replies are at risk. Whatever is left after that
// reserve is spread evenly over the remaining days so it is not all spent at once.
func SecondReplyPacing(in PacingInput) PacingDecision {
	d := PacingDecision{}
	if in.Limit <= 0 || !in.MonthEnd.After(in.MonthStart) {
		d.Reason = "Pacing unavailable: invalid billing month"
		return d
	}
	remaining := in.MonthEnd.Sub(in.Now).Hours() / 24
	d.DaysRemaining = math.Max(remaining, pacingMinDays)

	rate := 0.0
	if avg := tailAverage(in.BaselineDaily, 3); avg > rate {
		rate = avg
	}
	if avg := tailAverage(in.BaselineDaily, 7); avg > rate {
		rate = avg
	}
	elapsed := in.Now.Sub(in.MonthStart).Hours() / 24
	if elapsed >= 1 {
		if mtd := float64(in.BaselineMonthToDate) / elapsed; mtd > rate {
			rate = mtd
		}
	}
	// With under three days of this month's history, last month is the steadier guide.
	if elapsed < 3 && in.PreviousMonthDailyAverage > rate {
		rate = in.PreviousMonthDailyAverage
	}
	d.DailyRate = math.Round(rate*10) / 10
	d.Reserved = int(math.Ceil(rate*d.DaysRemaining*PacingSurgeFactor)) + PacingReserve
	d.Spare = in.Limit - in.Used - d.Reserved
	if d.Spare <= 0 {
		d.Reason = fmt.Sprintf("Second reply held: %d of %d used and about %d needed for first replies this month", in.Used, in.Limit, d.Reserved)
		return d
	}
	d.DailySecondCap = int(math.Floor(float64(d.Spare) / math.Ceil(d.DaysRemaining)))
	if d.DailySecondCap < 1 {
		d.DailySecondCap = 1
	}
	if in.SecondRepliesToday >= d.DailySecondCap {
		d.Reason = fmt.Sprintf("Second reply held: today's share of spare quota (%d) is used", d.DailySecondCap)
		return d
	}
	d.Allowed = true
	d.Reason = fmt.Sprintf("Second reply allowed: %d spare after reserving %d for first replies", d.Spare, d.Reserved)
	return d
}

// tailAverage averages the last n days. Today's partial day counts as a full day only when it
// already exceeds the average of the earlier days, so a quiet morning never lowers the estimate
// while a busy one raises it immediately.
func tailAverage(days []int, n int) float64 {
	if len(days) == 0 || n <= 0 {
		return 0
	}
	today := days[len(days)-1]
	past := days[:len(days)-1]
	if len(past) > n {
		past = past[len(past)-n:]
	}
	sum := 0
	for _, v := range past {
		sum += v
	}
	if len(past) == 0 {
		return float64(today)
	}
	avg := float64(sum) / float64(len(past))
	if float64(today) > avg {
		// Replace the oldest day with today so a surge shows up within hours.
		if len(past) == n {
			sum -= past[0]
			return float64(sum+today) / float64(n)
		}
		return float64(sum+today) / float64(len(past)+1)
	}
	return avg
}
