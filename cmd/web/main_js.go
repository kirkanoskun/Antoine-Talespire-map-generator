//go:build js && wasm

package main

import "syscall/js"

// main publishes globalThis.cartographeFetch(method, url, body), which returns
// a Promise of {status, contentType, body: Uint8Array}, then tells the page it
// is ready through the callbacks web/cartographe.js installs beforehand.
func main() {
	h, err := newHandler()
	if err != nil {
		if failed := js.Global().Get("__cartographeFailed"); failed.Type() == js.TypeFunction {
			failed.Invoke(err.Error())
		}
		return
	}

	js.Global().Set("cartographeFetch", js.FuncOf(func(_ js.Value, args []js.Value) any {
		method, target, body := "GET", "/", ""
		if len(args) > 0 {
			method = args[0].String()
		}
		if len(args) > 1 {
			target = args[1].String()
		}
		if len(args) > 2 && args[2].Type() == js.TypeString {
			body = args[2].String()
		}
		return promise(func() js.Value { return toJS(call(h, method, target, body)) })
	}))

	if ready := js.Global().Get("__cartographeReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	select {} // keep the engine alive for the page's lifetime
}

// promise runs work off the JS event loop — blocking inside a js.FuncOf
// callback would deadlock the Go runtime — and resolves with its result.
func promise(work func() js.Value) js.Value {
	var executor js.Func
	executor = js.FuncOf(func(_ js.Value, args []js.Value) any {
		resolve := args[0]
		go func() {
			defer executor.Release()
			resolve.Invoke(work())
		}()
		return nil
	})
	return js.Global().Get("Promise").New(executor)
}

func toJS(r response) js.Value {
	body := js.Global().Get("Uint8Array").New(len(r.body))
	js.CopyBytesToJS(body, r.body)
	out := js.Global().Get("Object").New()
	out.Set("status", r.status)
	out.Set("contentType", r.contentType)
	out.Set("body", body)
	return out
}
