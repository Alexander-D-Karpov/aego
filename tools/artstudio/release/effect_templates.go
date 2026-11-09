package main

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"aego/art"
)

// The downloadable starters and thumbnails come from the same checked-in art
// documents used by both native New Effect and the Go/WASM New dialog.
func writeEffectTemplates(out, fixtures string) ([]effectTemplate, error) {
	// Read the selected source tree, not bytes embedded in a builder executable
	// that may have been compiled from another checkout.
	entries, err := os.ReadDir(fixtures)
	if err != nil {
		return nil, err
	}
	found := make(map[string]bool)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), art.SourceExtension) {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), art.SourceExtension)
		if _, ok := art.EffectTemplateOf(id); !ok || !entry.Type().IsRegular() {
			return nil, fmt.Errorf("unexpected effect template source %q", entry.Name())
		}
		found[id] = true
	}
	if len(found) != len(art.EffectTemplates()) {
		return nil, fmt.Errorf("source tree does not contain the complete effect template catalog")
	}
	var result []effectTemplate
	for _, item := range art.EffectTemplates() {
		data, err := readBounded(filepath.Join(fixtures, item.ID+art.SourceExtension), maxFileBytes)
		if err != nil {
			return nil, err
		}
		doc, err := art.Decode(data, art.Limits{})
		if err != nil {
			return nil, err
		}
		info := doc.Info()
		if info.Properties[art.EffectTemplateProperty] != item.ID {
			doc.Close()
			return nil, fmt.Errorf("effect template %q has a different document identity", item.ID)
		}
		frames := info.Frames
		im, err := doc.Render(frames[len(frames)/2].ID, art.RenderOptions{})
		doc.Close()
		if err != nil {
			return nil, err
		}
		var thumbnail bytes.Buffer
		if err = png.Encode(&thumbnail, im); err != nil {
			return nil, err
		}
		file, thumb := "templates/"+item.ID+"/start.aeart", "templates/"+item.ID+"/preview.png"
		if err = writeNew(filepath.Join(out, filepath.FromSlash(file)), data); err != nil {
			return nil, err
		}
		if err = writeNew(filepath.Join(out, filepath.FromSlash(thumb)), thumbnail.Bytes()); err != nil {
			return nil, err
		}
		result = append(result, effectTemplate{example: example{
			ID: item.ID, Version: 1, Title: map[string]string{"en": item.NameEN, "ru": item.NameRU},
			Description: map[string]string{"en": "Editable flipbook with an Aego particle preset.", "ru": "Редактируемый flipbook с пресетом частиц Aego."},
			File:        file, Thumbnail: thumb, SHA256: digest(data), Bytes: len(data),
		}, ParticlePreset: item.ParticlePreset})
	}
	return result, nil
}
