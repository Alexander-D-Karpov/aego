package agent

import (
	"bytes"
	"errors"
	"image/png"
)

func variantCommand() *Command {
	return &Command{Name: "asset.import-variant", Write: true, Doc: "Import an inspected generated PNG variant from this editor conversation as a stable project source asset", Args: []Arg{{Name: "variant", Type: "string", Required: true}, {Name: "path", Type: "string", Required: true}, {Name: "replace", Type: "boolean"}}, Look: Look{}, Run: func(s *Session, a Args) (*Output, error) {
		host, ok := s.host.(interface{ GeneratedImage(string) ([]byte, bool) })
		if !ok {
			return nil, errors.New("requires an active editor generator session")
		}
		data, ok := host.GeneratedImage(a.Str("variant"))
		if !ok {
			return nil, errors.New("variant is missing or belongs to a previous conversation")
		}
		path, err := pngPath(a.Str("path"))
		if err != nil {
			return nil, err
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}

		return s.drawn(a, path, img, img, nil, false, nil, nil)
	}}
}
