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

type Pump struct {
	m      *mix.Mixer
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

func (p *Pump) Fill() int {
	if p.out == nil {
		return 0
	}
	live := p.live()
	q := p.out.Queued()
	if q == 0 && p.primed && live && p.m.Active() > 0 {
		p.mu.Lock()
		p.st.Underruns++
		p.mu.Unlock()
	}
	target := p.cfg.Target * p.cfg.Block
	n := 0
	for q < target {
		t := time.Now()
		p.m.Mix(p.buf)
		d := time.Since(t)
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
	period := time.Duration(float64(p.cfg.Block) / float64(p.cfg.SampleRate) / 2 * float64(time.Second))
	t := time.NewTicker(max(period, time.Millisecond))
	defer t.Stop()
	for {
		p.Fill()
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
