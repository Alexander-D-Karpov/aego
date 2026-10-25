package anim

import (
	"strings"
	"testing"

	"aego/format/aetext"
)

func decodeText(t *testing.T, src string, ctrl bool) error {
	t.Helper()
	n, err := aetext.Parse([]byte(src), "t")
	if err != nil {
		t.Fatal(err)
	}
	if ctrl {
		_, _, err = DecodeGraph(n)
	} else {
		_, err = Decode(n)
	}
	return err
}

// Every block of a clip refuses names it does not know instead of dropping them.
func TestClipRefusesUnknownNames(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"root", `anim version=1 length=1 speed=2 {}`, "unknown attribute speed of anim"},
		{"root block", "anim version=1 length=1 {\n  curve \"\" {}\n}\n", "unknown block curve"},
		{"track", "anim version=1 length=1 {\n  track \"\" prop=\"SpriteRenderer.size\" kind=vec2 loop=true {}\n}\n", "unknown attribute loop of track"},
		{"key", "anim version=1 length=1 {\n  track \"\" prop=\"SpriteRenderer.size\" kind=vec2 {\n    key t=0 v=[1, 1] ease=linear {}\n  }\n}\n", "unknown attribute ease of key"},
		{"frames", "anim version=1 length=1 {\n  frames \"Slash\" {\n    sprites=[asset(\"ae900000000000000000000000000b01\")]\n    start=0.08\n  }\n}\n", "start is an attribute of frames: write `frames \"Slash\" start=0.08 {`"},
		{"frames field", "anim version=1 length=1 {\n  frames \"\" {\n    sprites=[asset(\"ae900000000000000000000000000b01\")]\n    duration=[0.1]\n  }\n}\n", "unknown field duration of frames"},
		{"event", "anim version=1 length=1 {\n  event t=0.1 name=\"Hit\" when=1 {}\n}\n", "unknown attribute when of event"},
		{"event field", "anim version=1 length=1 {\n  event t=0.1 name=\"Hit\" {\n    payload=1\n  }\n}\n", "payload is an attribute of event"},
	}
	for _, c := range cases {
		err := decodeText(t, c.src, false)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

// Every block of a controller refuses names it does not know instead of dropping them.
func TestControllerRefusesUnknownNames(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"root", "controller version=1 start=\"Idle\" {\n  state \"Idle\" {}\n}\n", "unknown attribute start of controller"},
		{"root field", "controller version=1 {\n  ease=linear\n  state \"Idle\" {}\n}\n", "ease is an attribute of controller"},
		{"param", "controller version=1 {\n  param \"Speed\" kind=float min=0 {}\n}\n", "unknown attribute min of param"},
		{"state", "controller version=1 {\n  state \"Idle\" clipp=asset(\"a0a10000000000000000000000000005\") {}\n}\n", "unknown attribute clipp of state"},
		{"transition", "controller version=1 {\n  state \"A\" {}\n  state \"B\" {}\n  transition \"A\" to=\"B\" exit=1 {}\n}\n", "unknown attribute exit of transition"},
		{"cond", "controller version=1 {\n  param \"Speed\" kind=float {}\n  state \"A\" {}\n  state \"B\" {}\n  transition \"A\" to=\"B\" {\n    cond \"Speed\" op=gt val=1 {}\n  }\n}\n", "unknown attribute val of cond"},
	}
	for _, c := range cases {
		err := decodeText(t, c.src, true)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}
