package platform

const (
	DefaultAudioRate   = 48000
	DefaultAudioFrames = 512
)

type AudioSpec struct {
	SampleRate int
	Channels   int
	Frames     int
}

func (s AudioSpec) WithDefaults() AudioSpec {
	if s.SampleRate <= 0 {
		s.SampleRate = DefaultAudioRate
	}
	if s.Channels <= 0 {
		s.Channels = 2
	}
	if s.Frames <= 0 {
		s.Frames = DefaultAudioFrames
	}
	return s
}

type AudioOutput interface {
	Spec() AudioSpec
	Write(samples []float32) int
	Queued() int
	Pause(bool)
	Close()
}

type Audioer interface {
	OpenAudio(want AudioSpec) (AudioOutput, error)
}

// AudioCounters are cumulative output measurements. Sum is the sum of squared
// channel samples; Samples is its denominator for an RMS calculation.
type AudioCounters struct {
	Underruns  int
	Consumed   int64
	Sum        float64
	Samples    float64
	Suppressed int64
}

type AudioDevice struct {
	ID      uint32
	Default bool
}
