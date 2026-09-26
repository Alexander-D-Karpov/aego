//go:build release

package log

func (c *ChannelLogger) Trace(msg string, fields ...Field) {}
