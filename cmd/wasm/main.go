//go:build js && wasm

// Command wasm exposes archgopher to the browser as a global function:
//
//	archGopher(name: string, input: string): string   // JSON {"ok": ...} or {"error": ...}
package main

import (
	"syscall/js"

	"github.com/O6lvl4/archgopher/api"
)

func main() {
	js.Global().Set("archGopher", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 {
			return `{"error":"archGopher(name, input)"}`
		}
		input := ""
		if len(args) > 1 {
			input = args[1].String()
		}
		return api.Call(args[0].String(), input)
	}))
	js.Global().Call("dispatchEvent", js.Global().Get("Event").New("archgopher-ready"))
	select {}
}
