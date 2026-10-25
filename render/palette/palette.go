package palette

import (
	"math"

	"aego/core/mathx"
)

type Palette struct {
	ID     string
	Name   string
	Author string
	Group  string
	Colors []uint32
}

const (
	GroupPixel = "Pixel art"
	GroupUI    = "UI and code"
	GroupBasic = "Basic"
)

func (p Palette) Linear() []mathx.Color {
	out := make([]mathx.Color, len(p.Colors))
	for i, c := range p.Colors {
		out[i] = mathx.ColorRGBA8(uint8(c>>16), uint8(c>>8), uint8(c), 255)
	}
	return out
}

func Builtin() []Palette {
	return builtin
}

func ByID(id string) (Palette, bool) {
	for _, p := range builtin {
		if p.ID == id {
			return p, true
		}
	}
	return Palette{}, false
}

var builtin = []Palette{
	{ID: "sweetie16", Name: "Sweetie 16", Author: "GrafxKid", Group: GroupPixel, Colors: []uint32{
		0x1a1c2c, 0x5d275d, 0xb13e53, 0xef7d57, 0xffcd75, 0xa7f070, 0x38b764, 0x257179,
		0x29366f, 0x3b5dc9, 0x41a6f6, 0x73eff7, 0xf4f4f4, 0x94b0c2, 0x566c86, 0x333c57,
	}},
	{ID: "pico8", Name: "PICO-8", Author: "Lexaloffle", Group: GroupPixel, Colors: []uint32{
		0x000000, 0x1d2b53, 0x7e2553, 0x008751, 0xab5236, 0x5f574f, 0xc2c3c7, 0xfff1e8,
		0xff004d, 0xffa300, 0xffec27, 0x00e436, 0x29adff, 0x83769c, 0xff77a8, 0xffccaa,
	}},
	{ID: "db16", Name: "DawnBringer 16", Author: "Richard Fhager", Group: GroupPixel, Colors: []uint32{
		0x140c1c, 0x442434, 0x30346d, 0x4e4a4e, 0x854c30, 0x346524, 0xd04648, 0x757161,
		0x597dce, 0xd27d2c, 0x8595a1, 0x6daa2c, 0xd2aa99, 0x6dc2ca, 0xdad45e, 0xdeeed6,
	}},
	{ID: "db32", Name: "DawnBringer 32", Author: "Richard Fhager", Group: GroupPixel, Colors: []uint32{
		0x000000, 0x222034, 0x45283c, 0x663931, 0x8f563b, 0xdf7126, 0xd9a066, 0xeec39a,
		0xfbf236, 0x99e550, 0x6abe30, 0x37946e, 0x4b692f, 0x524b24, 0x323c39, 0x3f3f74,
		0x306082, 0x5b6ee1, 0x639bff, 0x5fcde4, 0xcbdbfc, 0xffffff, 0x9badb7, 0x847e87,
		0x696a6a, 0x595652, 0x76428a, 0xac3232, 0xd95763, 0xd77bba, 0x8f974a, 0x8a6f30,
	}},
	{ID: "endesga32", Name: "Endesga 32", Author: "ENDESGA", Group: GroupPixel, Colors: []uint32{
		0xbe4a2f, 0xd77643, 0xead4aa, 0xe4a672, 0xb86f50, 0x733e39, 0x3e2731, 0xa22633,
		0xe43b44, 0xf77622, 0xfeae34, 0xfee761, 0x63c74d, 0x3e8948, 0x265c42, 0x193c3e,
		0x124e89, 0x0099db, 0x2ce8f5, 0xffffff, 0xc0cbdc, 0x8b9bb4, 0x5a6988, 0x3a4466,
		0x262b44, 0x181425, 0xff0044, 0x68386c, 0xb55088, 0xf6757a, 0xe8b796, 0xc28569,
	}},
	{ID: "slso8", Name: "SLSO8", Author: "Luis Miguel Maldonado", Group: GroupPixel, Colors: []uint32{
		0x0d2b45, 0x203c56, 0x544e68, 0x8d697a, 0xd08159, 0xffaa5e, 0xffd4a3, 0xffecd6,
	}},
	{ID: "oil6", Name: "Oil 6", Author: "GrafxKid", Group: GroupPixel, Colors: []uint32{
		0xfbf5ef, 0xf2d3ab, 0xc69fa5, 0x8b6d9c, 0x494d7e, 0x272744,
	}},
	{ID: "twilight5", Name: "Twilight 5", Author: "Star", Group: GroupPixel, Colors: []uint32{
		0xfbbbad, 0xee8695, 0x4a7a96, 0x333f58, 0x292831,
	}},
	{ID: "gameboy", Name: "Game Boy", Author: "Nintendo DMG", Group: GroupPixel, Colors: []uint32{
		0x0f380f, 0x306230, 0x8bac0f, 0x9bbc0f,
	}},
	{ID: "icecreamgb", Name: "Ice Cream GB", Author: "Kerrie Lake", Group: GroupPixel, Colors: []uint32{
		0x7c3f58, 0xeb6b6f, 0xf9a875, 0xfff6d3,
	}},
	{ID: "kirokazegb", Name: "Kirokaze Game Boy", Author: "Kirokaze", Group: GroupPixel, Colors: []uint32{
		0x332c50, 0x46878f, 0x94e344, 0xe2f3e4,
	}},
	{ID: "hollow", Name: "Hollow", Author: "Poltergasm", Group: GroupPixel, Colors: []uint32{
		0x0f0f1b, 0x565a75, 0xc6b7be, 0xfafbf6,
	}},
	{ID: "nord", Name: "Nord", Author: "Arctic Ice Studio", Group: GroupUI, Colors: []uint32{
		0x2e3440, 0x3b4252, 0x434c5e, 0x4c566a, 0xd8dee9, 0xe5e9f0, 0xeceff4, 0x8fbcbb,
		0x88c0d0, 0x81a1c1, 0x5e81ac, 0xbf616a, 0xd08770, 0xebcb8b, 0xa3be8c, 0xb48ead,
	}},
	{ID: "gruvbox", Name: "Gruvbox Dark", Author: "Pavel Pertsev", Group: GroupUI, Colors: []uint32{
		0x282828, 0xcc241d, 0x98971a, 0xd79921, 0x458588, 0xb16286, 0x689d6a, 0xa89984,
		0x928374, 0xfb4934, 0xb8bb26, 0xfabd2f, 0x83a598, 0xd3869b, 0x8ec07c, 0xebdbb2,
	}},
	{ID: "solarized", Name: "Solarized", Author: "Ethan Schoonover", Group: GroupUI, Colors: []uint32{
		0x002b36, 0x073642, 0x586e75, 0x657b83, 0x839496, 0x93a1a1, 0xeee8d5, 0xfdf6e3,
		0xb58900, 0xcb4b16, 0xdc322f, 0xd33682, 0x6c71c4, 0x268bd2, 0x2aa198, 0x859900,
	}},
	{ID: "catppuccin", Name: "Catppuccin Mocha", Author: "Catppuccin", Group: GroupUI, Colors: []uint32{
		0xf5e0dc, 0xf2cdcd, 0xf5c2e7, 0xcba6f7, 0xf38ba8, 0xeba0ac, 0xfab387, 0xf9e2af,
		0xa6e3a1, 0x94e2d5, 0x89dceb, 0x74c7ec, 0x89b4fa, 0xb4befe, 0xcdd6f4, 0x6c7086,
		0x313244, 0x1e1e2e, 0x181825, 0x11111b,
	}},
	{ID: "grays", Name: "Grays", Group: GroupBasic, Colors: ramp(9, func(t float64) (float64, float64, float64) { return t, t, t })},
	{ID: "hues", Name: "Hues", Group: GroupBasic, Colors: ramp(12, func(t float64) (float64, float64, float64) { return hsv(t*11/12, 0.75, 0.95) })},
	{ID: "pastel", Name: "Pastel", Group: GroupBasic, Colors: ramp(12, func(t float64) (float64, float64, float64) { return hsv(t*11/12, 0.3, 1) })},
	{ID: "skin", Name: "Skin Tones", Group: GroupBasic, Colors: []uint32{
		0xffdbc4, 0xf1c27d, 0xe0ac69, 0xc68642, 0xa0662f, 0x8d5524, 0x6b3f1d, 0x3d2210,
	}},
	{ID: "earth", Name: "Earth and Foliage", Group: GroupBasic, Colors: []uint32{
		0x2f2a1f, 0x4e3b2a, 0x7a5a3a, 0xa8835a, 0xd4b483, 0x3b5a2a, 0x5c8a3a, 0x8fbf5a,
		0xc5e08a, 0x2a4a5a, 0x4a7a8a, 0x8ab8c5,
	}},
}

func ramp(n int, f func(t float64) (float64, float64, float64)) []uint32 {
	out := make([]uint32, n)
	for i := range n {
		r, g, b := f(float64(i) / float64(n-1))
		q := func(v float64) uint32 { return uint32(math.Round(mathx.Clamp(v, 0, 1) * 255)) }
		out[i] = q(r)<<16 | q(g)<<8 | q(b)
	}
	return out
}

func hsv(h, s, v float64) (float64, float64, float64) {
	h = math.Mod(h, 1) * 6
	i := math.Floor(h)
	f := h - i
	p, q, t := v*(1-s), v*(1-s*f), v*(1-s*(1-f))
	switch int(i) {
	case 0:
		return v, t, p
	case 1:
		return q, v, p
	case 2:
		return p, v, t
	case 3:
		return p, q, v
	case 4:
		return t, p, v
	}
	return v, p, q
}
