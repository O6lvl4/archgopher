// Package traffic turns the ways people describe load (a rate, users and
// what each does, users at work at the same time, a schedule, batches) into
// the Load the engine pushes through the graph: a monthly volume and a peak
// rate. Each result comes with the arithmetic that produced it, so a reader
// can check the numbers against their own sense of the system.
package traffic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/O6lvl4/archgopher/model"
)

// Reading is a resolved Traffic: the load and how it was worked out.
type Reading struct {
	Load  model.Load
	Basis string
}

// window is when traffic comes: days a month and hours a day.
type window struct {
	days, hours float64
	label       string
}

func (w window) seconds() float64 { return w.days * w.hours * 3600 }

// Resolve turns a Traffic into a Load.
func Resolve(t model.Traffic) (Reading, error) {
	shapes := 0
	for _, set := range []bool{t.Rate != nil, t.Users != nil, t.Concurrent != nil, t.Schedule != "", t.Batch != nil} {
		if set {
			shapes++
		}
	}
	if shapes != 1 {
		return Reading{}, fmt.Errorf("traffic needs exactly one of rate, users, concurrent, schedule and batch, got %d", shapes)
	}
	switch {
	case t.Schedule != "":
		return scheduled(t)
	case t.Batch != nil:
		return batched(t)
	}
	w, err := windowOf(t.Hours, t.Days)
	if err != nil {
		return Reading{}, err
	}
	var v volume
	switch {
	case t.Rate != nil:
		v, err = rated(*t.Rate, w)
	case t.Users != nil:
		v, err = used(*t.Users, w)
	default:
		v, err = concurrent(*t.Concurrent, w)
	}
	if err != nil {
		return Reading{}, err
	}
	return peaked(t, w, v)
}

// volume is the monthly volume of traffic that comes within a window, the
// factor its peak has over the average, and the arithmetic.
type volume struct {
	monthly, factor float64
	basis           string
}

// peaked adds the peak: the average over the active hours times a factor,
// unless the peak is given.
func peaked(t model.Traffic, w window, v volume) (Reading, error) {
	monthly, factor, basis := v.monthly, v.factor, v.basis
	if t.PeakFactor != nil {
		if *t.PeakFactor < 1 {
			return Reading{}, fmt.Errorf("peakFactor must be at least 1")
		}
		factor = *t.PeakFactor
	}
	avg := monthly / w.seconds()
	if t.PeakPerSecond != nil {
		return Reading{Load: model.Load{Monthly: monthly, PeakPerSecond: *t.PeakPerSecond},
			Basis: basis + fmt.Sprintf("; %s/s on average over %s, peak set to %s/s", num(avg), w.label, num(*t.PeakPerSecond))}, nil
	}
	peak := avg * factor
	how := fmt.Sprintf("; %s/s on average over %s", num(avg), w.label)
	if factor != 1 {
		how += fmt.Sprintf(", ×%s for peaks = %s/s", num(factor), num(peak))
	}
	return Reading{Load: model.Load{Monthly: monthly, PeakPerSecond: peak}, Basis: basis + how}, nil
}

var subDay = map[string]float64{"second": 1, "minute": 60, "hour": 3600}

// rated: per second, minute or hour is the rate while active; per day, week
// or month is a total spread over the active hours, which peaks.
func rated(r model.Rate, w window) (volume, error) {
	if r.Count < 0 {
		return volume{}, fmt.Errorf("rate count must not be negative")
	}
	if s, ok := subDay[r.Per]; ok {
		monthly := r.Count / s * w.seconds()
		return volume{monthly, 1, fmt.Sprintf("%s a %s while active × %s = %s a month", num(r.Count), r.Per, w.describe(), num(monthly))}, nil
	}
	periods, label, err := periodsPerMonth(r.Per, w)
	if err != nil {
		return volume{}, err
	}
	monthly := r.Count * periods
	return volume{monthly, 2, fmt.Sprintf("%s a %s × %s %s = %s a month", num(r.Count), r.Per, num(periods), label, num(monthly))}, nil
}

func used(u model.Users, w window) (volume, error) {
	if u.Count < 0 || u.Actions < 0 {
		return volume{}, fmt.Errorf("users and actions must not be negative")
	}
	periods, label, err := periodsPerMonth(u.Per, w)
	if err != nil {
		return volume{}, err
	}
	monthly := u.Count * u.Actions * periods
	return volume{monthly, 2, fmt.Sprintf("%s users × %s a %s × %s %s = %s a month", num(u.Count), num(u.Actions), u.Per, num(periods), label, num(monthly))}, nil
}

// concurrent: a closed model. The rate is users over the time between one
// person's actions, and never peaks above it.
func concurrent(c model.Concurrent, w window) (volume, error) {
	if c.Users < 0 || c.EverySeconds <= 0 {
		return volume{}, fmt.Errorf("concurrent needs users and everySeconds above 0")
	}
	rate := c.Users / c.EverySeconds
	monthly := rate * w.seconds()
	return volume{monthly, 1, fmt.Sprintf("%s users at a time, one action each every %s s = %s/s × %s = %s a month",
		num(c.Users), num(c.EverySeconds), num(rate), w.describe(), num(monthly))}, nil
}

func scheduled(t model.Traffic) (Reading, error) {
	if t.Hours != "" || t.Days != "" {
		return Reading{}, fmt.Errorf("a schedule says when it runs itself: drop hours and days")
	}
	f, err := Parse(t.Schedule)
	if err != nil {
		return Reading{}, err
	}
	load := model.Load{Monthly: f.PerMonth, PeakPerSecond: 1 / f.GapSeconds}
	if t.PeakPerSecond != nil {
		load.PeakPerSecond = *t.PeakPerSecond
	}
	return Reading{Load: load, Basis: fmt.Sprintf("%s fires %s times a month, at least %s apart", t.Schedule, num(f.PerMonth), duration(f.GapSeconds))}, nil
}

// batched: every run brings Items at once, worked through within
// WithinSeconds; the peak is that rate.
func batched(t model.Traffic) (Reading, error) {
	b := *t.Batch
	if b.Items < 0 || b.WithinSeconds <= 0 {
		return Reading{}, fmt.Errorf("batch needs items and withinSeconds above 0")
	}
	f, err := Parse(b.Every)
	if err != nil {
		return Reading{}, fmt.Errorf("batch every: %w", err)
	}
	load := model.Load{Monthly: b.Items * f.PerMonth, PeakPerSecond: b.Items / b.WithinSeconds}
	if t.PeakPerSecond != nil {
		load.PeakPerSecond = *t.PeakPerSecond
	}
	return Reading{Load: load, Basis: fmt.Sprintf("%s items × %s runs a month (%s) = %s a month; each run within %s = %s/s",
		num(b.Items), num(f.PerMonth), b.Every, num(load.Monthly), duration(b.WithinSeconds), num(b.Items/b.WithinSeconds))}, nil
}

// periodsPerMonth: a day counts only the active days; a week and a month are calendar.
func periodsPerMonth(per string, w window) (float64, string, error) {
	switch per {
	case "day":
		return w.days, w.dayLabel(), nil
	case "week":
		return float64(model.HoursPerMonth) / (7 * 24), "weeks", nil
	case "month":
		return 1, "month", nil
	}
	return 0, "", fmt.Errorf("per must be second, minute, hour, day, week or month, not %q", per)
}

func windowOf(hours, days string) (window, error) {
	w := window{days: daysPerMonth, hours: 24}
	switch days {
	case "", "all":
	case "weekdays":
		w.days = daysPerMonth * 5 / 7
	case "weekends":
		w.days = daysPerMonth * 2 / 7
	default:
		return window{}, fmt.Errorf("days must be all, weekdays or weekends, not %q", days)
	}
	if hours != "" {
		h, err := hoursOf(hours)
		if err != nil {
			return window{}, err
		}
		w.hours = h
	}
	w.label = fmt.Sprintf("%s h a day", num(w.hours))
	if days == "weekdays" || days == "weekends" {
		w.label += " on " + days
	}
	return w, nil
}

func (w window) dayLabel() string {
	if w.days == daysPerMonth {
		return "days"
	}
	if w.days < daysPerMonth/2 {
		return "weekend days"
	}
	return "weekdays"
}

func (w window) describe() string {
	return fmt.Sprintf("%s h × %s %s", num(w.hours), num(w.days), w.dayLabel())
}

// hoursOf reads "9-18" or "9-12,13-18" (a range may wrap midnight: "22-6").
func hoursOf(s string) (float64, error) {
	total := 0.0
	for _, part := range strings.Split(s, ",") {
		span, ok := hourSpan(part)
		if !ok {
			return 0, fmt.Errorf("hours must be ranges like 9-18, not %q", s)
		}
		total += span
	}
	if total > 24 {
		return 0, fmt.Errorf("hours %q add up to more than a day", s)
	}
	return total, nil
}

// hourSpan is the hours in one range "9-18"; a range that ends where or
// before it starts wraps midnight.
func hourSpan(part string) (float64, bool) {
	a, b, ok := strings.Cut(strings.TrimSpace(part), "-")
	from, e1 := strconv.ParseFloat(strings.TrimSpace(a), 64)
	to, e2 := strconv.ParseFloat(strings.TrimSpace(b), 64)
	if !ok || e1 != nil || e2 != nil || outsideDay(from) || outsideDay(to) {
		return 0, false
	}
	span := to - from
	if span <= 0 {
		span += 24
	}
	return span, true
}

func outsideDay(h float64) bool { return h < 0 || h > 24 }

func duration(s float64) string {
	switch {
	case s >= 86400 && int(s)%86400 == 0:
		return num(s/86400) + " d"
	case s >= 3600 && int(s)%3600 == 0:
		return num(s/3600) + " h"
	case s >= 60 && int(s)%60 == 0:
		return num(s/60) + " min"
	}
	return num(s) + " s"
}

// num writes a number short: 3 significant digits below 1,000, grouped above.
func num(v float64) string {
	switch {
	case v == 0:
		return "0"
	case v >= 1000:
		s := strconv.FormatFloat(v, 'f', 0, 64)
		for i := len(s) - 3; i > 0; i -= 3 {
			s = s[:i] + "," + s[i:]
		}
		return s
	}
	return strconv.FormatFloat(v, 'g', 3, 64)
}
