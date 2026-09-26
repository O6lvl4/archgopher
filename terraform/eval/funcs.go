package eval

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

// unknownFuncs are Terraform functions whose result cannot be known without
// touching the filesystem, the clock or randomness. They evaluate to unknown,
// which is enough: edges come from references, not from values.
var unknownFuncs = []string{
	"abspath", "base64decode", "base64encode", "base64gzip", "base64sha256", "base64sha512", "basename",
	"bcrypt", "cidrhost", "cidrnetmask", "cidrsubnet", "cidrsubnets", "dirname", "file", "filebase64",
	"filebase64sha256", "filebase64sha512", "fileexists", "filemd5", "fileset", "filesha1", "filesha256",
	"filesha512", "formatdate", "index", "md5", "pathexpand", "plantimestamp", "rsadecrypt", "sensitive",
	"nonsensitive", "sha1", "sha256", "sha512", "templatefile", "templatestring", "textdecodebase64",
	"textencodebase64", "timeadd", "timecmp", "timestamp", "transpose", "urlencode", "uuid", "uuidv5",
	"yamldecode", "yamlencode", "ephemeralasnull", "issensitive", "provider::aws::arn_build", "provider::aws::arn_parse",
}

var unknownFunc = function.New(&function.Spec{
	VarParam: &function.Parameter{
		Name: "args", Type: cty.DynamicPseudoType,
		AllowUnknown: true, AllowNull: true, AllowDynamicType: true, AllowMarked: true,
	},
	Type: function.StaticReturnType(cty.DynamicPseudoType),
	Impl: func([]cty.Value, cty.Type) (cty.Value, error) { return cty.DynamicVal, nil },
})

// lookupFunc is Terraform's lookup: the default may be left out or be null,
// which go-cty's refuses ("argument must not be null", "3 required").
var lookupFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "inputMap", Type: cty.DynamicPseudoType, AllowMarked: true},
		{Name: "key", Type: cty.String, AllowMarked: true},
	},
	VarParam: &function.Parameter{Name: "default", Type: cty.DynamicPseudoType, AllowNull: true, AllowUnknown: true, AllowDynamicType: true, AllowMarked: true},
	Type:     function.StaticReturnType(cty.DynamicPseudoType),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		if len(args) == 3 && !args[2].IsNull() {
			return stdlib.LookupFunc.Call(args)
		}
		m := args[0]
		if !m.IsKnown() {
			return cty.DynamicVal, nil
		}
		if v, ok := element(m, args[1]); ok {
			return v, nil
		}
		if len(args) == 3 {
			return args[2], nil
		}
		return cty.DynamicVal, fmt.Errorf("lookup failed to find key %q", args[1].AsString())
	},
})

// element is m[key] of a map or object, and whether it is there.
func element(m, key cty.Value) (cty.Value, bool) {
	switch {
	case m.IsNull():
	case m.Type().IsObjectType() && m.Type().HasAttribute(key.AsString()):
		return m.GetAttr(key.AsString()), true
	case m.Type().IsMapType() && m.HasIndex(key).True():
		return m.Index(key), true
	}
	return cty.NilVal, false
}

func stringPredicate(f func(s, x string) bool) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "str", Type: cty.String}, {Name: "x", Type: cty.String}},
		Type:   function.StaticReturnType(cty.Bool),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			return cty.BoolVal(f(args[0].AsString(), args[1].AsString())), nil
		},
	})
}

// replaceFunc follows Terraform: a substring wrapped in slashes is a regular expression.
var replaceFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "str", Type: cty.String}, {Name: "substr", Type: cty.String}, {Name: "replace", Type: cty.String}},
	Type:   function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		sub := args[1].AsString()
		if len(sub) > 1 && strings.HasPrefix(sub, "/") && strings.HasSuffix(sub, "/") {
			return stdlib.RegexReplace(args[0], cty.StringVal(sub[1:len(sub)-1]), args[2])
		}
		return stdlib.Replace(args[0], args[1], args[2])
	},
})

var oneFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "list", Type: cty.DynamicPseudoType}},
	Type:   function.StaticReturnType(cty.DynamicPseudoType),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		v := args[0]
		if !v.CanIterateElements() || v.LengthInt() == 0 {
			return cty.NullVal(cty.DynamicPseudoType), nil
		}
		it := v.ElementIterator()
		it.Next()
		_, e := it.Element()
		return e, nil
	},
})

func reduceBools(all bool) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "list", Type: cty.List(cty.Bool)}},
		Type:   function.StaticReturnType(cty.Bool),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			for it := args[0].ElementIterator(); it.Next(); {
				_, e := it.Element()
				if e.IsNull() {
					continue
				}
				if e.True() != all {
					return cty.BoolVal(!all), nil
				}
			}
			return cty.BoolVal(all), nil
		},
	})
}

var sumFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "list", Type: cty.List(cty.Number)}},
	Type:   function.StaticReturnType(cty.Number),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		total := cty.NumberIntVal(0)
		for it := args[0].ElementIterator(); it.Next(); {
			_, e := it.Element()
			total = total.Add(e)
		}
		return total, nil
	},
})

func functions() map[string]function.Function {
	f := map[string]function.Function{
		"abs": stdlib.AbsoluteFunc, "ceil": stdlib.CeilFunc, "floor": stdlib.FloorFunc, "log": stdlib.LogFunc,
		"max": stdlib.MaxFunc, "min": stdlib.MinFunc, "parseint": stdlib.ParseIntFunc, "pow": stdlib.PowFunc,
		"signum": stdlib.SignumFunc,
		"chomp":  stdlib.ChompFunc, "format": stdlib.FormatFunc, "formatlist": stdlib.FormatListFunc,
		"indent": stdlib.IndentFunc, "join": stdlib.JoinFunc, "lower": stdlib.LowerFunc, "upper": stdlib.UpperFunc,
		"regex": stdlib.RegexFunc, "regexall": stdlib.RegexAllFunc, "replace": replaceFunc, "split": stdlib.SplitFunc,
		"strrev": stdlib.ReverseFunc, "substr": stdlib.SubstrFunc, "title": stdlib.TitleFunc, "trim": stdlib.TrimFunc,
		"trimprefix": stdlib.TrimPrefixFunc, "trimsuffix": stdlib.TrimSuffixFunc, "trimspace": stdlib.TrimSpaceFunc,
		"startswith": stringPredicate(strings.HasPrefix), "endswith": stringPredicate(strings.HasSuffix),
		"strcontains": stringPredicate(strings.Contains),
		"chunklist":   stdlib.ChunklistFunc, "coalesce": stdlib.CoalesceFunc, "coalescelist": stdlib.CoalesceListFunc,
		"compact": stdlib.CompactFunc, "concat": stdlib.ConcatFunc, "contains": stdlib.ContainsFunc,
		"distinct": stdlib.DistinctFunc, "element": stdlib.ElementFunc, "flatten": stdlib.FlattenFunc,
		"keys": stdlib.KeysFunc, "length": stdlib.LengthFunc, "lookup": lookupFunc, "merge": stdlib.MergeFunc,
		"range": stdlib.RangeFunc, "reverse": stdlib.ReverseListFunc, "setintersection": stdlib.SetIntersectionFunc,
		"setproduct": stdlib.SetProductFunc, "setsubtract": stdlib.SetSubtractFunc, "setunion": stdlib.SetUnionFunc,
		"slice": stdlib.SliceFunc, "sort": stdlib.SortFunc, "values": stdlib.ValuesFunc, "zipmap": stdlib.ZipmapFunc,
		"one": oneFunc, "alltrue": reduceBools(true), "anytrue": reduceBools(false), "sum": sumFunc,
		"jsonencode": stdlib.JSONEncodeFunc, "jsondecode": stdlib.JSONDecodeFunc, "csvdecode": stdlib.CSVDecodeFunc,
		"tostring": stdlib.MakeToFunc(cty.String), "tonumber": stdlib.MakeToFunc(cty.Number),
		"tobool": stdlib.MakeToFunc(cty.Bool), "tolist": stdlib.MakeToFunc(cty.List(cty.DynamicPseudoType)),
		"toset": stdlib.MakeToFunc(cty.Set(cty.DynamicPseudoType)), "tomap": stdlib.MakeToFunc(cty.Map(cty.DynamicPseudoType)),
		"try": tryfunc.TryFunc, "can": tryfunc.CanFunc,
	}
	for _, name := range unknownFuncs {
		f[name] = unknownFunc
	}
	return f
}
