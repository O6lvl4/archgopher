package scout

import "reflect"

func typeOf[T any]() reflect.Type { return reflect.TypeFor[T]() }

func f(v float64) *float64 { return &v }
