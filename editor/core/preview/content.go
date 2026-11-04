package preview

import (
	"aego/core/mathx"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/tasks"
	"aego/gpu"
)

var ErrUnsupportedContent = errors.New("no specialized visualizer for this content")

var ErrPending = errors.New("thumbnail is being prepared")

type Surface struct {
	Target gpu.TextureHandle
	Close  func()
}

type Handler struct {
	// Component identifies a provider for one component type inside a shared .aepreset format.
	Component  string
	ReadSource bool
	Build      func(*Card) (bool, error)
	Motion     func(*Card) (Motion, error)
	Version    string
	Render     func(asset.Entry, int) (Surface, error)
}

func (s *Service) Register(kind asset.ArtifactKind, h Handler) error {
	if kind == "" || h.Version == "" || (h.Render == nil && h.Build == nil) {
		return errors.New("invalid thumbnail handler")
	}
	if s.handlers == nil {
		s.handlers = make(map[asset.ArtifactKind]Handler)
	}
	if h.Component != "" {
		if kind != asset.ArtPreset || h.Motion != nil || h.Render == nil || h.Build == nil {
			return errors.New("component preset previews require surface and card providers")
		}
		if _, ok := s.handlers[kind]; ok && len(s.presetHandlers) == 0 {
			return errors.New("preset preview is already owned")
		}
		for _, old := range s.presetHandlers {
			if old.Component == h.Component {
				return fmt.Errorf("preset preview already registered for %s", h.Component)
			}
		}
		s.presetHandlers = append(s.presetHandlers, h)
		s.publishPresetHandlers()
		return nil
	}
	if _, ok := s.handlers[kind]; ok {
		return fmt.Errorf("thumbnail handler already registered for %s", kind)
	}
	s.handlers[kind] = h
	return nil
}

// Publish a fresh immutable dispatch table. Outstanding thumbnail workers can finish
// with their old table when a module is enabled or disabled.
func (s *Service) publishPresetHandlers() {
	kind := asset.ArtPreset
	if len(s.presetHandlers) == 0 {
		s.Unregister(kind)
		return
	}
	set := slices.Clone(s.presetHandlers)
	slices.SortFunc(set, func(a, b Handler) int { return strings.Compare(a.Component, b.Component) })
	var key strings.Builder
	for _, h := range set {
		fmt.Fprintf(&key, "%s=%s;", h.Component, h.Version)
	}
	s.handlers[kind] = Handler{Version: key.String(), ReadSource: true,
		Render: func(e asset.Entry, size int) (Surface, error) {
			for _, h := range set {
				surface, err := h.Render(e, size)
				if !errors.Is(err, ErrUnsupportedContent) {
					return surface, err
				}
			}
			return Surface{}, ErrUnsupportedContent
		}, Build: func(c *Card) (bool, error) {
			for _, h := range set {
				handled, err := h.Build(c)
				if handled || err != nil {
					return handled, err
				}
			}
			return false, nil
		},
	}
	s.invalidateKind(kind)
}

// UnregisterComponent removes only the module that provided this preset type.
func (s *Service) UnregisterComponent(kind asset.ArtifactKind, component string) {
	if component == "" {
		s.Unregister(kind)
		return
	}
	if kind == asset.ArtPreset {
		s.presetHandlers = slices.DeleteFunc(s.presetHandlers, func(h Handler) bool { return h.Component == component })
		s.publishPresetHandlers()
	}
}

func (s *Service) Unregister(kind asset.ArtifactKind) {
	delete(s.handlers, kind)
	if kind == asset.ArtPreset {
		s.presetHandlers = nil
	}
	s.invalidateKind(kind)
}

func (s *Service) invalidateKind(kind asset.ArtifactKind) {
	for id := range s.items {
		if e, ok := s.entry(id); ok && e.Kind == kind {
			s.Forget(id)
		}
	}
}

// Content is sampled on the editor thread. A pending result lets the caller yield
// until the same budgeted task that serves the Assets grid has completed.
func (s *Service) Content(id ids.AssetID) (image.Image, string, error) {
	if tex, ok := asset.BuiltinTexture(id); ok {
		if tex.Width <= 0 || tex.Height <= 0 {
			return nil, "", errors.New("empty builtin image")
		}
		pix, w, h := downscale(tex.Pixels, tex.Width, tex.Height, Size)
		return &image.NRGBA{Pix: pix, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}, builtinKey + id.String(), nil
	}
	e, ok := s.entry(id)
	if !ok {
		return nil, "", errors.New("asset is not in the active project")
	}
	it := s.ensure(infoFor(e))
	if it == nil {
		return nil, "", errors.New("no imported artifact or thumbnail service")
	}
	if it.state == pending {
		return nil, it.key, ErrPending
	}
	if it.reason != "" {
		return nil, it.key, errors.New(it.reason)
	}
	if it.state == failed || len(it.content.buf) == 0 {
		return nil, it.key, errors.New("preview could not be prepared")
	}
	p := it.content
	return &image.NRGBA{Pix: slices.Clone(p.buf), Stride: p.w * 4, Rect: image.Rect(0, 0, p.w, p.h)}, it.key, nil
}

func (s *Service) Reason(id ids.AssetID) string {
	if it := s.items[id]; it != nil {
		return it.reason
	}
	return ""
}

func (s *Service) startCustom(e asset.Entry, it *item, spec tasks.Spec, h Handler) {
	key := it.key
	var out pixels
	cached := false
	it.task = s.d.Tasks.Work(spec, func(t *tasks.Task) error {
		var err error
		out, err = s.load(key)
		cached = err == nil
		return nil
	}).Then(func(t *tasks.Task) {
		if s.items[it.id] != it || it.key != key {
			return
		}
		if cached || t.State() != tasks.Done {
			s.finish(it, key, out, t)
			return
		}
		var surface Surface
		fail := func(t *tasks.Task) {
			if s.items[it.id] != it || it.key != key {
				return
			}
			if errors.Is(t.Err(), ErrUnsupportedContent) {
				s.startCard(e, it, spec, false)
				return
			}
			if t.Err() != nil {
				e.Error = t.Err().Error()
				s.startCard(e, it, spec, true)
				return
			}
			s.finish(it, key, out, t)
			if t.State() == tasks.Done {
				s.d.Tasks.Work(spec, func(*tasks.Task) error { return s.save(key, out) })
			}
		}
		it.task = s.d.Tasks.Step(spec, tasks.LaneRender, func(*tasks.Task) (bool, error) {
			var err error
			surface, err = h.Render(e, Size)
			it.dispose = surface.Close
			if err == nil && (surface.Target.IsZero() || surface.Close == nil) {
				err = errors.New("thumbnail renderer returned an incomplete surface")
			}
			return true, err
		}).Then(func(draw *tasks.Task) {
			if draw.State() != tasks.Done || s.items[it.id] != it || it.key != key {
				it.release()
				fail(draw)
				return
			}
			it.task = s.d.Tasks.Step(spec, tasks.LaneMain, func(*tasks.Task) (bool, error) {
				buf := make([]byte, Size*Size*4)
				err := s.d.GPU.ReadPixels(surface.Target, 0, 0, Size, Size, buf)
				if err == nil {
					straight(buf, true)
					out = pixels{buf: buf, w: Size, h: Size}
				}
				return true, err
			}).Then(func(read *tasks.Task) { it.release(); fail(read) })
		})
	})
}

func (s *Service) startCard(e asset.Entry, it *item, spec tasks.Spec, bad bool) {
	key := it.key
	var src []byte
	if s.d.Docs != nil {
		if a := s.d.Docs.AssetDocument(e.ID); a != nil && a.Binding.PreviewSource != nil {
			src = slices.Clone(a.Binding.PreviewSource())
		}
		if d := s.d.Docs.Script(e.Source); d != nil && (d.Dirty() || src == nil) {
			src = slices.Clone(d.Code.Bytes())
		}
	}
	var out pixels
	var lines []string
	handler := s.handlers[e.Kind]
	build := handler.Build
	reason := e.Error
	if reason == "" && e.Missing {
		reason = "Missing source: " + e.Source
	}
	if reason == "" && !s.gpuOK && (Rendered(e.Kind) || (handler.Render != nil && handler.Build == nil)) {
		reason = "GPU preview unavailable in this session"
	}
	it.task = s.d.Tasks.Work(spec, func(*tasks.Task) error {
		if !bad && reason == "" {
			if cached, err := s.load(key); err == nil {
				out = cached
				return nil
			}
		}

		if src == nil && (build == nil || handler.ReadSource) && s.d.Pipeline != nil && !asset.IsBuiltin(e.ID) && textKind(e) {
			f, err := os.OpenInRoot(s.d.Pipeline.Abs("."), e.Source)
			if err == nil {
				defer f.Close()
				src, err = io.ReadAll(io.LimitReader(f, 64<<10))
			}
			if err != nil && reason == "" {
				reason = err.Error()
			}
		}
		if reason == "" && build != nil {
			c := s.input(e, src)
			handled, err := build(c)
			if err != nil {
				reason = err.Error()
			} else if handled {
				out = raster(c.Image)
				return s.save(key, out)
			}
		}

		lines = cardLines(e, src)
		if len(src) == 0 && len(e.Artifacts) > 0 && s.d.Pipeline != nil {
			a := e.Artifacts[0]
			if f, err := s.d.Pipeline.Artifacts().Open(a.Path); err == nil {
				var b [8]byte
				_, err = io.ReadFull(f, b[:])
				f.Close()
				if err == nil {
					magic := string(b[:4])
					isHeader := true
					for _, c := range magic {
						if c < 'A' || c > 'Z' {
							isHeader = false
						}
					}
					ver := binary.LittleEndian.Uint32(b[4:])
					if isHeader && ver < 65536 {
						lines = append([]string{fmt.Sprintf("%s v%d", magic, ver)}, lines...)
					}
				}
			}
		} else if asset.IsBuiltin(e.ID) && e.Kind == asset.ArtMesh {
			if m, ok := asset.BuiltinMesh(e.ID); ok {
				lines = append([]string{fmt.Sprintf("%d vertices", len(m.Vertices)), fmt.Sprintf("%d triangles", len(m.Indices)/3)}, lines...)
			}
		}

		if reason != "" {
			lines = append([]string{"! " + reason}, lines...)
		}
		out = s.card(string(e.Kind), lines)
		if reason == "" {
			return s.save(key, out)
		}
		return nil
	}).Then(func(t *tasks.Task) {
		if s.items[it.id] != it || it.key != key {
			return
		}
		s.finish(it, key, out, t)
		it.lines = lines
		if reason != "" {
			it.reason = reason
		}
	})
}

func textKind(e asset.Entry) bool {
	switch strings.ToLower(filepath.Ext(e.Source)) {
	case ".go", ".aes", ".aeshader", ".glsl", ".vert", ".frag", ".aedata", ".aetable", ".aepreset", ".aectrl", ".aeanim", ".aemat", ".aescene", ".aeprefab", ".aego", ".aetext", ".json", ".csv", ".toml":
		return true
	}
	return false
}

func cardLines(e asset.Entry, src []byte) []string {
	if len(src) > 0 && !bytes.ContainsRune(src, 0) && textKind(e) {
		ls := firstLines(string(src), lineCount)
		if e.Kind == asset.ArtScript || filepath.Ext(e.Source) == ".go" {
			var decl []string
			for _, line := range strings.Split(string(src), "\n") {
				l := strings.TrimSpace(line)
				for _, k := range []string{"script ", "func ", "type "} {
					if strings.HasPrefix(l, k) {
						decl = append(decl, strings.TrimSuffix(l, " {"))
						break
					}
				}
			}
			if len(decl) > 0 {
				ls = decl
			}
		}
		if len(ls) > 0 {
			return ls
		}
	}
	out := []string{filepath.Base(e.Source)}
	for _, art := range e.Artifacts {
		out = append(out, fmt.Sprintf("%s %d B", art.Kind, art.Size))
	}
	if len(e.Deps) > 0 {
		out = append(out, fmt.Sprintf("%d dependencies", len(e.Deps)))
	}
	return append(out, e.ID.Short())
}

func infoFor(e asset.Entry) pipeline.AssetInfo {
	return pipeline.AssetInfo{ID: e.ID, Kind: e.Kind, Path: e.Source, Name: e.Name, Error: e.Error}
}

func (s *Service) Role(id ids.AssetID) asset.TextureRole {
	if it := s.items[id]; it != nil {
		return it.role
	}
	return ""
}
func straight(buf []byte, srgb bool) {
	for i := 0; i < len(buf); i += 4 {
		a := float32(buf[i+3]) / 255
		if a == 0 {
			clear(buf[i : i+3])
			continue
		}
		if a == 1 && srgb {
			continue
		}
		for j := i; j < i+3; j++ {
			x := float32(buf[j]) / 255
			if srgb {
				x = mathx.SRGBToLinear(x)
			}
			buf[j] = channel(mathx.LinearToSRGB(x / a))
		}
	}
}
func encodeRGB(buf []byte) {
	for i := 0; i < len(buf); i += 4 {
		for j := i; j < i+3; j++ {
			buf[j] = channel(mathx.LinearToSRGB(float32(buf[j]) / 255))
		}
	}
}
func channel(x float32) byte { return byte(min(max(x, 0), 1)*255 + .5) }
