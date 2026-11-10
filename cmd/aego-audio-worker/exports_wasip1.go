//go:build wasip1 && wasm

package main

import (
	"unsafe"

	"aego/audio/workerproto"
)

// The reactor is single-threaded. The JavaScript worker owns these buffers and
// must finish each transfer before the next call can resize WASM memory.
var reactor service
var reactorInput, reactorResult, reactorError []byte

//go:wasmexport aego_abi_version
func reactorABIVersion() int32 { return workerproto.Version }

func reactorFailure(err error) {
	reactorResult = nil
	if err == nil {
		reactorError = nil
		return
	}
	reactorError = append(reactorError[:0], err.Error()...)
}

//go:wasmexport aego_input
func reactorInputBuffer(n int32) *byte {
	if n <= 0 || n > maxInputBytes {
		reactorInput = nil
		reactorFailure(errRequest)
		return nil
	}
	if cap(reactorInput) < int(n) {
		reactorInput = make([]byte, n)
	} else {
		reactorInput = reactorInput[:n]
	}
	reactorFailure(nil)
	return unsafe.SliceData(reactorInput)
}

//go:wasmexport aego_call
func reactorCall(op int32) int32 {
	result, err := reactor.invoke(op, reactorInput)
	if err != nil {
		reactorFailure(err)
		return 0
	}
	reactorError = nil
	reactorResult = result
	return 1
}

//go:wasmexport aego_memory
func reactorMemoryBuffer() *float32 {
	if reactor.memoryID == 0 {
		reactorFailure(errRequest)
		return nil
	}
	reactorFailure(nil)
	return unsafe.SliceData(reactor.memory)
}

//go:wasmexport aego_stream
func reactorStreamBuffer(n int32) *float32 {
	data, err := reactor.streamBuffer(int(n))
	if err != nil {
		reactorFailure(err)
		return nil
	}
	reactorFailure(nil)
	return unsafe.SliceData(data)
}

//go:wasmexport aego_render
func reactorRender(n int32) *float32 {
	data, err := reactor.render(int(n))
	if err != nil {
		reactorFailure(err)
		return nil
	}
	reactorFailure(nil)
	return unsafe.SliceData(data)
}

//go:wasmexport aego_result
func reactorResultBuffer() *byte { return unsafe.SliceData(reactorResult) }

//go:wasmexport aego_result_size
func reactorResultSize() int32 { return int32(len(reactorResult)) }

//go:wasmexport aego_error
func reactorErrorBuffer() *byte { return unsafe.SliceData(reactorError) }

//go:wasmexport aego_error_size
func reactorErrorSize() int32 { return int32(len(reactorError)) }
