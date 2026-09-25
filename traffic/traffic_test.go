package traffic

import (
	"math"
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/model"
)

// A 730-hour month has 30.4167 days; weekdays are 5/7 of them (21.7262).
const (
	days     = 730.0 / 24
	weekdays = days * 5 / 7
)

func near(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6*math.Max(1, math.Abs(want)) {
		t.Errorf("%s: got %v, want %v", what, got, want)
	}
}

func f(v float64) *float64 { return &v }

func TestShapes(t *testing.T) {
	cases := []struct {
		name          string
		traffic       model.Traffic
		monthly, peak float64
	}{
		// 5,000 users × 20 a day on 21.73 weekdays, over 9 hours: 3.09/s on average, ×2.
		{"users on weekdays in office hours",
			model.Traffic{Users: &model.Users{Count: 5000, Actions: 20, Per: "day"}, Hours: "9-18", Days: "weekdays"},
			5000 * 20 * weekdays, 5000 * 20 / (9 * 3600.0) * 2},
		{"users a month",
			model.Traffic{Users: &model.Users{Count: 1000, Actions: 30, Per: "month"}},
			30000, 30000 / (days * 86400) * 2},
		{"a daily total, all day",
			model.Traffic{Rate: &model.Rate{Count: 20000, Per: "day"}},
			20000 * days, 20000 / 86400.0 * 2},
		{"a weekly total",
			model.Traffic{Rate: &model.Rate{Count: 7000, Per: "week"}},
			7000 * 730.0 / 168, 7000 * 730.0 / 168 / (days * 86400) * 2},
		// A rate per second is the rate while active: no peak factor.
		{"a rate while active",
			model.Traffic{Rate: &model.Rate{Count: 3, Per: "second"}, Hours: "9-12,13-18", Days: "weekdays"},
			3 * 8 * 3600 * weekdays, 3},
		{"a rate per minute with a peak factor",
			model.Traffic{Rate: &model.Rate{Count: 120, Per: "minute"}, PeakFactor: f(1.5)},
			2 * days * 86400, 3},
		// 50 people, one action every 30 s: 1.67/s, never more.
		{"concurrent users",
			model.Traffic{Concurrent: &model.Concurrent{Users: 50, EverySeconds: 30}, Hours: "9-18", Days: "weekdays"},
			50.0 / 30 * 9 * 3600 * weekdays, 50.0 / 30},
		{"a night window across midnight",
			model.Traffic{Rate: &model.Rate{Count: 1, Per: "second"}, Hours: "22-6"},
			8 * 3600 * days, 1},
		{"a given peak",
			model.Traffic{Rate: &model.Rate{Count: 1000, Per: "day"}, PeakPerSecond: f(40)},
			1000 * days, 40},
		{"a schedule",
			model.Traffic{Schedule: "rate(1 hour)"},
			730, 1.0 / 3600},
		// 10,000 items every 6 hours, each batch within 10 minutes.
		{"batches",
			model.Traffic{Batch: &model.Batch{Items: 10000, Every: "rate(6 hours)", WithinSeconds: 600}},
			10000 * 730.0 / 6, 10000.0 / 600},
	}
	for _, c := range cases {
		r, err := Resolve(c.traffic)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		near(t, c.name+" monthly", r.Load.Monthly, c.monthly)
		near(t, c.name+" peak", r.Load.PeakPerSecond, c.peak)
		if r.Basis == "" {
			t.Errorf("%s: no basis", c.name)
		}
	}
}

func TestBasisShowsTheArithmetic(t *testing.T) {
	r, err := Resolve(model.Traffic{Users: &model.Users{Count: 5000, Actions: 20, Per: "day"}, Hours: "9-18", Days: "weekdays"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"5,000 users", "× 20 a day", "21.7 weekdays", "2,172,619 a month", "9 h a day on weekdays", "×2 for peaks"} {
		if !strings.Contains(r.Basis, want) {
			t.Errorf("basis %q lacks %q", r.Basis, want)
		}
	}
}

func TestMistakesAreErrors(t *testing.T) {
	cases := map[string]model.Traffic{
		"no shape":              {},
		"two shapes":            {Rate: &model.Rate{Count: 1, Per: "day"}, Schedule: "rate(1 hour)"},
		"unknown period":        {Rate: &model.Rate{Count: 1, Per: "fortnight"}},
		"bad hours":             {Rate: &model.Rate{Count: 1, Per: "day"}, Hours: "nine to six"},
		"too many hours":        {Rate: &model.Rate{Count: 1, Per: "day"}, Hours: "0-20,1-10"},
		"unknown days":          {Rate: &model.Rate{Count: 1, Per: "day"}, Days: "holidays"},
		"hours on a schedule":   {Schedule: "rate(1 hour)", Hours: "9-18"},
		"a peak factor below 1": {Rate: &model.Rate{Count: 1, Per: "day"}, PeakFactor: f(0.5)},
		"no think time":         {Concurrent: &model.Concurrent{Users: 5}},
		"a batch with no time":  {Batch: &model.Batch{Items: 5, Every: "rate(1 hour)"}},
	}
	for name, tr := range cases {
		if _, err := Resolve(tr); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestSchedules(t *testing.T) {
	cases := []struct {
		expr          string
		perMonth, gap float64
	}{
		{"rate(5 minutes)", model.SecondsPerMonth / 300, 300},
		{"rate(1 day)", days, 86400},
		{"cron(0 9 * * ? *)", days, 86400},
		{"cron(0/15 * * * ? *)", 4 * 24 * days, 900},
		{"cron(0 9 ? * MON-FRI *)", weekdays, 86400},
		{"cron(0 9 1 * ? *)", 1, 86400},
		// Unix cron (Cloud Scheduler): every 15 minutes, 9:00 to 17:45, Monday to Friday.
		{"*/15 9-17 * * 1-5", 4 * 9 * weekdays, 900},
		{"0 3 * * *", days, 86400},
		// Azure timers put seconds first: every 5 minutes.
		{"0 */5 * * * *", 12 * 24 * days, 300},
		{"*/30 * * * * *", 2 * 60 * 24 * days, 30},
	}
	for _, c := range cases {
		got, err := Parse(c.expr)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		near(t, c.expr+" per month", got.PerMonth, c.perMonth)
		near(t, c.expr+" gap", got.GapSeconds, c.gap)
	}
	for _, bad := range []string{"", "at(2026-01-01T00:00:00)", "rate(5 fortnights)", "cron(0 9 * *)", "every day"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}
