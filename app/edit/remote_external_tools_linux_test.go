//go:build linux && !aego_runtime

package edit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/asset"
	audioimport "aego/asset/importers/audio"
	shaderimport "aego/asset/importers/shader"
	"aego/core/ids"
	"aego/core/log"
	"aego/project"
	rshader "aego/render/shader"
)

// The fake tools leave a marker if invoked. This exercises the actual editor
// startup and reimport pipeline, not just the importers in isolation.
func TestRemoteEditorNeverStartsImportTools(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "external-command-ran")
	for _, name := range []string{"glslangValidator", "ffmpeg"} {
		script := "#!/bin/sh\nprintf ran > \"$AEGO_REMOTE_TOOL_MARKER\"\nexit 1\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Setenv("AEGO_REMOTE_TOOL_MARKER", marker)
	t.Setenv("AEGO_REQUIRE_GLSLANG", "0")
	p, err := project.Create(t.TempDir(), "remote safety", false)
	if err != nil {
		t.Fatal(err)
	}
	shaderPath := filepath.Join(p.AssetsDir(), "sample.aeshader")
	if err := os.WriteFile(shaderPath, []byte("#pragma aego abi 1\nvoid fragment(){aego_out=vec4(1.0);}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.AssetsDir(), "sample.mp3"), []byte("not an audio file"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, Config{Headless: true, NoWatch: true, Logger: log.Nop(), RemoteProject: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	shader, _ := s.Importers.ByID("shader")
	audio, _ := s.Importers.ByID("audio")
	if v, ok := shader.(*shaderimport.Importer); !ok || !v.DisableExternal {
		t.Fatal("remote editor did not restrict the actual shader importer")
	}
	if v, ok := audio.(*audioimport.Importer); !ok || !v.DisableExternal {
		t.Fatal("remote editor did not restrict the actual audio importer")
	}
	var shaderID, audioID ids.AssetID
	for _, item := range s.Pipeline.Assets() {
		switch item.Path {
		case "assets/sample.aeshader":
			shaderID = item.ID
			if item.Error != "" {
				t.Fatalf("schema-checked shader import failed: %s", item.Error)
			}
		case "assets/sample.mp3":
			audioID = item.ID
			if !strings.Contains(item.Error, "disabled") {
				t.Fatalf("MP3 did not report disabled external conversion: %q", item.Error)
			}
		}
	}
	if shaderID.IsZero() || audioID.IsZero() {
		t.Fatal("fixture assets were not imported")
	}
	shaderEntry, ok := s.Loader.Index().Lookup(shaderID)
	if !ok {
		t.Fatal("shader was not in the prepared index")
	}
	var decoded *asset.ShaderArtifact
	for _, artifact := range shaderEntry.Artifacts {
		if artifact.Kind == asset.ArtShader {
			raw, err := s.Pipeline.Artifacts().ReadFile(artifact.Path)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err = asset.DecodeShader(raw)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if decoded == nil || decoded.Validation != rshader.SchemaChecked {
		t.Fatalf("unvalidated shader falsely advertised reference validation: %+v", decoded)
	}
	batch, err := s.Pipeline.Reimport(shaderID, audioID)
	if err != nil || batch.Failed[shaderID] != nil || batch.Failed[audioID] == nil {
		t.Fatalf("unexpected reimport result: %+v, %v", batch, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("external import tool was executed: %v", err)
	}
}

func TestRemoteNativeFlagIsRejectedBeforeSupervision(t *testing.T) {
	p, err := project.Create(t.TempDir(), "remote native", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := Start(Entry{Config: Config{RemoteProject: true, Recent: filepath.Join(t.TempDir(), "recent")}, Arg: p.Root}); err == nil || !strings.Contains(err.Error(), "native Go project") {
		t.Fatalf("native remote project reached editor/bootstrap: %v", err)
	}
}

func TestManagedRemoteReopenRejectsChangedNativeProject(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	host, _, _, entry := remoteFixtureAt(t, filepath.Join(config, "aego", "remote-examples"))
	root := filepath.Dir(filepath.Dir(entry))
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	p.Native = true
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p.NativeDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.NativeDir(), "native.go"), []byte("package native\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, managed, err := host.existingProject(root); err != nil || !managed {
		t.Fatalf("edited remote copy was not recognized: %v %v", managed, err)
	}
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "go-ran")
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\nprintf ran > \"$AEGO_REMOTE_TOOL_MARKER\"\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("AEGO_REMOTE_TOOL_MARKER", marker)
	err = Start(Entry{Config: Config{Recent: filepath.Join(t.TempDir(), "recent")}, Arg: root})
	if err == nil || !strings.Contains(err.Error(), "native Go project") {
		t.Fatalf("reopening edited remote project did not block native execution: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("native build was launched from the remote project: %v", err)
	}
}
