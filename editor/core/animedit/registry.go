package animedit

import "aego/editor/core/document"

type Registry struct {
	files  Files
	notify func(string)
	by     map[document.ID]*Service
	board  []CopiedKey
}

func NewRegistry(files Files, notify func(string)) *Registry {
	return &Registry{files: files, notify: notify, by: map[document.ID]*Service{}}
}

func (r *Registry) For(d *document.SceneDocument) *Service {
	if s, ok := r.by[d.ID]; ok && s.doc == d {
		return s
	}
	s := New(d, r.files, r.notify)
	s.board = &r.board
	r.by[d.ID] = s
	return s
}

func (r *Registry) Drop(id document.ID) {
	if s, ok := r.by[id]; ok {
		s.Close()
		delete(r.by, id)
	}
}

// Copied is the animation clipboard shared by every clip.
func (r *Registry) Copied() []CopiedKey {
	return r.board
}

func (r *Registry) StopAll() {
	for _, s := range r.by {
		s.Close()
	}
}

func (r *Registry) Tick(dt float32) {
	for _, s := range r.by {
		s.Tick(dt)
	}
}

func (r *Registry) SaveGuard(d *document.SceneDocument) func() {
	s, ok := r.by[d.ID]
	if !ok || s.mode == Normal || !s.applied {
		return nil
	}
	s.restore()
	return s.apply
}

// Loaded tells every open clip that a file was written outside its own edits.
func (r *Registry) Loaded(rel string, data []byte) {
	for _, s := range r.by {
		s.loaded(rel, data)
	}
}
