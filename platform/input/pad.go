package input

const MaxPads = 4

type PadButton uint8

const (
	PadA PadButton = iota
	PadB
	PadX
	PadY
	PadBack
	PadGuide
	PadStart
	PadL3
	PadR3
	PadLB
	PadRB
	PadUp
	PadDown
	PadLeft
	PadRight
	PadButtonCount
)

type PadAxis uint8

const (
	PadLeftX PadAxis = iota
	PadLeftY
	PadRightX
	PadRightY
	PadLT
	PadRT
	PadAxisCount
)

var padButtonNames = [PadButtonCount]string{"A", "B", "X", "Y", "Back", "Guide", "Start", "L3", "R3", "LB", "RB", "DPadUp", "DPadDown", "DPadLeft", "DPadRight"}

var padAxisNames = [PadAxisCount]string{"LeftX", "LeftY", "RightX", "RightY", "LT", "RT"}

func (b PadButton) String() string {
	if b < PadButtonCount {
		return padButtonNames[b]
	}
	return ""
}

func (a PadAxis) String() string {
	if a < PadAxisCount {
		return padAxisNames[a]
	}
	return ""
}

type PadEvent struct {
	ID     uint32
	Button PadButton
	Axis   PadAxis
	Value  float32
	Name   string
}

const (
	padDown uint8 = 1 << iota
	padPressed
	padReleased
)

type pad struct {
	on      bool
	id      uint32
	name    string
	buttons [PadButtonCount]uint8
	axes    [PadAxisCount]float32
}

func (s *State) padSlot(id uint32) int {
	for i := range s.pads {
		if s.pads[i].on && s.pads[i].id == id {
			return i
		}
	}
	return -1
}

func (s *State) ApplyPadAdded(id uint32, name string) int {
	if i := s.padSlot(id); i >= 0 {
		return i
	}
	for i := range s.pads {
		if !s.pads[i].on {
			s.pads[i] = pad{on: true, id: id, name: name}
			return i
		}
	}
	return -1
}

func (s *State) ApplyPadRemoved(id uint32) {
	i := s.padSlot(id)
	if i < 0 {
		return
	}
	p := &s.pads[i]
	p.on = false
	for b, v := range p.buttons {
		if v&padDown != 0 {
			p.buttons[b] = padReleased
		} else {
			p.buttons[b] = 0
		}
	}
	p.axes = [PadAxisCount]float32{}
}

func (s *State) ApplyPadButton(id uint32, b PadButton, down bool) {
	i := s.padSlot(id)
	if i < 0 || b >= PadButtonCount {
		return
	}
	v := &s.pads[i].buttons[b]
	switch {
	case down && *v&padDown == 0:
		*v |= padDown | padPressed
	case !down && *v&padDown != 0:
		*v = (*v &^ padDown) | padReleased
	}
}

func (s *State) ApplyPadAxis(id uint32, a PadAxis, v float32) {
	if i := s.padSlot(id); i >= 0 && a < PadAxisCount {
		s.pads[i].axes[a] = v
	}
}

func (s *State) beginPads() {
	for i := range s.pads {
		for b := range s.pads[i].buttons {
			s.pads[i].buttons[b] &= padDown
		}
	}
}

func (s *State) resetPads() {
	for i := range s.pads {
		p := &s.pads[i]
		for b, v := range p.buttons {
			if v&padDown != 0 {
				p.buttons[b] = padReleased
			}
		}
		p.axes = [PadAxisCount]float32{}
	}
}

func (s *State) padButton(i int, b PadButton) uint8 {
	if i < 0 || i >= MaxPads || b >= PadButtonCount {
		return 0
	}
	return s.pads[i].buttons[b]
}

func (s *State) PadConnected(i int) bool {
	return i >= 0 && i < MaxPads && s.pads[i].on
}

func (s *State) PadName(i int) string {
	if !s.PadConnected(i) {
		return ""
	}
	return s.pads[i].name
}

func (s *State) PadDown(i int, b PadButton) bool {
	return s.padButton(i, b)&padDown != 0
}

func (s *State) PadPressed(i int, b PadButton) bool {
	return s.padButton(i, b)&padPressed != 0
}

func (s *State) PadReleased(i int, b PadButton) bool {
	return s.padButton(i, b)&padReleased != 0
}

func (s *State) PadAxisValue(i int, a PadAxis) float32 {
	if i < 0 || i >= MaxPads || a >= PadAxisCount {
		return 0
	}
	return s.pads[i].axes[a]
}

func (s *State) CopyPads(src *State) {
	s.pads = src.pads
}
