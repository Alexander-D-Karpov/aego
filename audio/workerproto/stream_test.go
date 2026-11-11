package workerproto

import (
	"encoding/json"
	"reflect"
	"testing"

	"aego/audio/mix"
)

func streamStatusBytes(t *testing.T, buffered, free int, consumed ...uint64) []byte {
	t.Helper()
	var n uint64
	if len(consumed) != 0 {
		n = consumed[0]
	}
	b, err := json.Marshal(mix.RemoteStreamStatus{Buffered: buffered, Free: free, Consumed: n})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func streamStep(t *testing.T, s *StreamSender, op int32) StreamStep {
	t.Helper()
	step, ok, err := s.Next()
	if err != nil || !ok || step.Op != op {
		t.Fatalf("step = %+v, %v, %v, want operation %d", step, ok, err, op)
	}
	return step
}

func TestStreamSenderPartialWritesAndDeferredEOF(t *testing.T) {
	source := mix.NewStream(48000, 1, 8)
	if n := source.Ring.Write([]float32{1, 2, 3, 4, 5, 6, 7, 8}); n != 8 {
		t.Fatal(n)
	}
	source.SetEOF()
	s, err := NewStreamSender(1<<53+3, source, 4)
	if err != nil {
		t.Fatal(err)
	}
	reg := streamStep(t, s, Stream)
	if reg.Request.Source != 1<<53+3 || reg.Request.Frames != 8 {
		t.Fatalf("register = %+v", reg)
	}
	if err := s.Ack(reg.Token, nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		buffered, free, accepted int
		want                     []float32
	}{{0, 8, 2, []float32{1, 2, 3, 4}}, {2, 6, 4, []float32{3, 4, 5, 6}}, {6, 2, 2, []float32{7, 8}}} {
		if err := s.Poll(); err != nil {
			t.Fatal(err)
		}
		status := streamStep(t, s, StreamStatus)
		if err := s.Ack(status.Token, streamStatusBytes(t, tc.buffered, tc.free)); err != nil {
			t.Fatal(err)
		}
		write := streamStep(t, s, StreamWrite)
		if !reflect.DeepEqual(write.PCM, tc.want) || write.Request.Samples != len(tc.want) {
			t.Fatalf("write = %+v, want %v", write, tc.want)
		}
		if err := s.Ack(status.Token, nil); err == nil {
			t.Fatal("accepted a stale token")
		}
		result, _ := json.Marshal(struct {
			Frames int `json:"frames"`
		}{tc.accepted})
		if err := s.Ack(write.Token, result); err != nil {
			t.Fatal(err)
		}
	}
	if source.Ring.Len() != 8 {
		t.Fatal("accepted but unconsumed PCM was removed from the producer ring")
	}
	for _, accepted := range []bool{false, true} {
		if err := s.Poll(); err != nil {
			t.Fatal(err)
		}
		status := streamStep(t, s, StreamStatus)
		if err := s.Ack(status.Token, streamStatusBytes(t, 8, 0)); err != nil {
			t.Fatal(err)
		}
		end := streamStep(t, s, TryStreamEnd)
		result, _ := json.Marshal(struct {
			Ended bool `json:"ended"`
		}{accepted})
		if err := s.Ack(end.Token, result); err != nil {
			t.Fatal(err)
		}
		if s.Ended() != accepted {
			t.Fatalf("ended = %v after worker replied %v", s.Ended(), accepted)
		}
	}
	if _, ok, err := s.Next(); err != nil || ok {
		t.Fatalf("ended stream produced a step: %v, %v", ok, err)
	}
	// EOF means no more input, not that the worker has already consumed its
	// buffered audio. Polling must continue to reconcile the producer ring.
	for _, tc := range []struct {
		consumed      uint64
		buffered, len int
	}{{3, 5, 5}, {8, 0, 0}} {
		if err := s.Poll(); err != nil {
			t.Fatalf("cannot poll after EOF was accepted: %v", err)
		}
		if !s.Ended() {
			t.Fatal("polling consumption forgot that EOF was accepted")
		}
		status := streamStep(t, s, StreamStatus)
		reply, err := json.Marshal(mix.RemoteStreamStatus{Buffered: tc.buffered, Free: 8 - tc.buffered, Consumed: tc.consumed, EOF: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Ack(status.Token, reply); err != nil || source.Ring.Len() != tc.len || !s.Ended() {
			t.Fatalf("after-EOF consumption was not reconciled: len=%d, %v", source.Ring.Len(), err)
		}
		if _, ok, err := s.Next(); err != nil || ok {
			t.Fatalf("ended stream attempted another write or EOF: %v, %v", ok, err)
		}
	}
	if err := s.Close(); err == nil {
		t.Fatal("closed without a final status after the voice stopped")
	}
	if err := s.FinishAfterStop(); err != nil || s.ReadyToClose() {
		t.Fatalf("final status did not start safely: %v", err)
	}
	status := streamStep(t, s, StreamStatus)
	final, err := json.Marshal(mix.RemoteStreamStatus{Free: 8, Consumed: 8, EOF: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Ack(status.Token, final); err != nil || !s.ReadyToClose() {
		t.Fatalf("final status did not complete: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := mix.NewRemoteStreamSender(source)
	if err != nil {
		t.Fatal("completed sender did not release the source claim:", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStreamSenderRejectsMalformedWorkerReplies(t *testing.T) {
	for _, raw := range [][]byte{[]byte(`{}`), []byte(`{"buffered":0,"free":8}`),
		[]byte(`{"buffered":0,"free":8,"held":false,"skip":1,"control":0,"eof":false}`),
		[]byte(`{"buffered":9,"free":0,"held":false,"skip":"0","control":0,"eof":false}`),
		[]byte(`{"buffered":0,"free":8,"consumed":"0","held":false,"skip":"0","control":0,"eof":true}`)} {
		s, err := NewStreamSender(1, mix.NewStream(48000, 1, 8), 4)
		if err != nil {
			t.Fatal(err)
		}
		reg := streamStep(t, s, Stream)
		if err := s.Ack(reg.Token, nil); err != nil {
			t.Fatal(err)
		}
		if err := s.Poll(); err != nil {
			t.Fatal(err)
		}
		status := streamStep(t, s, StreamStatus)
		if err := s.Ack(status.Token, raw); err == nil {
			t.Fatalf("accepted invalid status %s", raw)
		}
		if _, _, err := s.Next(); err == nil {
			t.Fatal("continued after a malformed reply")
		}
		if err := s.AbortAfterTermination(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStreamSenderFinalStatusReconcilesControlBeforeClosing(t *testing.T) {
	source := mix.NewStream(48000, 1, 8)
	source.Ring.Write([]float32{1, 2, 3, 4, 5, 6, 7, 8})
	s, err := NewStreamSender(1, source, 4)
	if err != nil {
		t.Fatal(err)
	}
	reg := streamStep(t, s, Stream)
	if err := s.Ack(reg.Token, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishAfterStop(); err != nil {
		t.Fatal(err)
	}
	status := streamStep(t, s, StreamStatus)
	reply, err := json.Marshal(mix.RemoteStreamStatus{Free: 8, Held: true, Skip: 3, Control: 7})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Ack(status.Token, reply); err != nil {
		t.Fatal(err)
	}
	if s.ReadyToClose() || !source.Held() || source.Ring.Len() != 5 || s.Close() == nil {
		t.Fatal("closed before acknowledging final skip/hold")
	}
	control := streamStep(t, s, StreamControlAck)
	if control.Request.Control != 7 || s.ReadyToClose() {
		t.Fatal("final control was not sent exactly once")
	}
	if err := s.Ack(control.Token, nil); err != nil || !s.ReadyToClose() {
		t.Fatalf("final control was not acknowledged: %v", err)
	}
	if err := s.Close(); err != nil || source.Held() {
		t.Fatalf("closing the reconciled stream did not release its hold: %v", err)
	}
	got := make([]float32, 5)
	if n := source.Ring.Read(got); n != 5 || !reflect.DeepEqual(got, []float32{4, 5, 6, 7, 8}) {
		t.Fatalf("final skip changed the wrong PCM: %v (%d frames)", got, n)
	}
}

func TestStreamSenderAbortRetainsUnacknowledgedPCM(t *testing.T) {
	source := mix.NewStream(48000, 1, 8)
	source.Ring.Write([]float32{1, 2, 3, 4})
	s, err := NewStreamSender(1, source, 4)
	if err != nil {
		t.Fatal(err)
	}
	reg := streamStep(t, s, Stream)
	if err := s.Ack(reg.Token, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Poll(); err != nil {
		t.Fatal(err)
	}
	status := streamStep(t, s, StreamStatus)
	if err := s.Ack(status.Token, streamStatusBytes(t, 0, 8)); err != nil {
		t.Fatal(err)
	}
	_ = streamStep(t, s, StreamWrite)
	if err := s.Close(); err == nil {
		t.Fatal("closed with an in-flight PCM operation")
	}
	// The caller has terminated the worker. Its playback position is unknown;
	// the only safe automatic choice is to retain the unacknowledged PCM.
	if err := s.AbortAfterTermination(); err != nil {
		t.Fatal(err)
	}
	got := make([]float32, 4)
	if n := source.Ring.Read(got); n != 4 || !reflect.DeepEqual(got, []float32{1, 2, 3, 4}) {
		t.Fatalf("retained PCM = %v, %d frames", got, n)
	}
}

func TestStreamSenderStereoPartialWriteUsesFramesNotSamples(t *testing.T) {
	source := mix.NewStream(48000, 2, 8)
	if n := source.Ring.Write([]float32{1, 10, 2, 20, 3, 30, 4, 40}); n != 4 {
		t.Fatal(n)
	}
	s, err := NewStreamSender(2, source, 3)
	if err != nil {
		t.Fatal(err)
	}
	reg := streamStep(t, s, Stream)
	if err := s.Ack(reg.Token, nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		buffered, free, accepted int
		want                     []float32
	}{{0, 8, 2, []float32{1, 10, 2, 20, 3, 30}}, {2, 6, 2, []float32{3, 30, 4, 40}}} {
		if err := s.Poll(); err != nil {
			t.Fatal(err)
		}
		status := streamStep(t, s, StreamStatus)
		if err := s.Ack(status.Token, streamStatusBytes(t, tc.buffered, tc.free)); err != nil {
			t.Fatal(err)
		}
		write := streamStep(t, s, StreamWrite)
		if !reflect.DeepEqual(write.PCM, tc.want) || write.Request.Samples != len(tc.want) {
			t.Fatalf("stereo write = %+v, want %v", write, tc.want)
		}
		result, _ := json.Marshal(struct {
			Frames int `json:"frames"`
		}{tc.accepted})
		if err := s.Ack(write.Token, result); err != nil {
			t.Fatal(err)
		}
	}
	if source.Ring.Len() != 4 {
		t.Fatal("accepted stereo PCM was removed before consumption")
	}
	if err := s.Poll(); err != nil {
		t.Fatal(err)
	}
	status := streamStep(t, s, StreamStatus)
	if err := s.Ack(status.Token, streamStatusBytes(t, 0, 8, 4)); err != nil {
		t.Fatal(err)
	}
	if source.Ring.Len() != 0 {
		t.Fatal("stereo consumption did not release exactly four frames")
	}
	if err := s.AbortAfterTermination(); err != nil {
		t.Fatal(err)
	}
}

func TestStreamSenderRejectsMalformedWriteAcknowledgement(t *testing.T) {
	for _, reply := range []string{`{}`, `{"frames":null}`, `{"frames":-1}`, `{"frames":5}`} {
		source := mix.NewStream(48000, 1, 8)
		source.Ring.Write([]float32{1, 2, 3, 4})
		s, err := NewStreamSender(1, source, 4)
		if err != nil {
			t.Fatal(err)
		}
		reg := streamStep(t, s, Stream)
		if err := s.Ack(reg.Token, nil); err != nil {
			t.Fatal(err)
		}
		if err := s.Poll(); err != nil {
			t.Fatal(err)
		}
		status := streamStep(t, s, StreamStatus)
		if err := s.Ack(status.Token, streamStatusBytes(t, 0, 8)); err != nil {
			t.Fatal(err)
		}
		write := streamStep(t, s, StreamWrite)
		if err := s.Ack(write.Token, []byte(reply)); err == nil {
			t.Fatalf("accepted malformed acknowledgement %s", reply)
		}
		if err := s.AbortAfterTermination(); err != nil {
			t.Fatal(err)
		}
		if source.Ring.Len() != 4 {
			t.Fatalf("invalid acknowledgement consumed PCM: %s", reply)
		}
	}
}
