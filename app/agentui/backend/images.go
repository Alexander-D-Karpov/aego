package backend

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
)

func toolImages(raw json.RawMessage) [][]byte {
	// Claude nests the base64 source; Codex reports MCP content directly.
	var b []struct {
		Type, MimeType, Data string
		Source               struct {
			Type string
			Mime string `json:"media_type"`
			Data string
		}
	}
	var r struct{ Content json.RawMessage }
	if len(raw) > 0 && raw[0] == '{' {
		if json.Unmarshal(raw, &r) != nil {
			return nil
		}
		raw = r.Content
	}
	if json.Unmarshal(raw, &b) != nil {
		return nil
	}
	var out [][]byte
	for _, v := range b {
		if v.Type != "image" {
			continue
		}
		data, mime := v.Data, v.MimeType
		if v.Source.Type == "base64" {
			data, mime = v.Source.Data, v.Source.Mime
		}
		if mime != "image/png" || len(data) > 7<<20 {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil || len(decoded) > 5<<20 {
			continue
		}
		c, err := png.DecodeConfig(bytes.NewReader(decoded))
		if err != nil || c.Width < 1 || c.Height < 1 || c.Width > 16384 || c.Height > (16<<20)/c.Width {
			continue
		}
		out = append(out, decoded)
		if len(out) == 3 {
			break
		}
	}
	return out
}
