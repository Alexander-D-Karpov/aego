//go:build !release

package log

func (c *ChannelLogger) Trace(msg string, fields ...Field) {
	c.l.Log(c.ch, Trace, msg, fields...)
}
