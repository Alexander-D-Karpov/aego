package editor

import (
	"testing"

	"aego/modules/animation"
)

func TestEventDefaultsAndValidation(t *testing.T) {
	events := []animation.EventDesc{{Name: "Event1"}, {Name: "Event3"}}
	if got := nextName(events); got != "Event2" {
		t.Fatalf("next event name = %q", got)
	}
	for _, tc := range []struct {
		name   string
		events []animation.EventDesc
		valid  bool
	}{
		{"empty list", nil, true},
		{"normal", []animation.EventDesc{{Name: "Land", Payload: animation.PayloadFloat}}, true},
		{"missing name", []animation.EventDesc{{}}, false},
		{"duplicate", []animation.EventDesc{{Name: "Land"}, {Name: "Land"}}, false},
		{"unknown payload", []animation.EventDesc{{Name: "Land", Payload: 200}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := validationMessage(&animation.Settings{Events: tc.events})
			if (got == "") != tc.valid {
				t.Fatalf("validation = %q, valid=%v", got, tc.valid)
			}
		})
	}
}
