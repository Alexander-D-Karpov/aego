package texture

// EmissionMode describes prepared surface data, independently of the image's
// file extension or atlas placement. RGB is straight linear color after texture
// decoding; Mask is a scalar in R. Neither uses the texture's alpha as coverage.
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
