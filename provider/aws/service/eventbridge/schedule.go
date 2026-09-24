package eventbridge

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/O6lvl4/arch-scouter/model"
)

// Load turns rate() and cron() expressions into a load. The peak rate
// is one fire per interval for rate(), and the average rate for cron().
func Load(expr string) (model.Load, error) {
	expr = strings.TrimSpace(expr)
	switch {
	case strings.HasPrefix(expr, "rate(") && strings.HasSuffix(expr, ")"):
		f := strings.Fields(expr[5 : len(expr)-1])
		if len(f) != 2 {
			return model.Load{}, fmt.Errorf("cannot read %q", expr)
		}
		n, err := strconv.ParseFloat(f[0], 64)
		if err != nil || n <= 0 {
			return model.Load{}, fmt.Errorf("cannot read %q", expr)
		}
		unit := map[string]float64{"minute": 60, "minutes": 60, "hour": 3600, "hours": 3600, "day": 86400, "days": 86400}[f[1]]
		if unit == 0 {
			return model.Load{}, fmt.Errorf("unknown unit in %q", expr)
		}
		per := 1 / (n * unit)
		return model.Load{Monthly: per * model.SecondsPerMonth, PeakPerSecond: per}, nil
	case strings.HasPrefix(expr, "cron(") && strings.HasSuffix(expr, ")"):
		f := strings.Fields(expr[5 : len(expr)-1])
		if len(f) != 6 {
			return model.Load{}, fmt.Errorf("cannot read %q: want 6 fields", expr)
		}
		perDay := count(f[0], 0, 59) * count(f[1], 0, 23)
		days := 30.4
		switch {
		case f[2] != "*" && f[2] != "?":
			days = count(f[2], 1, 31)
		case f[4] != "*" && f[4] != "?":
			days = 30.4 * count(f[4], 1, 7) / 7
		}
		monthly := perDay * days * count(f[3], 1, 12) / 12
		return model.Load{Monthly: monthly, PeakPerSecond: monthly / model.SecondsPerMonth}, nil
	case expr == "":
		return model.Load{}, fmt.Errorf("no schedule expression")
	}
	return model.Load{}, fmt.Errorf("cannot read schedule %q", expr)
}

// count estimates how many values a cron field selects in [lo, hi].
func count(field string, lo, hi int) float64 {
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
		total += math.Floor((to-from)/step) + 1
	}
	return total
}
