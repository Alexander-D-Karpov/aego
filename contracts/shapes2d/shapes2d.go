// Package shapes2d exchanges local contours without a dependency on a physics
// solver, Scene, the renderer or the editor. Providers belong to one world.
package shapes2d

import (
	"errors"

	"aego/core/ids"
	"aego/core/mathx"
)

const (
	ServiceID = "shapes2d.geometry"

	// These bounds apply to one reusable extraction buffer. A consumer may set
	// lower Limits; exceeding a bound returns an error without partial append.
	MaxContourPoints = 256
	MaxContours      = 4096
	MaxPoints        = 256 * 1024
)

var (
	ErrClosed       = errors.New("shape provider is closed")
	ErrMissingNode  = errors.New("shape source node does not exist")
	ErrMissingShape = errors.New("selected shape does not exist on the source node")
	ErrSpace        = errors.New("shape source must be in 2D space")
	ErrBudget       = errors.New("shape geometry exceeds its bounded buffer budget")
	ErrBuffer       = errors.New("shape geometry needs an output buffer")
)

// ID remains stable when other shapes are inserted or removed; Part is a provider-defined slot, not a result index. IDs and Node IDs are scoped to the provider's world.
type ID struct {
	Node ids.NodeID
	Part uint32
}

type Contour struct {
	ID       ID
	Owner    ids.NodeID // apply this node's presentation transform exactly once
	Key      string     // stable provider key, suitable for an explicit selection
	Revision uint64     // changes with local geometry, independently of transforms
	First    int
	Count    int
	Closed   bool
}

type Limits struct {
	Contours int
	Points   int
}

// Buffer owns its points; copied geometry never aliases a component, provider cache or physics world, and contour offsets survive Points reallocation.
// Results remain valid until the caller resets or mutates Buffer.
type Buffer struct {
	Contours []Contour
	Points   []mathx.Vec2
	Limits   Limits
}

func (b *Buffer) Reset() {
	clear(b.Contours)
	b.Contours = b.Contours[:0]
	b.Points = b.Points[:0]
}

// Vertices returns a view into the caller-owned extraction buffer.
func (b *Buffer) Vertices(c Contour) []mathx.Vec2 {
	if c.First < 0 || c.Count < 0 || c.First > len(b.Points) || c.Count > len(b.Points)-c.First {
		return nil
	}
	return b.Points[c.First : c.First+c.Count]
}

// Append copies an already validated contour. It checks sizes only; validating
// topology belongs at a geometry revision boundary, not in a warmed draw loop.
func (b *Buffer) Append(c Contour, points []mathx.Vec2) error {
	if b == nil {
		return ErrBuffer
	}
	contours, count := b.Limits.Contours, b.Limits.Points
	if contours <= 0 || contours > MaxContours {
		contours = MaxContours
	}
	if count <= 0 || count > MaxPoints {
		count = MaxPoints
	}
	if len(points) > MaxContourPoints || len(points) < 2 || c.Closed && len(points) < 3 ||
		len(b.Contours) >= contours || len(points) > count-len(b.Points) {
		return ErrBudget
	}
	c.First, c.Count = len(b.Points), len(points)
	b.Points = append(b.Points, points...)
	b.Contours = append(b.Contours, c)
	return nil
}

type Provider interface {
	// Append copies node-local geometry with collider offsets but no world/body transform; an empty key selects all shapes in stable provider order.
	// Disabled nodes append nothing without error; errors leave dst unchanged, and missing nodes/explicit keys remain diagnosable.
	Append(node ids.NodeID, key string, dst *Buffer) error
}
