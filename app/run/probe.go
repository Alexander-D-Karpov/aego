package run

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"aego/app/agent"
	"aego/modules/input"
	"aego/scene/mutate"
	"aego/schema"
)

const (
	// maxShots caps the screenshots of one run, so a run does not fill the agent's context with images.
	maxShots = 3
	// shotWidth is the width a screenshot is scaled down to for the answer; the full frame stays on disk.
	shotWidth = 640
	// runsDir keeps the full frames of agent runs.
	runsDir = ".aego/cache/runs"
)

// inject is one change of an action at the start of a frame.
type inject struct {
	frame  int
	action string
	v      float32
}

// parseInput reads a run's input script: "30 press Jump; 10-60 hold Right; 90 release Right; 0-40 axis Move 1".
func parseInput(script string) ([]inject, error) {
	var out []inject
	for _, line := range strings.FieldsFunc(script, func(r rune) bool { return r == ';' || r == '\n' }) {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) < 3 {
			return nil, errors.New("input " + strconv.Quote(line) + ": use <frame> press|hold|release <Action> or <frame> axis <Action> <value>")
		}
		from, to, ranged, err := frames(f[0])
		if err != nil {
			return nil, errors.New("input " + strconv.Quote(line) + ": " + err.Error())
		}
		name := f[2]
		switch verb := f[1]; verb {
		case "press":
			out = append(out, inject{from, name, 1}, inject{to + 1, name, 0})
		case "hold":
			out = append(out, inject{from, name, 1})
			if ranged {
				out = append(out, inject{to + 1, name, 0})
			}
		case "release":
			out = append(out, inject{from, name, 0})
		case "axis":
			if len(f) < 4 {
				return nil, errors.New("input " + strconv.Quote(line) + ": axis needs a value")
			}
			v, err := strconv.ParseFloat(f[3], 32)
			if err != nil {
				return nil, errors.New("input " + strconv.Quote(line) + ": " + f[3] + " is not a number")
			}
			out = append(out, inject{from, name, float32(v)})
			if ranged {
				out = append(out, inject{to + 1, name, 0})
			}
		default:
			return nil, errors.New("input " + strconv.Quote(line) + ": " + verb + " is not press, hold, release or axis")
		}
	}
	return out, nil
}

// frames reads "30" or "10-60".
func frames(s string) (int, int, bool, error) {
	a, b, ranged := strings.Cut(s, "-")
	from, err := strconv.Atoi(a)
	if err != nil || from < 0 {
		return 0, 0, false, errors.New(s + " is not a frame")
	}
	if !ranged {
		return from, from, false, nil
	}
	to, err := strconv.Atoi(b)
	if err != nil || to < from {
		return 0, 0, false, errors.New(s + " is not a range of frames")
	}
	return from, to, true, nil
}

// check is one assertion: at a frame, or at any frame of the run when frame is -1.
type check struct {
	text  string
	frame int
	node  string
	path  string
	op    string
	want  string
	// exists is "exists" or "gone" for a check on the node itself.
	exists string
	res    agent.Check
	done   bool
}

var ops = []string{"~=", "==", "!=", "<=", ">=", "<", ">"}

// parseChecks reads assertions: "@120 Player.transform.position.y < 3", "EnemyLeft.Health.hp == 0", "exists Ledge", "@60 gone Coin".
func parseChecks(list []string) ([]*check, error) {
	var out []*check
	for _, text := range list {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		c := &check{text: text, frame: -1}
		rest := text
		if strings.HasPrefix(rest, "@") {
			at, after, _ := strings.Cut(rest[1:], " ")
			n, err := strconv.Atoi(at)
			if err != nil || n < 0 {
				return nil, errors.New("assert " + strconv.Quote(text) + ": @" + at + " is not a frame")
			}
			c.frame, rest = n, strings.TrimSpace(after)
		}
		if verb, node, ok := strings.Cut(rest, " "); ok && (verb == "exists" || verb == "gone") {
			c.exists, c.node = verb, strings.TrimSpace(node)
			out = append(out, c)
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 3 || !contains(ops, f[1]) {
			return nil, errors.New("assert " + strconv.Quote(text) + ": use <Node>.<path> <op> <value> with op one of " + strings.Join(ops, " ") + ", or exists|gone <Node>")
		}
		node, path, ok := strings.Cut(f[0], ".")
		if !ok {
			return nil, errors.New("assert " + strconv.Quote(text) + ": " + f[0] + " names no field; write Node.Component.field or Node.transform.position.y")
		}
		c.node, c.path, c.op, c.want = node, path, f[1], strings.Join(f[2:], " ")
		out = append(out, c)
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// eval checks c on the running scene; it reports the value it found.
func (c *check) eval(d *mutate.Document) (bool, string, error) {
	if c.exists != "" {
		_, err := d.Node(c.node)
		alive := err == nil
		got := "exists"
		if !alive {
			got = "gone"
		}
		return (c.exists == "exists") == alive, got, nil
	}
	v, fd, err := d.Read(c.node, c.path)
	if err != nil {
		return false, "", err
	}
	got := d.Text(fd, v)
	want := strings.Trim(c.want, `"`)
	if n, ok := number(v); ok {
		w, err := strconv.ParseFloat(want, 64)
		if err != nil {
			return false, got, errors.New(c.path + " is a number; " + c.want + " is not")
		}
		return compare(n, w, c.op), got, nil
	}
	switch c.op {
	case "==", "~=":
		return strings.Trim(got, `"`) == want, got, nil
	case "!=":
		return strings.Trim(got, `"`) != want, got, nil
	}
	return false, got, errors.New(c.path + " is not a number; compare it with == or !=")
}

func number(v schema.Value) (float64, bool) {
	switch v.Kind {
	case schema.KindInt32, schema.KindInt64:
		return float64(v.I), true
	case schema.KindFloat32, schema.KindFloat64:
		return v.F, true
	}
	return 0, false
}

// compare is the numeric comparison; ~= allows 0.01 plus a tenth of a percent.
func compare(a, b float64, op string) bool {
	switch op {
	case "~=":
		return math.Abs(a-b) <= 0.01+0.001*math.Abs(b)
	case "==":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	}
	return a >= b
}

// run checks c after a frame: a frame check decides at its frame, an any-frame check passes at the first frame it holds.
func (c *check) run(d *mutate.Document, frame int) {
	if c.done || c.frame >= 0 && c.frame != frame {
		return
	}
	ok, got, err := c.eval(d)
	c.res = agent.Check{Assert: c.text, OK: ok, Value: got, Frame: frame}
	if err != nil {
		c.res.Error = err.Error()
	}
	if ok || err != nil || c.frame >= 0 {
		c.done = true
	}
}

// finish closes the checks the run never decided: a frame past the end, an any-frame check that never held.
func (c *check) finish(last int) agent.Check {
	if !c.done {
		c.res.Assert, c.res.OK = c.text, false
		if c.frame >= 0 {
			c.res.Error = "the run ended at frame " + strconv.Itoa(last) + " before frame " + strconv.Itoa(c.frame)
		} else if c.res.Frame == 0 {
			c.res.Frame = last
		}
	}
	return c.res
}

// shot saves a captured frame in full under .aego/cache/runs and keeps a copy scaled to width for the answer.
func shot(p string, name string, frame, width int, img *image.RGBA) (agent.Shot, error) {
	s := agent.Shot{Frame: frame}
	dir := filepath.Join(p, filepath.FromSlash(runsDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return s, err
	}
	full := filepath.Join(dir, name+"-f"+strconv.Itoa(frame)+".png")
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return s, err
	}
	if err := os.WriteFile(full, buf.Bytes(), 0o644); err != nil {
		return s, err
	}
	s.Path = filepath.ToSlash(filepath.Join(runsDir, filepath.Base(full)))
	small := scaleDown(img, width)
	buf.Reset()
	if err := png.Encode(&buf, small); err != nil {
		return s, err
	}
	b := small.Bounds()
	s.Width, s.Height, s.PNG = b.Dx(), b.Dy(), buf.Bytes()
	return s, nil
}

// scaleDown averages the pixels into an image width wide; a narrower image stays as it is.
func scaleDown(src *image.RGBA, width int) *image.RGBA {
	b := src.Bounds()
	if width <= 0 || b.Dx() <= width {
		return src
	}
	height := max(1, b.Dy()*width/b.Dx())
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		y0, y1 := b.Min.Y+y*b.Dy()/height, b.Min.Y+(y+1)*b.Dy()/height
		for x := 0; x < width; x++ {
			x0, x1 := b.Min.X+x*b.Dx()/width, b.Min.X+(x+1)*b.Dx()/width
			var r, g, bl, n uint32
			for sy := y0; sy < max(y1, y0+1); sy++ {
				i := src.PixOffset(x0, sy)
				for sx := x0; sx < max(x1, x0+1); sx++ {
					r += uint32(src.Pix[i])
					g += uint32(src.Pix[i+1])
					bl += uint32(src.Pix[i+2])
					i += 4
					n++
				}
			}
			o := dst.PixOffset(x, y)
			dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = uint8(r/n), uint8(g/n), uint8(bl/n), 255
		}
	}
	return dst
}

// injector feeds the input script to the running game at the start of each frame.
type injector struct {
	steps []inject
	svc   *input.Service
}

func (j *injector) frame(n int) error {
	for _, s := range j.steps {
		if s.frame == n {
			if err := j.svc.Inject(0, s.action, s.v); err != nil {
				return err
			}
		}
	}
	return nil
}
