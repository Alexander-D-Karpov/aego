package asset

import (
	"fmt"
	"math"
	"path"
	"slices"
	"strings"

	"aego/core/ids"
	"aego/format/aetext"
)

// TextureRole is authoring meaning; a height source can produce a normal
// artifact, so source role and runtime TextureSemantic are intentionally separate.
type TextureRole string

const (
	RoleColor        TextureRole = "color"
	RoleNormal       TextureRole = "normal"
	RoleHeight       TextureRole = "height"
	RoleEmissionRGB  TextureRole = "emission-rgb"
	RoleEmissionMask TextureRole = "emission-mask"
	RoleCookieMask   TextureRole = "cookie-mask"
	RoleData         TextureRole = "data"
	RoleMask         TextureRole = "mask"
)

type TextureSettingSpec struct {
	Name    string        `json:"name"`
	Type    string        `json:"type"`
	Default aetext.Value  `json:"-"`
	Choices []string      `json:"choices,omitempty"`
	Roles   []TextureRole `json:"roles,omitempty"`
}

// TextureSettings is the common Inspector/CLI/importer schema. Unknown keys
// remain semantic data, but typed mutations only author registered settings.
func TextureSettings() []TextureSettingSpec {
	return []TextureSettingSpec{
		{Name: "role", Type: "enum", Default: aetext.Ident("auto"), Choices: []string{"auto", "color", "normal", "height", "emission-rgb", "emission-mask", "cookie-mask", "data", "mask"}},
		{Name: "semantic", Type: "enum", Default: aetext.Ident("auto"), Choices: []string{"auto", "color", "normal", "height", "emission-rgb", "emission-mask", "cookie-mask", "data", "mask"}},
		{Name: "colorspace", Type: "enum", Default: aetext.Ident("auto"), Choices: []string{"auto", "srgb", "linear"}},
		{Name: "filter", Type: "enum", Default: aetext.Ident("nearest"), Choices: []string{"nearest", "linear"}},
		{Name: "srgb", Type: "bool", Default: aetext.Bool(true)},
		{Name: "data", Type: "bool", Default: aetext.Bool(false)},
		{Name: "atlas", Type: "string", Default: aetext.String("")},
		{Name: "padding", Type: "int", Default: aetext.Int(2)},
		{Name: "extrude", Type: "bool", Default: aetext.Bool(true)},
		{Name: "trim", Type: "bool", Default: aetext.Bool(false)},
		{Name: "rotate", Type: "bool", Default: aetext.Bool(false)},
		{Name: "mips", Type: "bool", Default: aetext.Bool(false)},
		{Name: "normal_y", Type: "enum", Default: aetext.Ident("up"), Choices: []string{"up", "down"}, Roles: []TextureRole{RoleNormal, RoleHeight}},
		{Name: "generate_normal", Type: "bool", Default: aetext.Bool(false), Roles: []TextureRole{RoleHeight}},
		{Name: "height_strength", Type: "float", Default: aetext.Float32(1), Roles: []TextureRole{RoleHeight}},
		{Name: "height_channel", Type: "enum", Default: aetext.Ident("r"), Choices: []string{"r", "g", "b", "a", "luminance"}, Roles: []TextureRole{RoleHeight}},
		{Name: "mask_channel", Type: "enum", Default: aetext.Ident("r"), Choices: []string{"r", "g", "b", "a", "luminance"}, Roles: []TextureRole{RoleEmissionMask, RoleCookieMask, RoleMask}},
		{Name: "auto_normal", Type: "bool", Default: aetext.Bool(true), Roles: []TextureRole{RoleColor}},
		{Name: "auto_emission", Type: "bool", Default: aetext.Bool(true), Roles: []TextureRole{RoleColor}},
	}
}
func TextureSetting(name string) (TextureSettingSpec, bool) {
	for _, f := range TextureSettings() {
		if f.Name == name {
			return f, true
		}
	}
	return TextureSettingSpec{}, false
}
func TextureDefaults() ImportSettings {
	s := ImportSettings{}
	for _, f := range TextureSettings() {
		s[f.Name] = f.Default.Clone()
	}
	return s
}

func TextureRoleOf(name string, s ImportSettings) (TextureRole, error) {
	name = strings.TrimSuffix(name, MetaSuffix)
	kind := s.String("role", "auto")
	if kind == "auto" {
		kind = s.String("semantic", "auto")
	}
	if kind == "auto" {
		stem := strings.ToLower(strings.TrimSuffix(name, path.Ext(name)))
		switch {
		case s.Bool("data", false):
			kind = "data"
		case strings.HasSuffix(stem, "_n"):
			kind = "normal"
		case strings.HasSuffix(stem, "_e"):
			kind = "emission-rgb"
		case strings.HasSuffix(stem, "_em"):
			kind = "emission-mask"
		default:
			kind = "color"
		}
	}
	role := TextureRole(kind)
	switch role {
	case RoleColor, RoleNormal, RoleHeight, RoleEmissionRGB, RoleEmissionMask, RoleCookieMask, RoleData, RoleMask:
		return role, nil
	}
	return "", fmt.Errorf("unsupported texture role %q", kind)
}
func TextureSRGB(role TextureRole, s ImportSettings) (bool, error) {
	space := s.String("colorspace", "auto")
	if space != "auto" && space != "linear" && space != "srgb" {
		return false, fmt.Errorf("colorspace must be auto, srgb or linear")
	}
	color := role == RoleColor || role == RoleEmissionRGB
	if !color {
		if space == "srgb" {
			return false, fmt.Errorf("%s is linear data and cannot use sRGB colorspace", role)
		}
		return false, nil
	}
	if space == "auto" {
		return s.Bool("srgb", true), nil
	}
	return space == "srgb", nil
}

type TextureSourceInfo struct {
	ID                        ids.AssetID
	SourceID                  ids.AssetID
	Path                      string
	Width, Height             int // logical frame dimensions, before atlas trim
	SourceWidth, SourceHeight int
	Rect                      [4]int // source image coordinates; never atlas coordinates
	Pivot                     [2]float32
	Role                      TextureRole
	GenerateNormal            bool
}

type TextureSourceProblem struct {
	Code, Field, Message string
	Error, Missing       bool
}

func textureProblem(field, message string) TextureSourceProblem {
	return TextureSourceProblem{Code: "TEX001", Field: field, Message: message, Error: true}
}

// TextureChannel reads an explicit source/frame association. An explicit zero
// disables that channel; absence is inheritance/discovery, not a zero value.
func TextureChannel(m *SourceMeta, frame ids.AssetID, channel string) (ids.AssetID, bool, error) {
	if channel != "normal" && channel != "height" && channel != "emission" {
		return ids.AssetID{}, false, fmt.Errorf("unsupported surface channel %q", channel)
	}
	if frame.IsZero() || frame == m.ID {
		v, present := m.Settings[channel]
		return sourceAssetRef(v, present, channel)
	}
	bindings, err := TextureFrameChannels(m.Settings, channel)
	if err != nil {
		return ids.AssetID{}, false, err
	}
	id, ok := bindings[frame]
	return id, ok, nil
}
func sourceAssetRef(v aetext.Value, present bool, key string) (ids.AssetID, bool, error) {
	if !present {
		return ids.AssetID{}, false, nil
	}
	if v.Kind != aetext.VAsset {
		return ids.AssetID{}, true, fmt.Errorf("%s must be an asset(ID) reference", key)
	}
	id, err := ids.ParseAssetID(v.Str)
	return id, true, err
}
func TextureFrameChannels(s ImportSettings, channel string) (map[ids.AssetID]ids.AssetID, error) {
	out := map[ids.AssetID]ids.AssetID{}
	v, ok := s[channel+"_frames"]
	if !ok {
		return out, nil
	}
	if v.Kind != aetext.VList {
		return nil, fmt.Errorf("%s_frames must be a list of frame references", channel)
	}
	for _, entry := range v.List {
		if entry.Kind != aetext.VStruct {
			return nil, fmt.Errorf("%s_frames needs structured frame references", channel)
		}
		fields := ImportSettings{}
		for _, f := range entry.Obj {
			fields[f.Name] = f.Value
		}
		a, has, err := sourceAssetRef(fields["albedo"], true, "albedo")
		if err != nil || !has || a.IsZero() {
			return nil, fmt.Errorf("%s_frames requires a nonzero albedo frame AssetID", channel)
		}
		b, _, err := sourceAssetRef(fields[channel], true, channel)
		if err != nil {
			return nil, err
		}
		if _, duplicate := out[a]; duplicate {
			return nil, fmt.Errorf("%s_frames repeats frame %s", channel, a)
		}
		out[a] = b
	}
	return out, nil
}
func SetTextureChannel(m *SourceMeta, frame ids.AssetID, channel string, target ids.AssetID, explicit bool) error {
	if channel != "normal" && channel != "height" && channel != "emission" {
		return fmt.Errorf("unsupported surface channel %q", channel)
	}
	if m.Settings == nil {
		m.Settings = ImportSettings{}
	}
	if frame.IsZero() || frame == m.ID {
		if explicit {
			m.Settings[channel] = aetext.AssetRef(target.String())
		} else {
			delete(m.Settings, channel)
		}
		return nil
	}
	if _, ok := m.Frame(frame); !ok {
		return fmt.Errorf("frame %s is not in this source", frame)
	}
	if _, err := TextureFrameChannels(m.Settings, channel); err != nil {
		return err
	}
	key := channel + "_frames"
	list := m.Settings[key].List
	out := make([]aetext.Value, 0, len(list)+1)
	found := false
	for _, entry := range list {
		match := false
		for _, f := range entry.Obj {
			if f.Name == "albedo" && f.Value.Str == frame.String() {
				match = true
				break
			}
		}
		if !match {
			out = append(out, entry.Clone())
			continue
		}
		found = true
		if explicit {
			updated := entry.Clone()
			setSourceField(&updated.Obj, channel, aetext.AssetRef(target.String()))
			out = append(out, updated)
		}
	}
	if explicit && !found {
		out = append(out, aetext.Struct(aetext.Field{Name: "albedo", Value: aetext.AssetRef(frame.String())}, aetext.Field{Name: channel, Value: aetext.AssetRef(target.String())}))
	}
	m.Settings[key] = aetext.List(out...)
	return nil
}

// ValidateTextureSource does not require a GPU or importer. Missing explicit
// references are diagnostics while their IDs remain valid authoring data.
func ValidateTextureSource(m *SourceMeta, name string, width, height int, resolve func(ids.AssetID) (TextureSourceInfo, bool)) []TextureSourceProblem {
	var problems []TextureSourceProblem
	add := func(field, msg string) { problems = append(problems, textureProblem(field, msg)) }
	if m.ID.IsZero() {
		add("id", "source AssetID must be nonzero")
	}
	if m.Importer != "texture" {
		add("importer", "texture metadata requires the texture importer")
	}
	for _, spec := range TextureSettings() {
		v, ok := m.Settings[spec.Name]
		if !ok {
			continue
		}
		valid := true
		switch spec.Type {
		case "bool":
			valid = v.Kind == aetext.VBool
		case "string":
			valid = v.Kind == aetext.VString
		case "int":
			valid = v.Kind == aetext.VInt
		case "float":
			valid = (v.Kind == aetext.VFloat || v.Kind == aetext.VInt) && !math.IsNaN(v.AsFloat()) && !math.IsInf(v.AsFloat(), 0)
		case "enum":
			valid = v.Kind == aetext.VString || v.Kind == aetext.VIdent
			word := m.Settings.String(spec.Name, "")
			found := false
			for _, choice := range spec.Choices {
				found = found || choice == word
			}
			valid = valid && found
		}
		if !valid {
			add(spec.Name, "invalid "+spec.Type+" value for "+spec.Name)
		}
	}
	role, err := TextureRoleOf(name, m.Settings)
	if err != nil {
		add("role", err.Error())
	}
	if _, err := TextureSRGB(role, m.Settings); err != nil {
		add("colorspace", err.Error())
	}
	if m.Settings.Bool("generate_normal", false) && role != RoleHeight {
		add("generate_normal", "generate_normal requires a height source")
	}
	strength := m.Settings.Float("height_strength", 1)
	if strength < 0 || strength > 65536 {
		add("height_strength", "height strength must be in [0,65536]")
	}
	if p := m.Settings.Int("padding", 2); p < 0 || p > 64 {
		add("padding", "padding must be in [0,64]")
	}
	if m.Settings.Bool("rotate", false) {
		add("rotate", "rotated atlas regions are not supported")
	}
	if m.Settings.Bool("trim", false) && m.Settings.String("atlas", "") == "" {
		add("trim", "trim requires an atlas group")
	}
	if m.Settings.Bool("mips", false) && m.Settings.String("atlas", "") != "" {
		add("mips", "atlas mipmaps require per-region mip isolation")
	}
	idsSeen := map[ids.AssetID]bool{m.ID: true}
	names := map[string]bool{}
	for _, f := range m.Subs {
		if f.ID.IsZero() || idsSeen[f.ID] {
			add("frames", "frame AssetIDs must be distinct and nonzero")
		}
		idsSeen[f.ID] = true
		if resolve != nil {
			if info, exists := resolve(f.ID); exists && !info.SourceID.IsZero() && info.SourceID != m.ID {
				add("frames", "frame AssetID belongs to another source")
			}
		}

		if f.Name == "" || names[f.Name] || strings.ContainsAny(f.Name, "#\\/\n\r") {
			add("frames", "frame names must be unique, nonempty and contain no path separators")
		}
		names[f.Name] = true
		r := f.Rect
		if r[0] < 0 || r[1] < 0 || r[2] <= 0 || r[3] <= 0 || (width > 0 && (r[0] > width || r[2] > width-r[0])) || (height > 0 && (r[1] > height || r[3] > height-r[1])) {
			add("frames", "frame "+f.Name+" rectangle is outside the source image")
		}
	}
	check := func(frame, target ids.AssetID, channel string) {
		if target.IsZero() {
			return
		}
		field := channel
		if frame != m.ID {
			field = frame.String() + "." + channel
		}
		if target == m.ID || target == frame {
			add(field, "a surface cannot be its own "+channel+" source")
			return
		}
		if role != RoleColor {
			add(field, "surface channels belong to a color source")
			return
		}
		if resolve == nil {
			return
		}
		info, ok := resolve(target)
		if !ok {
			problems = append(problems, TextureSourceProblem{Code: "TEX002", Field: field, Message: "unresolved " + channel + " AssetID " + target.String() + "; reference retained", Error: true, Missing: true})
			return
		}
		compatible := false
		switch channel {
		case "normal":
			compatible = info.Role == RoleNormal || (info.Role == RoleHeight && info.GenerateNormal)
		case "height":
			compatible = info.Role == RoleHeight
		case "emission":
			compatible = info.Role == RoleEmissionRGB || info.Role == RoleEmissionMask
		}
		if !compatible {
			add(field, "incompatible "+string(info.Role)+" source for "+channel)
		}
		w, h, pivot := width, height, m.EffectivePivot()
		if f, ok := m.Frame(frame); ok {
			w, h, pivot = f.Rect[2], f.Rect[3], f.EffectivePivot()
		}
		if w > 0 && h > 0 && (w != info.Width || h != info.Height) {
			add(field, fmt.Sprintf("logical size %dx%d differs from channel %dx%d; no implicit stretching", w, h, info.Width, info.Height))
		}
		if pivot != info.Pivot {
			add(field, "channel source pivot differs from the albedo logical pivot")
		}
	}
	for _, channel := range []string{"normal", "height", "emission"} {
		if v, exists := m.Settings[channel]; exists {
			target, _, err := sourceAssetRef(v, true, channel)
			if err != nil {
				add(channel, err.Error())
			} else {
				check(m.ID, target, channel)
			}
		}
		frames, err := TextureFrameChannels(m.Settings, channel)
		if err != nil {
			add(channel+"_frames", err.Error())
			continue
		}
		frameIDs := make([]ids.AssetID, 0, len(frames))
		for frame := range frames {
			frameIDs = append(frameIDs, frame)
		}
		slices.SortFunc(frameIDs, func(a, b ids.AssetID) int { return strings.Compare(a.String(), b.String()) })
		for _, frame := range frameIDs {
			if _, exists := m.Frame(frame); exists {
				check(frame, frames[frame], channel)
			}
		}
	}
	return problems
}

// TextureSource describes one stable source/frame identity without loading GPU
// artifacts. Top-left is the source-image origin; rect is never atlas UV.
func TextureSource(m *SourceMeta, name string, width, height int, id ids.AssetID) (TextureSourceInfo, bool) {
	role, err := TextureRoleOf(name, m.Settings)
	if err != nil {
		return TextureSourceInfo{}, false
	}
	info := TextureSourceInfo{ID: id, SourceID: m.ID, Path: name, Width: width, Height: height, SourceWidth: width, SourceHeight: height, Rect: [4]int{0, 0, width, height}, Pivot: m.EffectivePivot(), Role: role, GenerateNormal: m.Settings.Bool("generate_normal", false)}
	if id == m.ID {
		return info, true
	}
	f, ok := m.Frame(id)
	if !ok {
		return TextureSourceInfo{}, false
	}
	info.Rect, info.Width, info.Height, info.Pivot = f.Rect, f.Rect[2], f.Rect[3], f.EffectivePivot()
	return info, true
}
