package host

import (
	"image"
	"testing"

	"aego/art"
)

func TestReceiptGestureAndCloseIsolation(t *testing.T) {
	h := New()
	a := document(t, h, "first.aeart")
	b := document(t, h, "second.aeart")
	sa, sb := session(t, h, a), session(t, h, b)
	draw(t, sa, 1)
	draw(t, sb, 2)
	first, second := export(t, h, a), export(t, h, b)
	if err := sb.BeginGesture("pencil", image.Pt(3, 1), art.Brush{Size: 1}, false); err != nil {
		t.Fatal(err)
	}
	if err := h.BeginWrite(second.Receipt); err == nil {
		t.Fatal("write began during the target document's active gesture")
	}
	if _, err := h.Export(b, "aeart", 1); err == nil {
		t.Fatal("export captured an unfinished gesture")
	}
	if err := h.Close(a, true); err != nil {
		t.Fatal(err)
	}
	if err := h.BeginWrite(first.Receipt); err == nil {
		t.Fatal("closing one tab did not revoke its receipt")
	}
	if h.Active() != b || !sb.Info().Gesture || !sb.Info().Dirty {
		t.Fatal("closing another tab changed the gesture or dirty state")
	}
	sb.CancelGesture()
	if err := h.BeginWrite(second.Receipt); err != nil {
		t.Fatal("another tab's closure or a rejected write revoked the receipt", err)
	}
	if err := h.Saved(second.Receipt); err != nil || sb.Info().Dirty {
		t.Fatal("completed write did not save its original tab", err)
	}
}
