package art

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const MaxOperationBytes = 2 << 20

// DecodeOperations is the bounded wire decoder used by agent/CLI transports.
// The algorithms and transaction state remain in the ordinary art core.
func DecodeOperations(data []byte) ([]Operation, error) {
	if len(data) == 0 || len(data) > MaxOperationBytes {
		return nil, ErrLimit
	}
	var ops []Operation
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&ops); err != nil {
		return nil, fmt.Errorf("art: invalid operations: %w", err)
	}
	if len(ops) == 0 || len(ops) > 4096 {
		return nil, fmt.Errorf("art: proposal needs 1..4096 operations")
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("art: trailing operation data")
	}
	return ops, nil
}

// A batch can name a newly created entity with As and use "$name" in later
// entity fields. These are local result references, never external paths or
// selectors. No caller needs to predict the engine's stable entity IDs.
func resolveOperation(op Operation, names map[string]ID) (Operation, error) {
	resolve := func(id *ID) error {
		if !strings.HasPrefix(string(*id), "$") {
			return nil
		}
		name := string(*id)[1:]
		value, ok := names[name]
		if !ok || value == "" {
			return fmt.Errorf("art: unknown batch result %q", name)
		}
		*id = value
		return nil
	}
	for _, id := range []*ID{&op.Cel, &op.Layer, &op.Frame, &op.Parent} {
		if err := resolve(id); err != nil {
			return op, err
		}
	}
	op.FrameOrder = append([]ID(nil), op.FrameOrder...)
	for i := range op.FrameOrder {
		if err := resolve(&op.FrameOrder[i]); err != nil {
			return op, err
		}
	}
	op.Cells = append([]CellSlot(nil), op.Cells...)
	for i := range op.Cells {
		for _, id := range []*ID{&op.Cells[i].Layer, &op.Cells[i].Frame} {
			if err := resolve(id); err != nil {
				return op, err
			}
		}
	}

	if op.LayerValue != nil {
		v := *op.LayerValue
		op.LayerValue = &v
		if err := resolve(&v.ID); err != nil {
			return op, err
		}
		if err := resolve(&v.Parent); err != nil {
			return op, err
		}
	}
	if op.Tag != nil {
		v := *op.Tag
		op.Tag = &v
		for _, id := range []*ID{&v.ID, &v.From, &v.To} {
			if err := resolve(id); err != nil {
				return op, err
			}
		}
	}
	return op, nil
}
func validResultName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}
