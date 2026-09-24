package facet

import "reflect"

// Includes names the assumption structs a resource definition can pull in
// with `includes: [logs]`, so their fields and defaults stay in one place.
var Includes = map[string]reflect.Type{
	"logs":   reflect.TypeFor[LogAssume](),
	"tokens": reflect.TypeFor[TokenAssume](),
}
