package support

import (
	"testing"
	"time"
)

func month(y int, m time.Month) (time.Time, time.Time) {
	start := time.Date(y, m, 1, 0, 0, 0, 0, PKT)
	return start, start.AddDate(0, 1, 0)
}

func TestSecondReplyPacingNormalMonth(t *testing.T) {
	// November 2026 as observed: 224 sends by the 19th, typical days 5-20.
	start, end := month(2026, time.November)
	d := SecondReplyPacing(PacingInput{
		Now: time.Date(2026, 11, 19, 12, 0, 0, 0, PKT), MonthStart: start, MonthEnd: end,
		Limit: 1000, Used: 224, BaselineMonthToDate: 220,
		BaselineDaily:             []int{6, 7, 12, 14, 7, 12, 14, 9},
		PreviousMonthDailyAverage: 23,
	})
	if !d.Allowed {
		t.Fatalf("normal month should allow second replies: %+v", d)
	}
	if d.Spare < 300 || d.DailySecondCap < 20 {
		t.Fatalf("expected generous spare in a quiet month, got %+v", d)
	}
}

func TestSecondReplyPacingSurgeStopsSecondReplies(t *testing.T) {
	// An incident on the 10th: 70+ first replies a day. Second replies must stop.
	start, end := month(2026, time.October)
	d := SecondReplyPacing(PacingInput{
		Now: time.Date(2026, 10, 10, 15, 0, 0, 0, PKT), MonthStart: start, MonthEnd: end,
		Limit: 1000, Used: 420, BaselineMonthToDate: 400,
		BaselineDaily: []int{20, 22, 25, 30, 72, 77, 71},
	})
	if d.Allowed {
		t.Fatalf("surge should hold second replies: %+v", d)
	}
}

func TestSecondReplyPacingSurgeTodayRaisesProjectionImmediately(t *testing.T) {
	start, end := month(2026, time.November)
	quiet := PacingInput{
		Now: time.Date(2026, 11, 20, 14, 0, 0, 0, PKT), MonthStart: start, MonthEnd: end,
		Limit: 1000, Used: 600, BaselineMonthToDate: 560,
		BaselineDaily: []int{15, 15, 15, 15, 15, 15, 15, 10},
	}
	surge := quiet
	surge.BaselineDaily = []int{15, 15, 15, 15, 15, 15, 15, 120}
	q, s := SecondReplyPacing(quiet), SecondReplyPacing(surge)
	if s.DailyRate <= q.DailyRate || s.Reserved <= q.Reserved {
		t.Fatalf("a busy day must raise the projection: quiet %+v surge %+v", q, s)
	}
	if s.Allowed {
		t.Fatalf("a 120-message day this late in the month should hold second replies: %+v", s)
	}
}

func TestSecondReplyPacingEarlyMonthUsesPreviousMonth(t *testing.T) {
	start, end := month(2026, time.December)
	d := SecondReplyPacing(PacingInput{
		Now: time.Date(2026, 12, 1, 10, 0, 0, 0, PKT), MonthStart: start, MonthEnd: end,
		Limit: 1000, Used: 3, BaselineMonthToDate: 3,
		BaselineDaily:             []int{12, 14, 17, 3},
		PreviousMonthDailyAverage: 35,
	})
	if d.DailyRate < 35 {
		t.Fatalf("first days of the month should lean on last month's rate, got %+v", d)
	}
}

func TestSecondReplyPacingDailyCapSpreadsSpare(t *testing.T) {
	start, end := month(2026, time.November)
	in := PacingInput{
		Now: time.Date(2026, 11, 25, 12, 0, 0, 0, PKT), MonthStart: start, MonthEnd: end,
		Limit: 1000, Used: 700, BaselineMonthToDate: 600,
		BaselineDaily: []int{20, 20, 20, 20, 20, 20, 20, 10},
	}
	d := SecondReplyPacing(in)
	if !d.Allowed || d.DailySecondCap <= 0 {
		t.Fatalf("expected an allowance: %+v", d)
	}
	in.SecondRepliesToday = d.DailySecondCap
	if held := SecondReplyPacing(in); held.Allowed {
		t.Fatalf("today's share is used, must hold: %+v", held)
	}
}

func TestSecondReplyPacingNeverExceedsLimit(t *testing.T) {
	start, end := month(2026, time.November)
	d := SecondReplyPacing(PacingInput{
		Now: time.Date(2026, 11, 30, 23, 0, 0, 0, PKT), MonthStart: start, MonthEnd: end,
		Limit: 1000, Used: 990, BaselineDaily: []int{0, 0, 0},
	})
	if d.Allowed {
		t.Fatalf("the fixed reserve must protect the last sends of the month: %+v", d)
	}
}

func TestTailAverage(t *testing.T) {
	if got := tailAverage([]int{10, 10, 10, 2}, 3); got != 10 {
		t.Fatalf("quiet partial day must not lower the average, got %v", got)
	}
	if got := tailAverage([]int{10, 10, 10, 40}, 3); got != 20 {
		t.Fatalf("busy partial day must raise the average, got %v", got)
	}
	if got := tailAverage(nil, 3); got != 0 {
		t.Fatalf("empty history, got %v", got)
	}
}
