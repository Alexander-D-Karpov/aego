package projectsettings

func (p *Panel) Edit(id string, fn func(d any)) {
	if p.work == nil {
		p.reset()
	}
	d := p.draft(id)
	fn(d)
	p.touch()
	p.gesture = false
	p.base = p.capture()
}

func (p *Panel) Draft(id string) any {
	if p.work == nil {
		p.reset()
	}
	return p.draft(id)
}
