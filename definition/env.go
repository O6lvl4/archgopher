package definition

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/O6lvl4/archgopher/field"
	"github.com/O6lvl4/archgopher/model"
)

// Flow is how expressions see a load: demand.read.monthly, total.peak. It
// keeps the load so units() can count its operations by size.
type Flow struct {
	Monthly float64 `expr:"monthly"`
	Peak    float64 `expr:"peak"`
	load    model.Load
}

func flowOf(l model.Load) Flow { return Flow{Monthly: l.Monthly, Peak: l.PeakPerSecond, load: l} }

// sizeOf reads the fallback size: a number, or an optional one that may be unset.
func sizeOf(v any) *float64 {
	switch x := v.(type) {
	case nil:
		return nil
	case *float64:
		return x
	}
	f := toFloat(v)
	return &f
}

// unitsOf counts a flow in billing units of step KB: each operation whose size
// an edge gave is rounded up on its own, the rest is taken as the node's size.
func unitsOf(params []any, peak bool) (any, error) {
	flow, ok := params[0].(Flow)
	if !ok {
		return nil, fmt.Errorf("units: the first argument must be a demand, such as demand.read")
	}
	monthly, p, known := flow.load.Units(toFloat(params[1]), sizeOf(params[2]))
	if !known {
		return nil, fmt.Errorf("the size of some operations is unknown: set it on this node or kb on the edges that bring them")
	}
	if peak {
		return p, nil
	}
	return monthly, nil
}

// declare builds the typed environment expressions compile against, so an
// unknown name or a type mismatch fails when the definition loads.
// reserved names are in every expression: the demand and the spec region.
var reserved = map[string]bool{"total": true, "demand": true, "region": true}

func declare(attrs, assume []field.Field) map[string]any {
	env := map[string]any{"total": Flow{}, "demand": map[string]Flow{}, "region": ""}
	for _, f := range append(append([]field.Field(nil), attrs...), assume...) {
		env[f.Key] = reflect.Zero(typeOf(f)).Interface()
	}
	return env
}

// runtimeEnv fills the environment for one node.
func runtimeEnv(region string, attrs, assume []field.Field, a, p map[string]any, kinds []string, d model.Demand) map[string]any {
	demand := map[string]Flow{}
	for _, k := range kinds {
		demand[k] = flowOf(d.Of(k))
	}
	env := map[string]any{"total": flowOf(d.Total()), "demand": demand, "region": region}
	put := func(fields []field.Field, values map[string]any) {
		for _, f := range fields {
			v, set := values[f.Key]
			t := typeOf(f)
			switch {
			case !set:
				env[f.Key] = reflect.Zero(t).Interface()
			case t.Kind() == reflect.Pointer:
				ptr := reflect.New(t.Elem())
				ptr.Elem().Set(reflect.ValueOf(v))
				env[f.Key] = ptr.Interface()
			default:
				env[f.Key] = v
			}
		}
	}
	put(attrs, a)
	put(assume, p)
	return env
}

var functions = []expr.Option{
	// units(demand.read, 4, itemSizeKb) is the monthly volume in 4 KB units;
	// peakUnits the same at the peak. The last argument is the size of
	// operations no edge gave a size for.
	expr.Function("units", func(params ...any) (any, error) { return unitsOf(params, false) },
		new(func(Flow, float64, any) float64)),
	expr.Function("peakUnits", func(params ...any) (any, error) { return unitsOf(params, true) },
		new(func(Flow, float64, any) float64)),
	// ceilDiv rounds a/b up; billing units (4 KB reads, 64 KB messages) use it. At least 1.
	expr.Function("ceilDiv", func(params ...any) (any, error) {
		a, b := toFloat(params[0]), toFloat(params[1])
		if a <= 0 {
			return 1.0, nil
		}
		return math.Ceil(a / b), nil
	}, new(func(float64, float64) float64)),
}

func compile(src string, decl map[string]any) (*vm.Program, error) {
	opts := append([]expr.Option{expr.Env(decl)}, functions...)
	p, err := expr.Compile(src, opts...)
	if err != nil {
		return nil, fmt.Errorf("expression %q: %w", src, err)
	}
	return p, nil
}

type let struct {
	name string
	prog *vm.Program
}

func (l let) eval(env map[string]any) (any, error) {
	v, err := expr.Run(l.prog, env)
	if err != nil {
		return nil, fmt.Errorf("let %s: %w", l.name, err)
	}
	switch x := deref(v).(type) {
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	default:
		return x, nil
	}
}

func compileLets(lets []map[string]string, decl map[string]any) ([]let, error) {
	var out []let
	for _, m := range lets {
		if len(m) != 1 {
			return nil, fmt.Errorf("each let entry names one value")
		}
		for name, src := range m {
			if _, taken := decl[name]; taken {
				return nil, fmt.Errorf("let %s shadows a field", name)
			}
			p, err := compile(src, decl)
			if err != nil {
				return nil, fmt.Errorf("let %s: %w", name, err)
			}
			decl[name] = zeroOf(p)
			out = append(out, let{name: name, prog: p})
		}
	}
	return out, nil
}

// zeroOf declares a let's type for the expressions after it. A value whose
// type the compiler cannot tell (a map lookup) is declared a number, the
// common case; eval makes the runtime value agree.
func zeroOf(p *vm.Program) any {
	t := p.Node().Type()
	if t == nil || t.Kind() == reflect.Interface {
		return 0.0
	}
	return reflect.Zero(t).Interface()
}

func deref(v any) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		return rv.Elem().Interface()
	}
	return v
}

func toFloat(v any) float64 {
	switch x := deref(v).(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}

// text is a string with {expression} parts, such as "aws.lambda.{arch}.gb_second".
type text struct {
	parts []string
	progs []*vm.Program // progs[i] follows parts[i]
}

func compileText(src string, decl map[string]any) (text, error) {
	var t text
	rest := src
	for {
		open := strings.Index(rest, "{")
		if open < 0 {
			t.parts = append(t.parts, rest)
			return t, nil
		}
		end := strings.Index(rest[open:], "}")
		if end < 0 {
			return t, fmt.Errorf("unclosed { in %q", src)
		}
		p, err := compile(rest[open+1:open+end], decl)
		if err != nil {
			return t, err
		}
		t.parts = append(t.parts, rest[:open])
		t.progs = append(t.progs, p)
		rest = rest[open+end+1:]
	}
}

func (t text) eval(env map[string]any) (string, error) {
	var b strings.Builder
	for i, part := range t.parts {
		b.WriteString(part)
		if i >= len(t.progs) {
			continue
		}
		v, err := expr.Run(t.progs[i], env)
		if err != nil {
			return "", err
		}
		b.WriteString(format(deref(v)))
	}
	return b.String(), nil
}

func format(v any) string {
	switch x := v.(type) {
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}
