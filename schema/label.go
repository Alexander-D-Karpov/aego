package schema

import (
	"strings"
	"unicode"
)

// Label is the name the Inspector shows for a field: its DisplayName hint, else the humanized field name.
func Label(fd *FieldDescriptor) string {
	if fd.Hints.DisplayName != "" {
		return fd.Hints.DisplayName
	}
	return Humanize(fd.Name)
}

// TypeLabel is the name the editor shows for a component type: its display name, else its full name.
func TypeLabel(td *TypeDescriptor) string {
	if td.DisplayName != "" {
		return Title(td)
	}
	return td.FullName
}

// Humanize turns clearColor or clear_color into Clear Color.
func Humanize(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		if r == '_' {
			b.WriteByte(' ')
			continue
		}
		if i == 0 {
			b.WriteRune(unicode.ToUpper(r))
			continue
		}
		prev := rs[i-1]
		var next rune
		if i+1 < len(rs) {
			next = rs[i+1]
		}
		if unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && unicode.IsLower(next))) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
