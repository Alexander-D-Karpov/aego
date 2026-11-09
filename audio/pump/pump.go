package pump

import (
	"sync"
	"time"

	"aego/audio/mix"
)

type Output interface {
	Write(samples []float32) int
	Queued() int
	Close()
}

type Config struct {
	SampleRate int
	Channels   int
	Block      int
	Target     int
}

type Stats struct {
	Underruns uint64
	Dropped   uint64
	Blocks    uint64
	MixMax    time.Duration
	MixAvg    time.Duration
	Voices    int
}

const quietPeriod = 250 * time.Millisecond

type Pump struct {
	m      *mix.Mixer
	quiet  time.Time
	frac   float64
	out    Output
	cfg    Config
	buf    []float32
	mu     sync.Mutex
	st     Stats
	total  time.Duration
	primed bool
	stop   chan struct{}
	done   chan struct{}
}

func normalize(m *mix.Mixer, c Config) Config {
	if c.SampleRate <= 0 {
		c.SampleRate = m.SampleRate()
	}
	if c.Channels <= 0 {
		c.Channels = 2
	}
	if c.Block <= 0 {
		c.Block = mix.DefaultBlock
	}
	if c.Target <= 0 {
		c.Target = 3
	}
	return c
}

func New(m *mix.Mixer, out Output, cfg Config) *Pump {
	p := &Pump{m: m}
	p.SetOutput(out, cfg)
	return p
}

func (p *Pump) SetTarget(n int) {
	if n > 0 {
		p.cfg.Target = n
	}
}

func (p *Pump) SetOutput(out Output, cfg Config) {
	p.out = out
	p.cfg = normalize(p.m, cfg)
	if n := p.cfg.Block * p.cfg.Channels; cap(p.buf) < n {
		p.buf = make([]float32, n)
	} else {
		p.buf = p.buf[:n]
	}
	p.primed = false
}

func (p *Pump) live() bool {
	if r, ok := p.out.(interface{ Running() bool }); ok {
		return r.Running()
	}
	return true
}

func (p *Pump) silent() bool {
	if m, ok := p.out.(interface{ Muted() bool }); ok && m.Muted() {
		return true
	}
	return !p.live()
}

// skip moves the mixer on by the time since the last call while nothing can be heard, so voices end and streams keep their place without mixing.
func (p *Pump) skip() {
	now := time.Now()
	if p.quiet.IsZero() {
		p.quiet = now
		p.m.Skip(0)
		return
	}
	f := now.Sub(p.quiet).Seconds()*float64(p.cfg.SampleRate) + p.frac
	p.quiet = now
	n := int(f)
	p.frac = f - float64(n)
	p.m.Skip(n)
}

func (p *Pump) Fill() int {
	if p.out == nil {
		return 0
	}
	if p.silent() {
		p.skip()
		p.primed = false
		return 0
	}
	p.quiet, p.frac = time.Time{}, 0
	live := p.live()
	q := p.out.Queued()
	if q == 0 && p.primed && live && p.m.Active() > 0 {
		p.mu.Lock()
		p.st.Underruns++
		p.mu.Unlock()
	}
	target := p.cfg.Target * p.cfg.Block
	if !p.primed && q < target {
		// Async outputs may need to batch-publish startup blocks so the first new block cannot drain while the rest of the reserve is mixing.
		if out, ok := p.out.(interface {
			BeginAudioPrime(int)
			EndAudioPrime()
		}); ok {
			out.BeginAudioPrime(target - q + p.cfg.Block - 1)
			defer out.EndAudioPrime()
		}
	}
	n := 0
	for q < target {
		t := startMixTimer()
		p.m.Mix(p.buf)
		d := t.elapsed()
		w := p.out.Write(p.buf)
		p.record(d)
		n++
		if w <= 0 {
			break
		}
		q += w
	}
	if n > 0 && live {
		p.primed = true
	}
	return n
}

func (p *Pump) record(d time.Duration) {
	p.mu.Lock()
	p.st.Blocks++
	p.total += d
	p.st.MixMax = max(p.st.MixMax, d)
	p.st.MixAvg = p.total / time.Duration(p.st.Blocks)
	p.mu.Unlock()
}

func (p *Pump) Stats() Stats {
	p.mu.Lock()
	s := p.st
	p.mu.Unlock()
	s.Dropped = p.m.Commands().Dropped()
	s.Voices = p.m.Active()
	return s
}

func (p *Pump) Start() {
	if p.stop != nil {
		return
	}
	p.stop, p.done = make(chan struct{}), make(chan struct{})
	go p.run(p.stop, p.done)
}

func (p *Pump) run(stop, done chan struct{}) {
	defer close(done)
	period := max(time.Duration(float64(p.cfg.Block*p.cfg.Target)/float64(p.cfg.SampleRate)/6*float64(time.Second)), time.Millisecond)
	t := time.NewTicker(period)
	defer t.Stop()
	wake := p.m.Commands().Wake()
	quiet := false
	for {
		p.Fill()
		if q := p.silent(); q != quiet {
			quiet = q
			if quiet {
				t.Reset(quietPeriod)
			} else {
				t.Reset(period)
			}
		}
		if p.m.Active() == 0 && p.m.Commands().Len() == 0 {
			t.Stop()
			select {
			case <-stop:
				return
			case <-wake:
			}
			p.quiet = time.Time{}
			t.Reset(period)
			continue
		}
		select {
		case <-stop:
			return
		case <-t.C:
		}
	}
}

func (p *Pump) Running() bool {
	return p.stop != nil
}

func (p *Pump) Stop() {
	if p.stop == nil {
		return
	}
	close(p.stop)
	<-p.done
	p.stop, p.done = nil, nil
}

func (p *Pump) Close() {
	p.Stop()
	if p.out != nil {
		p.out.Close()
		p.out = nil
	}
}
