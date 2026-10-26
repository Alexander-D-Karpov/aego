package run

import (
	"image"
	"image/color"
	"testing"
)

func TestParseInput(t *testing.T) {
	got, err := parseInput("30 press Jump; 10-60 hold Right\n90 release Right; 0-40 axis Move -0.5; 5 hold Fire")
	if err != nil {
		t.Fatal(err)
	}
	want := []inject{{30, "Jump", 1}, {31, "Jump", 0}, {10, "Right", 1}, {61, "Right", 0}, {90, "Right", 0}, {0, "Move", -0.5}, {41, "Move", 0}, {5, "Fire", 1}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d is %v, want %v", i, got[i], want[i])
		}
	}
	for _, bad := range []string{"30 jump", "x press Jump", "30 kick Ball", "40-10 hold Right", "3 axis Move"} {
		if _, err := parseInput(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestParseChecks(t *testing.T) {
	list, err := parseChecks([]string{"@120 Player/Head.Hurtbox2D.active == true", "Player.transform.position.y ~= 2.9", "exists Ledge", "@60 gone Coin", "Enemy.script/Enemy.hp < 3"})
	if err != nil {
		t.Fatal(err)
	}
	c := list[0]
	if c.frame != 120 || c.node != "Player/Head" || c.path != "Hurtbox2D.active" || c.op != "==" || c.want != "true" {
		t.Fatalf("first check is %+v", c)
	}
	if list[1].frame != -1 || list[2].exists != "exists" || list[3].exists != "gone" || list[3].frame != 60 || list[4].path != "script/Enemy.hp" {
		t.Fatalf("checks are %+v %+v %+v %+v", list[1], list[2], list[3], list[4])
	}
	for _, bad := range []string{"Player.x", "Player = 3", "@x Player.y == 1", "Player.y === 1"} {
		if _, err := parseChecks([]string{bad}); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestCompareWithin(t *testing.T) {
	if !compare(2.905, 2.9, "~=") || compare(2.95, 2.9, "~=") || !compare(1000.5, 1000, "~=") || !compare(1, 2, "<") || compare(2, 2, "!=") {
		t.Fatal("a comparison is wrong")
	}
}

func TestScaleDownKeepsTheAspect(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	for y := 0; y < 720; y++ {
		for x := 0; x < 1280; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x % 2 * 255), A: 255})
		}
	}
	dst := scaleDown(src, 640)
	if b := dst.Bounds(); b.Dx() != 640 || b.Dy() != 360 {
		t.Fatalf("scaled to %v", b)
	}
	if r := dst.RGBAAt(10, 10).R; r < 120 || r > 135 {
		t.Fatalf("two columns average to %d", r)
	}
	if scaleDown(src, 2000) != src {
		t.Fatal("a narrow image was scaled")
	}
}
