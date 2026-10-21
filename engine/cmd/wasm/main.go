//go:build js && wasm

package main

import (
	"encoding/binary"
	"math"
	"syscall/js"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/session"
)

const f64 = 8

func main() {
	s := session.New(session.DefaultConfig())
	in := make([]byte, session.SampleLen*f64)
	sample := make([]float64, session.SampleLen)
	out := make([]float64, session.OutMaxLen)
	outBytes := make([]byte, session.OutMaxLen*f64)
	uint8Array := js.Global().Get("Uint8Array")
	isBytes := func(v js.Value) bool { return v.Type() == js.TypeObject && v.InstanceOf(uint8Array) }

	// Bad arguments are ignored, never a panic: a panic would end the Go runtime for the whole page.
	push := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 || !isBytes(args[0]) || args[0].Get("byteLength").Int() != len(in) {
			return nil
		}
		js.CopyBytesToGo(in, args[0])
		for i := range sample {
			sample[i] = math.Float64frombits(binary.LittleEndian.Uint64(in[i*f64:]))
		}
		if x, err := session.SampleFromSlice(sample); err == nil {
			s.Push(x)
		}
		return nil
	})

	estimate := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 2 || args[0].Type() != js.TypeNumber || !isBytes(args[1]) {
			return 0
		}
		n := session.Encode(s.Estimate(args[0].Float()), out)
		for i, v := range out[:n] {
			binary.LittleEndian.PutUint64(outBytes[i*f64:], math.Float64bits(v))
		}
		return js.CopyBytesToJS(args[1], outBytes[:n*f64]) / f64
	})

	reset := js.FuncOf(func(js.Value, []js.Value) any {
		s.Reset()
		return nil
	})

	js.Global().Set("vitalsEngine", map[string]any{"push": push, "estimate": estimate, "reset": reset})
	select {}
}
