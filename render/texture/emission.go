package texture

// EmissionMode describes prepared data independent of file extension and atlas placement: RGB is straight linear after decoding, Mask is R; neither uses alpha as coverage.
type EmissionMode uint8

const (
	EmissionNone EmissionMode = iota
	EmissionRGB
	EmissionMask
)

type Emission struct {
	Region Region
	Mode   EmissionMode
}

func (e Emission) Valid() bool {
	return !e.Region.IsZero() && (e.Mode == EmissionRGB || e.Mode == EmissionMask)
}
