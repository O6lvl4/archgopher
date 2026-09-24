//go:build js && wasm

// Command wasm exposes arch-scouter to the browser as a global function:
//
//	archScouter(name: string, input: string): string   // JSON {"ok": ...} or {"error": ...}
package main

import (
	"syscall/js"

	"github.com/O6lvl4/arch-scouter/api"
)

func main() {
	js.Global().Set("archScouter", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 {
			return `{"error":"archScouter(name, input)"}`
		}
		input := ""
		if len(args) > 1 {
			input = args[1].String()
		}
		return api.Call(args[0].String(), input)
	}))
	js.Global().Call("dispatchEvent", js.Global().Get("Event").New("arch-scouter-ready"))
	select {}
}
