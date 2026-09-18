package log

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type Level uint8

const (
	Trace Level = iota
	Debug
	Info
	Warn
	Error
	Fatal
)

func (l Level) String() string {
	switch l {
	case Trace:
		return "TRACE"
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	case Fatal:
		return "FATAL"
	}
	return "L" + strconv.Itoa(int(l))
}

type Channel string

const (
	Engine   Channel = "engine"
	Platform Channel = "platform"
	GPU      Channel = "gpu"
	Render   Channel = "render"
	Asset    Channel = "asset"
	Game     Channel = "game"
)

type Field struct {
	Key string
	Val any
}

func F(key string, v any) Field {
	return Field{Key: key, Val: v}
}

type Record struct {
	Time    float64
	Level   Level
	Channel Channel
	Msg     string
	Fields  []Field
}

type Sink interface {
	Write(Record)
}

type Logger struct {
	mu    sync.Mutex
	sinks []Sink
	level atomic.Uint32
	start time.Time
}

func New(sinks ...Sink) *Logger {
	l := &Logger{sinks: sinks, start: time.Now()}
	l.level.Store(uint32(Info))
	return l
}

func Nop() *Logger {
	return New()
}

func (l *Logger) SetLevel(lv Level) {
	l.level.Store(uint32(lv))
}

func (l *Logger) Level() Level {
	return Level(l.level.Load())
}

func (l *Logger) Enabled(lv Level) bool {
	return lv >= l.Level()
}

func (l *Logger) AddSink(s Sink) {
	l.mu.Lock()
	l.sinks = append(l.sinks, s)
	l.mu.Unlock()
}

func (l *Logger) Log(ch Channel, lv Level, msg string, fields ...Field) {
	if !l.Enabled(lv) {
		return
	}
	rec := Record{
		Time:    time.Since(l.start).Seconds(),
		Level:   lv,
		Channel: ch,
		Msg:     msg,
		Fields:  fields,
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.sinks {
		s.Write(rec)
	}
}

func (l *Logger) With(ch Channel) *ChannelLogger {
	return &ChannelLogger{l: l, ch: ch}
}

type ChannelLogger struct {
	l  *Logger
	ch Channel
}

func (c *ChannelLogger) Logger() *Logger {
	return c.l
}

func (c *ChannelLogger) Channel() Channel {
	return c.ch
}

func (c *ChannelLogger) Enabled(lv Level) bool {
	return c.l.Enabled(lv)
}

func (c *ChannelLogger) Debug(msg string, fields ...Field) {
	c.l.Log(c.ch, Debug, msg, fields...)
}

func (c *ChannelLogger) Info(msg string, fields ...Field) {
	c.l.Log(c.ch, Info, msg, fields...)
}

func (c *ChannelLogger) Warn(msg string, fields ...Field) {
	c.l.Log(c.ch, Warn, msg, fields...)
}

func (c *ChannelLogger) Error(msg string, fields ...Field) {
	c.l.Log(c.ch, Error, msg, fields...)
}

func (c *ChannelLogger) Fatal(msg string, fields ...Field) {
	c.l.Log(c.ch, Fatal, msg, fields...)
}
