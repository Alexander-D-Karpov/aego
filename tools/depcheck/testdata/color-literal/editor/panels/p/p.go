package p

import "aego/core/mathx"

var bad = mathx.Color{R: 1, G: 1, B: 1, A: 0.5}

var call = mathx.ColorRGB8(1, 2, 3)

var list = [...]mathx.Color{{R: 0.2, A: 1}}

var zero = mathx.Color{}

var tokens = [2]mathx.Color{zero, bad}
