//go:build js && wasm

package workerproto

import (
	"encoding/json"
	"testing"

	"aego/audio/mix"
)

// Silent advancement is a frequent part of the game frame, not a startup-only
// operation. Exercise real Go/JS serialization and the ordinary controller's
// acknowledgement on every iteration; JS object allocation is not measured.
func TestBrowserQuietSkipDoesNotAllocateInFrame(t *testing.T) {
	m, err := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
	if err != nil {
		t.Fatal(err)
	}
	b, host := browserFixture(t, m, BrowserOptions{})
	defer b.Terminate()
	host.Call("emit", map[string]any{"type": "ready"})
	if err := b.Quiesce(); err != nil {
		t.Fatal(err)
	}
	browserReply(host)
	if !b.quiesced {
		t.Fatal("fixture did not acknowledge quiescence")
	}
	var failed error
	allocs := testing.AllocsPerRun(100, func() {
		if err := b.QueueSkip(512); err != nil {
			failed = err
			return
		}
		if b.pending == 0 {
			failed = ErrBrowserTransport
			return
		}
		failed = b.controller.Ack(b.pending, nil)
		b.pending = 0
	})
	if failed != nil {
		t.Fatal(failed)
	}
	last := host.Get("sent").Index(host.Get("sent").Length() - 1)
	var request Request
	if last.Get("op").Int() != int(Skip) || json.Unmarshal([]byte(last.Get("input").String()), &request) != nil || request.Frames != 512 {
		t.Fatal("quiet advancement did not send the requested frames")
	}
	if allocs != 0 {
		t.Fatalf("quiet skip allocated %.2f Go objects per frame, want zero", allocs)
	}
}
