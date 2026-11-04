package context

import "context"

// An adapter must decline a request it cannot fulfil within MaxCents; references are project PNGs explicitly selected by the user or agent.
type ImageGeneration struct {
	Prompt          string
	Count, MaxCents int
	References      [][]byte
}
type ImageGenerator interface {
	Name() string
	Generate(context.Context, ImageGeneration) ([][]byte, error)
}
