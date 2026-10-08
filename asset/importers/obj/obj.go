package obj

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/render/mesh"
)

const Version = 1

type Options struct {
	Scale           float32
	FlipUV          bool
	GenerateNormals bool
}

type Importer struct{}

func New() *Importer {
	return &Importer{}
}

func (Importer) ID() string {
	return "obj"
}

func (Importer) Version() int {
	return Version
}

func (Importer) Extensions() []string {
	return []string{".obj"}
}

func (Importer) Defaults() pipeline.Settings {
	return pipeline.Settings{
		"scale":           aetext.Float64(1),
		"flipUV":          aetext.Bool(true),
		"generateNormals": aetext.Bool(false),
	}
}

func (Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	s := ctx.Meta.Settings
	d, err := Parse(ctx.Source, Options{
		Scale:           float32(s.Float("scale", 1)),
		FlipUV:          s.Bool("flipUV", true),
		GenerateNormals: s.Bool("generateNormals", false),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.AssetImportFailed, "cannot parse the obj").With(log.F("path", ctx.Path))
	}
	return &pipeline.Result{
		Kind:      asset.ArtMesh,
		Artifacts: []pipeline.Produced{{Kind: asset.ArtMesh, Data: mesh.Encode(d)}},
	}, nil
}

type corner struct {
	v, t, n int32
}

type parser struct {
	opt     Options
	pos     []mathx.Vec3
	uv      []mathx.Vec2
	nrm     []mathx.Vec3
	index   map[corner]uint32
	face    []uint32
	data    *mesh.Data
	line    int
	noNorms bool
}

func Parse(src []byte, o Options) (*mesh.Data, error) {
	if o.Scale == 0 {
		o.Scale = 1
	}
	p := &parser{opt: o, index: map[corner]uint32{}, data: &mesh.Data{}}
	sc := bufio.NewScanner(bytes.NewReader(src))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		p.line++
		if err := p.parseLine(strings.TrimSpace(sc.Text())); err != nil {
			return nil, err
		}
	}
	if err := sc.Err(); err != nil {
		return nil, p.errf("cannot read the file: " + err.Error())
	}
	if len(p.data.Indices) == 0 {
		return nil, errs.New(errs.AssetImportFailed, "obj has no faces")
	}
	if o.GenerateNormals || p.noNorms {
		p.generateNormals()
	}
	p.data.ComputeBounds()
	return p.data, nil
}

func (p *parser) errf(msg string) error {
	return errs.New(errs.AssetImportFailed, msg, log.F("line", p.line))
}

func (p *parser) parseLine(line string) error {
	if line == "" || line[0] == '#' {
		return nil
	}
	f := strings.Fields(line)
	switch f[0] {
	case "v":
		v, err := p.numbers(f[1:], 3)
		if err != nil {
			return err
		}
		s := p.opt.Scale
		p.pos = append(p.pos, mathx.Vec3{X: v[0] * s, Y: v[1] * s, Z: v[2] * s})
	case "vt":
		v, err := p.numbers(f[1:], 2)
		if err != nil {
			return err
		}
		if p.opt.FlipUV {
			v[1] = 1 - v[1]
		}
		p.uv = append(p.uv, mathx.Vec2{X: v[0], Y: v[1]})
	case "vn":
		v, err := p.numbers(f[1:], 3)
		if err != nil {
			return err
		}
		p.nrm = append(p.nrm, mathx.Vec3{X: v[0], Y: v[1], Z: v[2]}.Normalized())
	case "f":
		return p.parseFace(f[1:])
	}
	return nil
}

func (p *parser) numbers(fields []string, n int) ([3]float32, error) {
	var out [3]float32
	if len(fields) < n {
		return out, p.errf("expected " + strconv.Itoa(n) + " numbers")
	}
	for i := 0; i < n; i++ {
		f, err := strconv.ParseFloat(fields[i], 32)
		if err != nil {
			return out, p.errf("malformed number " + strconv.Quote(fields[i]))
		}
		out[i] = float32(f)
	}
	return out, nil
}

func (p *parser) resolve(s string, count int) (int32, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, p.errf("malformed index " + strconv.Quote(s))
	}
	if n < 0 {
		n = count + n + 1
	}
	if n < 1 || n > count {
		return 0, p.errf("index " + s + " is out of range")
	}
	return int32(n - 1), nil
}

func (p *parser) corner(tok string) (corner, error) {
	parts := strings.Split(tok, "/")
	c := corner{t: -1, n: -1}
	v, err := p.resolve(parts[0], len(p.pos))
	if err != nil {
		return c, err
	}
	c.v = v
	if len(parts) > 1 && parts[1] != "" {
		if c.t, err = p.resolve(parts[1], len(p.uv)); err != nil {
			return c, err
		}
	}
	if len(parts) > 2 && parts[2] != "" {
		if c.n, err = p.resolve(parts[2], len(p.nrm)); err != nil {
			return c, err
		}
	}
	return c, nil
}

func (p *parser) parseFace(fields []string) error {
	if len(fields) < 3 {
		return p.errf("face needs at least three vertices")
	}
	p.face = p.face[:0]
	for _, tok := range fields {
		c, err := p.corner(tok)
		if err != nil {
			return err
		}
		idx, ok := p.index[c]
		if !ok {
			v := mesh.Vertex{Pos: p.pos[c.v], Color: mesh.White}
			if c.t >= 0 {
				v.UV = p.uv[c.t]
			}
			if c.n >= 0 {
				v.Normal = p.nrm[c.n]
			} else {
				p.noNorms = true
			}
			idx = uint32(len(p.data.Vertices))
			p.data.Vertices = append(p.data.Vertices, v)
			p.index[c] = idx
		}
		p.face = append(p.face, idx)
	}
	for i := 1; i+1 < len(p.face); i++ {
		p.data.Indices = append(p.data.Indices, p.face[0], p.face[i], p.face[i+1])
	}
	return nil
}

func (p *parser) generateNormals() {
	vs := p.data.Vertices
	for i := range vs {
		vs[i].Normal = mathx.Vec3{}
	}
	ix := p.data.Indices
	for i := 0; i+2 < len(ix); i += 3 {
		a, b, c := ix[i], ix[i+1], ix[i+2]
		n := vs[b].Pos.Sub(vs[a].Pos).Cross(vs[c].Pos.Sub(vs[a].Pos))
		vs[a].Normal = vs[a].Normal.Add(n)
		vs[b].Normal = vs[b].Normal.Add(n)
		vs[c].Normal = vs[c].Normal.Add(n)
	}
	for i := range vs {
		if vs[i].Normal.LenSq() < 1e-20 {
			vs[i].Normal = mathx.Vec3{Y: 1}
			continue
		}
		vs[i].Normal = vs[i].Normal.Normalized()
	}
}
