//go:build aego_editor && aego_gltest && linux && cgo && !aego_nonative

package docsview

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"aego/app/playground"
	"aego/engine/docs/democheck"
)

// These focused probes run real native and Chromium Try instances; they do
// not replace the separate all-component acceptance gate.
func TestCapsuleComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "physics.CapsuleCollider2D")
}

func TestPolygonComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "physics.PolygonCollider2D")
}

func TestCameraShakeComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "camera.CameraShake2D")
}

func TestEdgeComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "physics.EdgeCollider2D")
}

func TestLifetimeComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "gameplay.Lifetime")
}

func TestAnimationPlayerComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "animation.AnimationPlayer")
}

func TestAnimatorComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "animation.Animator")
}

func TestStateMachineComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "gameplay.StateMachine")
}

func TestMotionComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "gameplay.Motion2D")
}

func TestRevoluteComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "physics.RevoluteJoint2D")
}

func TestDistanceComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "physics.DistanceJoint2D")
}

func TestHitboxComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "gameplay.Hitbox2D")
}

func TestHurtboxComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "gameplay.Hurtbox2D")
}

func TestKillZoneComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "gameplay.KillZone2D")
}

func TestPickupComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "gameplay.Pickup2D")
}

func TestCheckpointComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "gameplay.Checkpoint2D")
}

func TestRespawnComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "gameplay.Respawn")
}

func TestAudioSourceComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "audio.AudioSource2D")
}

func TestAudioListenerComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "audio.AudioListener2D")
}

func TestAudioZoneComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "audio.AudioZone2D")
}

func TestPersistComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "save.Persist")
}

func TestTagsComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "engine.Tags")
}

func TestScriptComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "engine.Script")
}

func TestMaterialOverrideComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "engine.MaterialOverride")
}

func TestPrefabComponentDemoAcrossNativeAndChromium(t *testing.T) {
	testComponentAcrossNativeAndChromium(t, "engine.PrefabInstance")
}

func testComponentAcrossNativeAndChromium(t *testing.T, id string) {
	t.Helper()
	site := os.Getenv("AEGO_TRY_SITE")
	if site == "" {
		t.Skip("AEGO_TRY_SITE is unset: native+browser component probe NOT RUN")
	}
	site, err := filepath.Abs(site)
	if err != nil {
		t.Fatal(err)
	}
	script := os.Getenv("AEGO_TRY_BROWSER_RUNNER")
	if script == "" {
		script = filepath.Join("..", "..", "tools", "websmoke", "try-contract.mjs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	process, err := startBrowserProcess(ctx, site, script)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := process.close(); err != nil {
			t.Errorf("browser runner shutdown: %v", err)
		}
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := p.Docs().Get(id)
	if !ok || entry.Demo == nil {
		t.Fatalf("%s contract is absent", id)
	}
	var component democheck.Component
	for _, c := range p.Docs().ComponentDemos() {
		if c.ID == entry.ID {
			component = c
			break
		}
	}
	runner := combinedTryRunner{native: &nativeTryRunner{p: p}, browser: &browserTryRunner{p: p, process: process}}
	report := democheck.Check(ctx, []democheck.Component{component}, []democheck.Contract{*entry.Demo}, runner)
	if !report.Passed() {
		t.Fatalf("%s native/Chromium Try failed: %+v", id, report)
	}
	t.Logf("%s native/Chromium action, rendered change and Reset passed: %+v", id, report.Results[0])
}
