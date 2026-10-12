package clip

import "bytes"

type Memory struct {
	items []Item
	text  string
}

func textMime(m string) bool {
	return m == MimeText || m == MimeTextPlain
}

func (m *Memory) Text() string {
	return m.text
}

func (m *Memory) SetText(s string) {
	m.items, m.text = nil, s
}

func (m *Memory) SetData(mime string, data []byte, text string) bool {
	m.items = []Item{{Mime: mime, Data: bytes.Clone(data)}}
	m.text = text
	return true
}

func (m *Memory) SetItems(items []Item) bool {
	m.items, m.text = make([]Item, 0, len(items)), ""
	for _, it := range items {
		m.items = append(m.items, Item{Mime: it.Mime, Data: bytes.Clone(it.Data)})
		if textMime(it.Mime) {
			m.text = string(it.Data)
		}
	}
	return true
}

func (m *Memory) Data(mime string) ([]byte, bool) {
	for _, it := range m.items {
		if it.Mime == mime {
			return it.Data, true
		}
	}
	if textMime(mime) && m.text != "" {
		return []byte(m.text), true
	}
	return nil, false
}

func (m *Memory) Has(mime string) bool {
	_, ok := m.Data(mime)
	return ok
}
