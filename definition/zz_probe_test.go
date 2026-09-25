package definition

import (
	"testing"

	"github.com/expr-lang/expr"
)

func TestZZProbe(t *testing.T) {
	env := map[string]any{"a": []string{"SCRATCH", "X"}, "n": (*float64)(nil)}
	for _, src := range []string{
		`count(a, # == "SCRATCH")`,
		`n ?? float(count(a, # == "SCRATCH"))`,
		`n ?? count(a, # == "SCRATCH")`,
		`uniq(a)`,
		`filter(0..(len(a) - 1), # > 0)`,
		`map(0..-1, # * 2)`,
		`sum(map(a, 1.0))`,
		`map(a, let d = get(a, 5) ?? ""; d == "" ? 1.0 : 2.0)`,
	} {
		p, err := expr.Compile(src, expr.Env(env))
		if err != nil {
			t.Logf("%s: compile %v", src, err)
			continue
		}
		v, err := expr.Run(p, env)
		t.Logf("%s => %v %v", src, v, err)
	}
}
