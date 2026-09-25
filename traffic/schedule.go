package traffic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/O6lvl4/archgopher/model"
)

// daysPerMonth is the days in a 730-hour month.
const daysPerMonth = float64(model.HoursPerMonth) / 24

// Fires is what a schedule does: how often it fires in a month and the
// shortest gap between two fires, which sets the peak.
type Fires struct {
	PerMonth   float64
	GapSeconds float64
}

// Parse reads a schedule expression: EventBridge rate() and cron() (six
// fields with a year), five-field Unix cron (Cloud Scheduler, Kubernetes)
// and six-field cron with seconds first (Azure Functions timers).
func Parse(expr string) (Fires, error) {
	expr = strings.TrimSpace(expr)
	switch {
	case expr == "":
		return Fires{}, fmt.Errorf("no schedule expression")
	case strings.HasPrefix(expr, "rate(") && strings.HasSuffix(expr, ")"):
		return rate(expr, expr[5:len(expr)-1])
	case strings.HasPrefix(expr, "cron(") && strings.HasSuffix(expr, ")"):
		f := strings.Fields(expr[5 : len(expr)-1])
		if len(f) != 6 {
			return Fires{}, fmt.Errorf("cannot read %q: EventBridge cron has 6 fields", expr)
		}
		return cron(cronFields{min: f[0], hour: f[1], dom: f[2], month: f[3], dow: f[4]}), nil
	}
	f := strings.Fields(expr)
	switch len(f) {
	case 5:
		return cron(cronFields{min: f[0], hour: f[1], dom: f[2], month: f[3], dow: f[4]}), nil
	case 6:
		return cron(cronFields{sec: f[0], min: f[1], hour: f[2], dom: f[3], month: f[4], dow: f[5]}), nil
	}
	return Fires{}, fmt.Errorf("cannot read schedule %q", expr)
}

func rate(expr, body string) (Fires, error) {
	f := strings.Fields(body)
	if len(f) != 2 {
		return Fires{}, fmt.Errorf("cannot read %q", expr)
	}
	n, err := strconv.ParseFloat(f[0], 64)
	if err != nil || n <= 0 {
		return Fires{}, fmt.Errorf("cannot read %q", expr)
	}
	unit := map[string]float64{"minute": 60, "minutes": 60, "hour": 3600, "hours": 3600, "day": 86400, "days": 86400}[f[1]]
	if unit == 0 {
		return Fires{}, fmt.Errorf("unknown unit in %q", expr)
	}
	gap := n * unit
	return Fires{PerMonth: model.SecondsPerMonth / gap, GapSeconds: gap}, nil
}

type cronFields struct{ sec, min, hour, dom, month, dow string }

func restricted(f string) bool { return f != "" && f != "*" && f != "?" }

func cron(f cronFields) Fires {
	secs := 1.0
	if restricted(f.sec) || f.sec == "*" {
		secs = count(f.sec, 0, 59)
	}
	mins, hours := count(f.min, 0, 59), count(f.hour, 0, 23)
	var days float64
	byDate := count(f.dom, 1, 31)
	byWeekday := daysPerMonth * count(f.dow, 1, 7) / 7
	switch {
	case restricted(f.dom) && restricted(f.dow):
		// Unix cron fires when either matches.
		days = byDate + byWeekday - byDate*byWeekday/daysPerMonth
	case restricted(f.dom):
		days = byDate
	case restricted(f.dow):
		days = byWeekday
	default:
		days = daysPerMonth
	}
	perMonth := secs * mins * hours * days * count(f.month, 1, 12) / 12
	// The fires of the finest field that fires more than once are the closest.
	gap := 86400.0
	switch {
	case secs > 1:
		gap = 60 / secs
	case mins > 1:
		gap = 3600 / mins
	case hours > 1:
		gap = 86400 / hours
	}
	return Fires{PerMonth: perMonth, GapSeconds: gap}
}

var names = strings.NewReplacer(
	"JAN", "1", "FEB", "2", "MAR", "3", "APR", "4", "MAY", "5", "JUN", "6",
	"JUL", "7", "AUG", "8", "SEP", "9", "OCT", "10", "NOV", "11", "DEC", "12",
	"SUN", "1", "MON", "2", "TUE", "3", "WED", "4", "THU", "5", "FRI", "6", "SAT", "7",
)

// count estimates how many values a cron field selects in [lo, hi].
func count(field string, lo, hi int) float64 {
	field = names.Replace(strings.ToUpper(field))
	total := 0.0
	for _, part := range strings.Split(field, ",") {
		base, stepStr, stepped := strings.Cut(part, "/")
		step := 1.0
		if stepped {
			if s, err := strconv.ParseFloat(stepStr, 64); err == nil && s > 0 {
				step = s
			}
		}
		from, to := float64(lo), float64(hi)
		switch {
		case base == "*" || base == "?":
		case strings.Contains(base, "-"):
			a, b, _ := strings.Cut(base, "-")
			x, e1 := strconv.Atoi(a)
			y, e2 := strconv.Atoi(b)
			if e1 != nil || e2 != nil {
				total++
				continue
			}
			from, to = float64(x), float64(y)
		default:
			if !stepped {
				total++ // a single value, or L / W / # forms
				continue
			}
			if x, err := strconv.Atoi(base); err == nil {
				from = float64(x)
			}
		}
		if to >= from {
			total += float64(int((to-from)/step)) + 1
		}
	}
	return total
}
