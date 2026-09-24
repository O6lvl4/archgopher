package definition

import (
	"fmt"
	"sort"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/O6lvl4/arch-scouter/facet"
	"github.com/O6lvl4/arch-scouter/meter"
)

// Params are one reading's parameters as written in YAML.
type Params map[string]any

type pkind int

const (
	num  pkind = iota // a number expression
	opt               // a number expression that may be nil
	txt               // text with {expression} parts
	flag              // a literal boolean
)

type param struct {
	kind     pkind
	required bool
}

var (
	reqNum = param{num, true}
	optNum = param{num, false}
	nilNum = param{opt, false}
	reqTxt = param{txt, true}
	optTxt = param{txt, false}
)

// kinds lists every reading a definition can use and the parameters it takes.
// Each maps onto one facet (L2) or one meter call (L1).
var kinds = map[string]map[string]param{
	"cost":        {"name": reqTxt, "quantity": reqNum, "unit": reqTxt, "price": reqTxt},
	"requests":    {"name": reqTxt, "count": reqNum, "price": reqTxt, "unit": optTxt, "chunkKb": optNum, "sizeKb": optNum},
	"rate":        {"name": reqTxt, "peak": reqNum, "unit": reqTxt, "quota": reqTxt, "scale": optNum},
	"compute":     {"name": reqTxt, "count": reqNum, "seconds": reqNum, "memoryMb": reqNum, "price": reqTxt, "stepMb": optNum},
	"concurrency": {"name": reqTxt, "peak": reqNum, "seconds": reqNum, "unit": reqTxt, "quota": reqTxt, "capacity": nilNum},
	"storage":     {"name": reqTxt, "gb": reqNum, "price": reqTxt},
	"capacity":    {"name": reqTxt, "units": reqNum, "unit": reqTxt, "price": reqTxt},
	"limit":       {"name": reqTxt, "demand": reqNum, "unit": reqTxt, "quota": optTxt, "capacity": nilNum},
	"logs":        {"count": reqNum, "kb": reqNum, "retentionDays": reqNum, "ingest": reqTxt, "storage": reqTxt},
	"tokens":      {"prefix": reqTxt, "monthly": reqNum, "peak": reqNum, "input": reqNum, "output": reqNum, "cacheRead": optNum, "cacheWrite": optNum},
	"session":     {"count": reqNum, "peak": reqNum, "sessionSeconds": reqNum, "activeVcpuSeconds": reqNum, "memoryGb": reqNum, "vcpuPrice": reqTxt, "memoryPrice": reqTxt, "concurrentQuota": reqTxt},
	"fail":        {"message": reqTxt, "continue": {flag, false}},
}

// Kinds lists the reading kinds, for documentation and errors.
func Kinds() []string {
	out := make([]string, 0, len(kinds))
	for k := range kinds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type reading struct {
	kind  string
	when  *vm.Program
	nums  map[string]*vm.Program
	texts map[string]text
	flags map[string]bool
}

func compileReading(raw map[string]Params, decl map[string]any) (reading, error) {
	if len(raw) != 1 {
		return reading{}, fmt.Errorf("a reading has exactly one kind (%s)", strings.Join(Kinds(), ", "))
	}
	var rd reading
	for kind, params := range raw {
		spec, ok := kinds[kind]
		if !ok {
			return rd, fmt.Errorf("unknown reading %q (want one of %s)", kind, strings.Join(Kinds(), ", "))
		}
		rd = reading{kind: kind, nums: map[string]*vm.Program{}, texts: map[string]text{}, flags: map[string]bool{}}
		if w, ok := params["when"]; ok {
			p, err := compile(fmt.Sprint(w), decl)
			if err != nil {
				return rd, fmt.Errorf("%s when: %w", kind, err)
			}
			rd.when = p
		}
		if err := rd.compileParams(kind, spec, params, decl); err != nil {
			return rd, err
		}
	}
	return rd, nil
}

func (rd *reading) compileParams(kind string, spec map[string]param, params Params, decl map[string]any) error {
	for name := range params {
		if _, ok := spec[name]; !ok && name != "when" {
			return fmt.Errorf("%s: unknown parameter %q", kind, name)
		}
	}
	for name, p := range spec {
		v, set := params[name]
		if !set {
			if p.required {
				return fmt.Errorf("%s: missing parameter %q", kind, name)
			}
			continue
		}
		switch p.kind {
		case num, opt:
			prog, err := compile(fmt.Sprint(v), decl)
			if err != nil {
				return fmt.Errorf("%s %s: %w", kind, name, err)
			}
			rd.nums[name] = prog
		case txt:
			t, err := compileText(fmt.Sprint(v), decl)
			if err != nil {
				return fmt.Errorf("%s %s: %w", kind, name, err)
			}
			rd.texts[name] = t
		case flag:
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("%s %s: want true or false", kind, name)
			}
			rd.flags[name] = b
		}
	}
	return nil
}

// values evaluates every parameter for one node.
type values struct {
	nums  map[string]any
	texts map[string]string
}

func (v values) n(name string) float64 { return toFloat(v.nums[name]) }

func (v values) optional(name string) *float64 {
	x := deref(v.nums[name])
	if x == nil {
		return nil
	}
	f := toFloat(x)
	return &f
}

func (v values) has(name string) bool { _, ok := v.nums[name]; return ok }

func (rd reading) eval(env map[string]any) (values, error) {
	out := values{nums: map[string]any{}, texts: map[string]string{}}
	for name, p := range rd.nums {
		x, err := expr.Run(p, env)
		if err != nil {
			return out, fmt.Errorf("%s %s: %w", rd.kind, name, err)
		}
		out.nums[name] = deref(x)
	}
	for name, t := range rd.texts {
		s, err := t.eval(env)
		if err != nil {
			return out, fmt.Errorf("%s %s: %w", rd.kind, name, err)
		}
		out.texts[name] = s
	}
	return out, nil
}

// run records the reading; stop is true when a fail reading ends the node.
func (rd reading) run(env map[string]any, r *meter.Recorder) (stop bool, err error) {
	if rd.when != nil {
		ok, err := expr.Run(rd.when, env)
		if err != nil {
			return false, fmt.Errorf("%s when: %w", rd.kind, err)
		}
		if b, _ := deref(ok).(bool); !b {
			return false, nil
		}
	}
	v, err := rd.eval(env)
	if err != nil {
		return false, err
	}
	if rd.kind == "fail" {
		r.Fail("%s", v.texts["message"])
		return !rd.flags["continue"], nil
	}
	record[rd.kind](v, r)
	return false, nil
}

var record = map[string]func(v values, r *meter.Recorder){
	"cost": func(v values, r *meter.Recorder) {
		r.Cost(v.texts["name"], v.n("quantity"), v.texts["unit"], v.texts["price"])
	},
	"requests": func(v values, r *meter.Recorder) {
		unit := v.texts["unit"]
		if unit == "" {
			unit = "request"
		}
		facet.Requests{Name: v.texts["name"], Unit: unit, PriceID: v.texts["price"], ChunkKB: v.n("chunkKb")}.Read(r, v.n("count"), v.n("sizeKb"))
	},
	"rate": func(v values, r *meter.Recorder) {
		f := facet.Rate{Name: v.texts["name"], Unit: v.texts["unit"], QuotaID: v.texts["quota"]}
		if v.has("scale") {
			f.ReadScaled(r, v.n("peak"), v.n("scale"))
			return
		}
		f.Read(r, v.n("peak"))
	},
	"compute": func(v values, r *meter.Recorder) {
		facet.Compute{Name: v.texts["name"], PriceID: v.texts["price"], StepMB: v.n("stepMb")}.Read(r, v.n("count"), v.n("seconds"), v.n("memoryMb"))
	},
	"concurrency": func(v values, r *meter.Recorder) {
		facet.Concurrency{Name: v.texts["name"], Unit: v.texts["unit"], QuotaID: v.texts["quota"]}.Read(r, v.n("peak"), v.n("seconds"), v.optional("capacity"))
	},
	"storage": func(v values, r *meter.Recorder) {
		facet.Storage{Name: v.texts["name"], PriceID: v.texts["price"]}.Read(r, v.n("gb"))
	},
	"capacity": func(v values, r *meter.Recorder) {
		facet.Capacity{Name: v.texts["name"], Unit: v.texts["unit"], PriceID: v.texts["price"]}.Read(r, v.n("units"))
	},
	"limit": func(v values, r *meter.Recorder) {
		r.LimitOverride(v.texts["name"], v.n("demand"), v.texts["unit"], v.texts["quota"], v.optional("capacity"))
	},
	"logs": func(v values, r *meter.Recorder) {
		facet.Logs{IngestPriceID: v.texts["ingest"], StoragePriceID: v.texts["storage"]}.Read(r, v.n("count"), facet.LogAssume{LogKb: v.n("kb"), LogRetentionDays: v.n("retentionDays")})
	},
	"session": func(v values, r *meter.Recorder) {
		a := facet.SessionAssume{SessionSeconds: v.n("sessionSeconds"), ActiveVcpuSeconds: v.n("activeVcpuSeconds"), PeakMemoryGb: v.n("memoryGb")}
		facet.Session{VcpuPriceID: v.texts["vcpuPrice"], MemoryPriceID: v.texts["memoryPrice"], ConcurrentQuotaID: v.texts["concurrentQuota"]}.Read(r, v.n("count"), v.n("peak"), a)
	},
	"tokens": func(v values, r *meter.Recorder) {
		a := facet.TokenAssume{InputTokens: v.n("input"), OutputTokens: v.n("output"), CacheReadShare: v.n("cacheRead"), CacheWriteShare: v.n("cacheWrite")}
		facet.Tokens{Prefix: v.texts["prefix"]}.Read(r, v.n("monthly"), v.n("peak"), a)
	},
}
