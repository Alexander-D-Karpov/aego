package edit

import (
	"bytes"

	"aego/app/run"
)

func (s *Session) runTests(done func(out string, failed bool)) error {
	p, mods := s.Project, s.mods
	go func() {
		var buf bytes.Buffer
		code := run.Tool(p, []string{"test", "-v"}, &buf, &buf, mods...)
		s.Engine.Jobs().OnMain(func() { done(buf.String(), code != 0) })
	}()
	return nil
}
