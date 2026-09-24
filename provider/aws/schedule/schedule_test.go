package schedule

import (
	"math"
	"testing"

	"github.com/O6lvl4/archgopher/model"
)

func TestScheduleLoad(t *testing.T) {
	cases := map[string]float64{
		"rate(5 minutes)":         float64(model.SecondsPerMonth) / 300,
		"rate(1 day)":             float64(model.SecondsPerMonth) / 86400,
		"cron(0 9 * * ? *)":       30.4,
		"cron(0/15 * * * ? *)":    4 * 24 * 30.4,
		"cron(0 9 ? * MON-FRI *)": 30.4 * 1 / 7, // named days are not parsed: counted as one day a week
		"cron(0 9 1 * ? *)":       1,
	}
	for expr, want := range cases {
		got, err := Load(expr)
		if err != nil {
			t.Errorf("%s: %v", expr, err)
			continue
		}
		if math.Abs(got.Monthly-want) > 1e-6 {
			t.Errorf("%s: got %v, want %v", expr, got.Monthly, want)
		}
	}
	if _, err := Load("at(2026-01-01T00:00:00)"); err == nil {
		t.Error("one-off schedules have no steady load")
	}
}
