package particles

import (
	"errors"
	"math"

	"aego/asset"
	"aego/core/curve"
	"aego/core/mathx"
	"aego/render/sprite"
)

const MaxCapacity = 65536
const maxBursts = 128

type Shape uint8

const (
	Point Shape = iota
	Circle
	Rect
	Edge
	SpriteMask
)

type Space uint8

const (
	Local Space = iota
	World
)

type TimeDomain uint8

const (
	Game TimeDomain = iota
	Unscaled
)

type SortMode uint8

const (
	Age SortMode = iota
	YoungestFirst
	Distance
)

type ReducedMotionMode uint8

const (
	Keep ReducedMotionMode = iota
	Reduce
	Hide
)

type ColorKey struct {
	Time  float32     `aego:"time"`
	Color mathx.Color `aego:"color"`
}
type ParticleSystem2D struct {
	Prewarm          float32                    `aego:"prewarm,min=0,max=2"`
	EdgeA            mathx.Vec2                 `aego:"edgeA"`
	EdgeB            mathx.Vec2                 `aego:"edgeB"`
	Mask             asset.Ref[asset.Texture]   `aego:"mask,asset=texture,omitempty"`
	MaskThreshold    float32                    `aego:"maskThreshold,min=0,max=1"`
	Frames           []asset.Ref[asset.Texture] `aego:"frames,asset=texture,omitempty"`
	FrameFPS         float32                    `aego:"frameFPS,min=0"`
	FrameLoop        bool                       `aego:"frameLoop"`
	SortMode         SortMode                   `aego:"sortMode"`
	ReducedMotion    ReducedMotionMode          `aego:"reducedMotion"`
	ReducedRate      float32                    `aego:"reducedRate,min=0,max=1"`
	Drag             float32                    `aego:"drag,min=0"`
	NoiseStrength    float32                    `aego:"noiseStrength,min=0"`
	NoiseFrequency   float32                    `aego:"noiseFrequency,min=0"`
	KillYEnabled     bool                       `aego:"killYEnabled"`
	KillY            float32                    `aego:"killY"`
	PlaneEnabled     bool                       `aego:"planeEnabled"`
	PlaneNormal      mathx.Vec2                 `aego:"planeNormal"`
	PlaneDistance    float32                    `aego:"planeDistance"`
	ColorOverLife    []ColorKey                 `aego:"colorOverLife"`
	VelocityOverLife curve.Curve                `aego:"velocityOverLife"`
	PlayOnStart      bool                       `aego:"playOnStart"`
	Loop             bool                       `aego:"loop"`
	Duration         float32                    `aego:"duration,min=0"`
	TimeDomain       TimeDomain                 `aego:"timeDomain"`
	Seed             int64                      `aego:"seed"`
	MaxParticles     int32                      `aego:"maxParticles,min=1,max=65536"`
	Rate             float32                    `aego:"rate,min=0"`
	// Each key is (seconds within the cycle, birth count); equal times retain source order.
	Bursts        []mathx.Vec2              `aego:"bursts,min=0"`
	Shape         Shape                     `aego:"shape"`
	Offset        mathx.Vec2                `aego:"offset"`
	RectSize      mathx.Vec2                `aego:"rectSize,min=0"`
	Radius        float32                   `aego:"radius,min=0"`
	Lifetime      mathx.Vec2                `aego:"lifetime,minmax,min=0"`
	Speed         mathx.Vec2                `aego:"speed,minmax,min=0"`
	Size          mathx.Vec2                `aego:"size,minmax,min=0"`
	Rotation      mathx.Vec2                `aego:"rotation,minmax"`
	Direction     float32                   `aego:"direction"`
	Spread        float32                   `aego:"spread,min=0,max=6.2831855"`
	Color         mathx.Color               `aego:"color"`
	Gravity       mathx.Vec2                `aego:"gravity"`
	SizeOverLife  curve.Curve               `aego:"sizeOverLife"`
	AlphaOverLife curve.Curve               `aego:"alphaOverLife"`
	RotationSpeed float32                   `aego:"rotationSpeed"`
	Space         Space                     `aego:"space"`
	Sprite        asset.Ref[asset.Texture]  `aego:"sprite,asset=texture,omitempty"`
	Material      asset.Ref[asset.Material] `aego:"material,asset=material,omitempty"`
	Layer         int32                     `aego:"layer"`
	Order         int32                     `aego:"order"`
}

func DefaultParticleSystem2D() ParticleSystem2D {
	return ParticleSystem2D{
		PlayOnStart: true, Loop: true, Duration: 1, MaxParticles: 64, Rate: 8,
		EdgeB: mathx.V2(1, 0), MaskThreshold: .5, FrameFPS: 12, FrameLoop: true, ReducedRate: .3, NoiseFrequency: 1, PlaneNormal: mathx.V2(0, 1), VelocityOverLife: curve.Constant(1),
		RectSize: mathx.V2(1, 1), Radius: .5, Lifetime: mathx.V2(.4, .7),
		Speed: mathx.V2(.5, 1.5), Size: mathx.V2(.08, .16),
		Direction: -mathx.Pi / 2, Spread: mathx.Pi / 2, Color: mathx.White,
		SizeOverLife: curve.Constant(1), AlphaOverLife: curve.Line(0, 1, 1, 0),
	}
}

func finite(v float32) bool      { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }
func interval(v mathx.Vec2) bool { return finite(v.X) && finite(v.Y) && v.X <= v.Y }
func finiteCurve(c *curve.Curve) bool {
	if len(c.Keys) > 256 {
		return false
	}
	for i, k := range c.Keys {
		if !finite(k.T) || !finite(k.V) || !finite(k.In) || !finite(k.Out) || k.T < 0 || k.T > 1 || (i > 0 && c.Keys[i-1].T >= k.T) {
			return false
		}
	}
	return true
}
func (c *ParticleSystem2D) Validate() error {
	switch {
	case c.MaxParticles < 1 || c.MaxParticles > MaxCapacity:
		return errors.New("maxParticles must be between 1 and 65536")
	case !interval(c.Lifetime) || c.Lifetime.X <= 0:
		return errors.New("lifetime must be an ordered positive range")
	case !interval(c.Speed) || c.Speed.X < 0:
		return errors.New("speed must be an ordered nonnegative range")
	case !interval(c.Size) || c.Size.X < 0:
		return errors.New("size must be an ordered nonnegative range")
	case !interval(c.Rotation) || !finite(c.RotationSpeed):
		return errors.New("invalid rotation or rotationSpeed")
	case !finite(c.Rate) || c.Rate < 0:
		return errors.New("rate must be finite and nonnegative")
	case !finite(c.Duration) || c.Duration <= 0:
		return errors.New("duration must be finite and positive")
	case !finite(c.Direction) || !finite(c.Spread) || c.Spread < 0 || c.Spread > 2*mathx.Pi:
		return errors.New("spread must be in [0, 2π] and direction must be finite")
	case !finite(c.Gravity.X) || !finite(c.Gravity.Y) || !finite(c.Offset.X) || !finite(c.Offset.Y):
		return errors.New("invalid gravity or shape offset")
	case !finite(c.Radius) || c.Radius < 0 || !finite(c.RectSize.X) || !finite(c.RectSize.Y) || c.RectSize.X < 0 || c.RectSize.Y < 0:
		return errors.New("shape dimensions must be finite and nonnegative")
	case !finite(c.Prewarm) || c.Prewarm < 0 || c.Prewarm > 2:
		return errors.New("prewarm must be between 0 and 2 seconds")
	case !finiteVec(c.EdgeA) || !finiteVec(c.EdgeB) || !unitNumber(c.MaskThreshold):
		return errors.New("invalid edge or mask threshold")
	case !finite(c.FrameFPS) || c.FrameFPS < 0 || len(c.Frames) > 1024 || (len(c.Frames) > 1 && c.FrameFPS == 0):
		return errors.New("flipbooks require a finite positive fps and at most 1024 frames")
	case c.SortMode > Distance || c.ReducedMotion > Hide || !unitNumber(c.ReducedRate):
		return errors.New("invalid particle sorting or reduced motion policy")
	case !finite(c.Drag) || c.Drag < 0 || !finite(c.NoiseStrength) || c.NoiseStrength < 0 || !finite(c.NoiseFrequency) || c.NoiseFrequency < 0:
		return errors.New("drag, noise strength and frequency must be finite and nonnegative")
	case !finite(c.KillY) || !finite(c.PlaneDistance) || !finiteVec(c.PlaneNormal) || (c.PlaneEnabled && c.PlaneNormal.X == 0 && c.PlaneNormal.Y == 0):
		return errors.New("invalid particle kill boundary")
	case len(c.ColorOverLife) > 256 || !finiteCurve(&c.VelocityOverLife):
		return errors.New("invalid over-life curve or gradient")
	case c.Shape > SpriteMask || c.Space > World || c.TimeDomain > Unscaled:
		return errors.New("unknown particle shape, space or time domain")
	case c.Layer < int32(sprite.LayerMin) || c.Layer > int32(sprite.LayerMax) || c.Order < int32(sprite.OrderMin) || c.Order > int32(sprite.OrderMax):
		return errors.New("particle layer and order must fit the sprite painter key")
	case !finite(c.Color.R) || !finite(c.Color.G) || !finite(c.Color.B) || !finite(c.Color.A) || c.Color.A < 0 || c.Color.A > 1:
		return errors.New("color must be finite with alpha in [0,1]")
	case !finiteCurve(&c.SizeOverLife) || !finiteCurve(&c.AlphaOverLife):
		return errors.New("life curves require finite ordered keys within [0,1]")
	case len(c.Bursts) > maxBursts:
		return errors.New("at most 128 burst keys per cycle")
	}
	for i, k := range c.ColorOverLife {
		if !unitNumber(k.Time) || (i > 0 && c.ColorOverLife[i-1].Time >= k.Time) || !finiteColor(k.Color) {
			return errors.New("gradient requires finite colors and ordered times within [0,1]")
		}
	}
	for i, k := range c.Bursts {
		if !finite(k.X) || k.X < 0 || k.X >= c.Duration || !finite(k.Y) || k.Y < 0 || k.Y > MaxCapacity || math.Trunc(float64(k.Y)) != float64(k.Y) || (i > 0 && c.Bursts[i-1].X > k.X) {
			return errors.New("burst keys require ordered times in [0,duration) and integer counts in [0,65536]")
		}
	}
	return nil
}

// InvalidFields identifies individual authoring fields that violate the same
// constraints as Validate. It does not change the emitter or discard data.
func (c *ParticleSystem2D) InvalidFields() map[string]bool {
	bad := make(map[string]bool)
	mark := func(condition bool, names ...string) {
		if condition {
			for _, name := range names {
				bad[name] = true
			}
		}
	}
	mark(c.MaxParticles < 1 || c.MaxParticles > MaxCapacity, "maxParticles")
	mark(!interval(c.Lifetime) || c.Lifetime.X <= 0, "lifetime")
	mark(!interval(c.Speed) || c.Speed.X < 0, "speed")
	mark(!interval(c.Size) || c.Size.X < 0, "size")
	mark(!interval(c.Rotation), "rotation")
	mark(!finite(c.RotationSpeed), "rotationSpeed")
	mark(!finite(c.Rate) || c.Rate < 0, "rate")
	mark(!finite(c.Duration) || c.Duration <= 0, "duration")
	mark(!finite(c.Direction), "direction")
	mark(!finite(c.Spread) || c.Spread < 0 || c.Spread > 2*mathx.Pi, "spread")
	mark(!finiteVec(c.Gravity), "gravity")
	mark(!finiteVec(c.Offset), "offset")
	mark(!finite(c.Radius) || c.Radius < 0, "radius")
	mark(!finiteVec(c.RectSize) || c.RectSize.X < 0 || c.RectSize.Y < 0, "rectSize")
	mark(!finite(c.Prewarm) || c.Prewarm < 0 || c.Prewarm > 2, "prewarm")
	mark(!finiteVec(c.EdgeA), "edgeA")
	mark(!finiteVec(c.EdgeB), "edgeB")
	mark(!unitNumber(c.MaskThreshold), "maskThreshold")
	mark(!finite(c.FrameFPS) || c.FrameFPS < 0 || len(c.Frames) > 1 && c.FrameFPS == 0, "frameFPS")
	mark(len(c.Frames) > 1024, "frames")
	mark(c.SortMode > Distance, "sortMode")
	mark(c.ReducedMotion > Hide, "reducedMotion")
	mark(!unitNumber(c.ReducedRate), "reducedRate")
	mark(!finite(c.Drag) || c.Drag < 0, "drag")
	mark(!finite(c.NoiseStrength) || c.NoiseStrength < 0, "noiseStrength")
	mark(!finite(c.NoiseFrequency) || c.NoiseFrequency < 0, "noiseFrequency")
	mark(!finite(c.KillY), "killY")
	mark(!finite(c.PlaneDistance), "planeDistance")
	mark(!finiteVec(c.PlaneNormal) || c.PlaneEnabled && c.PlaneNormal.X == 0 && c.PlaneNormal.Y == 0, "planeNormal")
	mark(len(c.ColorOverLife) > 256, "colorOverLife")
	for i, k := range c.ColorOverLife {
		mark(!unitNumber(k.Time) || i > 0 && c.ColorOverLife[i-1].Time >= k.Time || !finiteColor(k.Color), "colorOverLife")
	}
	mark(!finiteCurve(&c.VelocityOverLife), "velocityOverLife")
	mark(!finiteCurve(&c.SizeOverLife), "sizeOverLife")
	mark(!finiteCurve(&c.AlphaOverLife), "alphaOverLife")
	mark(c.Shape > SpriteMask, "shape")
	mark(c.Space > World, "space")
	mark(c.TimeDomain > Unscaled, "timeDomain")
	mark(c.Layer < int32(sprite.LayerMin) || c.Layer > int32(sprite.LayerMax), "layer")
	mark(c.Order < int32(sprite.OrderMin) || c.Order > int32(sprite.OrderMax), "order")
	mark(!finiteColor(c.Color), "color")
	mark(len(c.Bursts) > maxBursts, "bursts")
	for i, k := range c.Bursts {
		mark(!finite(k.X) || k.X < 0 || k.X >= c.Duration || !finite(k.Y) || k.Y < 0 || k.Y > MaxCapacity || math.Trunc(float64(k.Y)) != float64(k.Y) || i > 0 && c.Bursts[i-1].X > k.X, "bursts")
	}
	return bad
}

func finiteVec(v mathx.Vec2) bool { return finite(v.X) && finite(v.Y) }
func finiteColor(v mathx.Color) bool {
	return finite(v.R) && finite(v.G) && finite(v.B) && unitNumber(v.A)
}
func unitNumber(v float32) bool { return finite(v) && v >= 0 && v <= 1 }
func (c *ParticleSystem2D) FrameIndex(age float32) int {
	n := len(c.Frames)
	if n == 0 {
		return -1
	}
	if age <= 0 || !finite(age) {
		return 0
	}
	frame := math.Floor(float64(age) * float64(c.FrameFPS))
	if c.FrameLoop {
		return int(math.Mod(frame, float64(n)))
	}
	return int(min(frame, float64(n-1)))
}
func GradientAt(keys []ColorKey, t float32) mathx.Color {
	if len(keys) == 0 {
		return mathx.White
	}
	if t <= keys[0].Time {
		return keys[0].Color
	}
	for i := 1; i < len(keys); i++ {
		k := keys[i]
		if t <= k.Time {
			prev := keys[i-1]
			return prev.Color.Lerp(k.Color, (t-prev.Time)/(k.Time-prev.Time))
		}
	}
	return keys[len(keys)-1].Color
}
