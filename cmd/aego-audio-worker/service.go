package main

import (
	"encoding/json"
	"errors"

	"aego/audio/mix"
	"aego/audio/workerproto"
)

const (
	opInit             = workerproto.Init
	opBeginMemory      = workerproto.BeginMemory
	opCommitMemory     = workerproto.CommitMemory
	opStream           = workerproto.Stream
	opStreamWrite      = workerproto.StreamWrite
	opStreamEnd        = workerproto.StreamEnd
	opStreamStatus     = workerproto.StreamStatus
	opCommand          = workerproto.Command
	opDrop             = workerproto.Drop
	opDrain            = workerproto.Drain
	opSkip             = workerproto.Skip
	opStreamControlAck = workerproto.StreamControlAck
	opSetSampleRate    = workerproto.SetSampleRate
	opTryStreamEnd     = workerproto.TryStreamEnd
)

const (
	maxInputBytes    = workerproto.MaxJSON
	maxStreamSamples = 1 << 16
	maxOutputFrames  = 2048
	maxWorkerBudget  = 512 << 20
	maxWorkerVoices  = 1024
	maxWorkerBuses   = 64
)

var errRequest = errors.New("audio worker: invalid request or state")

type commandRequest = workerproto.CommandMessage
type request = workerproto.Request
type wireBatch = workerproto.Batch

type service struct {
	worker   *mix.RemoteSources
	frames   int
	memory   []float32
	memoryID uint64
	stream   []float32
	output   [maxOutputFrames * 2]float32
}

func decodeRequest(data []byte) (request, error) {
	r, err := workerproto.DecodeRequest(data)
	if err != nil {
		return request{}, errRequest
	}
	return r, nil
}

func (s *service) streamBuffer(samples int) ([]float32, error) {
	if s.worker == nil || samples < 0 || samples > maxStreamSamples {
		return nil, errRequest
	}
	if cap(s.stream) < samples {
		s.stream = make([]float32, samples)
	} else {
		s.stream = s.stream[:samples]
	}
	return s.stream, nil
}

func (s *service) render(frames int) ([]float32, error) {
	if s.worker == nil || frames <= 0 || frames > maxOutputFrames {
		return nil, errRequest
	}
	out := s.output[:frames*2]
	if err := s.worker.Mix(out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *service) invoke(op int32, data []byte) ([]byte, error) {
	r, err := decodeRequest(data)
	if err != nil {
		return nil, err
	}
	if op == opInit {
		if s.worker != nil || r.Rate <= 0 || r.Rate > 768000 || r.Frames <= 0 || r.Frames > maxOutputFrames ||
			r.Voices <= 0 || r.Voices > maxWorkerVoices || r.Budget <= 0 || r.Budget > maxWorkerBudget || len(r.Buses) > maxWorkerBuses {
			return nil, errRequest
		}
		cfg := mix.Config{SampleRate: r.Rate, Channels: 2, Voices: r.Voices, Block: mix.DefaultBlock, Buses: r.Buses}
		worker, err := mix.NewRemoteSources(cfg, r.Budget)
		if err != nil {
			return nil, err
		}
		s.worker, s.frames = worker, r.Frames
		return nil, nil
	}
	if s.worker == nil {
		return nil, errRequest
	}
	switch op {
	case opBeginMemory:
		if s.memoryID != 0 {
			return nil, errRequest
		}
		desc := mix.Memory{Rate: r.Rate, Channels: r.Channels, LoopStart: r.LoopStart, LoopEnd: r.LoopEnd, Loop: r.Loop}
		pcm, err := s.worker.BeginMemory(r.Source, desc, r.Samples)
		if err != nil {
			return nil, err
		}
		s.memory, s.memoryID = pcm, r.Source
	case opCommitMemory:
		if s.memoryID == 0 || s.memoryID != r.Source {
			return nil, errRequest
		}
		if err := s.worker.CommitMemory(r.Source); err != nil {
			return nil, err
		}
		s.memory, s.memoryID = nil, 0
	case opStream:
		if err := s.worker.RegisterStream(r.Source, r.Rate, r.Channels, r.Frames); err != nil {
			return nil, err
		}
	case opStreamWrite:
		if r.Samples < 0 || r.Samples > len(s.stream) {
			return nil, errRequest
		}
		n, err := s.worker.WriteStream(r.Source, s.stream[:r.Samples])
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Frames int `json:"frames"`
		}{n})
	case opStreamEnd:
		if err := s.worker.EndStream(r.Source); err != nil {
			return nil, err
		}
	case opTryStreamEnd:
		ended, err := s.worker.TryEndStream(r.Source)
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Ended bool `json:"ended"`
		}{ended})
	case opStreamStatus:
		status, err := s.worker.StreamStatus(r.Source)
		if err != nil {
			return nil, err
		}
		return json.Marshal(status)
	case opStreamControlAck:
		if err := s.worker.AckStreamControl(r.Source, r.Control); err != nil {
			return nil, err
		}
	case opCommand:
		if err := s.worker.Accept(r.Command.Mix()); err != nil {
			return nil, err
		}
	case opDrop:
		if err := s.worker.Drop(r.Source); err != nil {
			return nil, err
		}
		if s.memoryID == r.Source {
			s.memory, s.memoryID = nil, 0
		}
	case opDrain:
		batch, err := s.worker.Drain()
		if err != nil {
			return nil, err
		}
		return json.Marshal(batchWire(batch))
	case opSkip:
		if r.Frames < 0 {
			return nil, errRequest
		}
		if err := s.worker.Skip(r.Frames); err != nil {
			return nil, err
		}
	case opSetSampleRate:
		if r.Rate <= 0 || r.Rate > 768000 {
			return nil, errRequest
		}
		if err := s.worker.SetSampleRate(r.Rate); err != nil {
			return nil, err
		}
	default:
		return nil, errRequest
	}
	return nil, nil
}

func batchWire(b mix.RemoteBatch) wireBatch {
	return workerproto.BatchFromMix(b)
}
