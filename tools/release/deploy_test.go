package release

import (
	"os"
	"strings"
	"testing"
)

func TestSiteImageRequiresPinnedShaderValidator(t *testing.T) {
	dockerfile, err := os.ReadFile("../../deploy/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	version, err := os.ReadFile("../glslang.version")
	if err != nil {
		t.Fatal(err)
	}
	build := string(dockerfile)
	pinned := "glslang-tools=" + strings.TrimSpace(string(version)) + "-"
	if !strings.Contains(build, pinned) {
		t.Fatalf("site image must install the pinned reference validator (%s)", pinned)
	}
	check := strings.Index(build, "bash tools/check-glslang.sh")
	export := strings.Index(build, "make site-headless")
	if check < 0 || export < 0 || check > export {
		t.Fatal("site image must verify the pinned validator before exporting the site")
	}
	copyInstallers := strings.Index(build, "COPY tools/release/install.sh tools/release/install.ps1 site/")
	makeReadable := strings.Index(build, "RUN chmod 0644 site/install.sh site/install.ps1")
	if copyInstallers < 0 || makeReadable < copyInstallers {
		t.Fatal("site image must make the installer scripts publicly readable")
	}
}

func TestProductionSiteImageRequiresVerifiedStudioArtifact(t *testing.T) {
	dockerfile, err := os.ReadFile("../../deploy/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	build := string(dockerfile)
	for _, part := range []string{
		"ARTSTUDIO_MANIFEST_SHA256", "studio-artifact", "verify-site-input",
		"ARTSTUDIO_ARTIFACT=", "https://aeditor.akarpov.ru", "https://aego.akarpov.ru",
		"site/asset-studio/integration.json",
	} {
		if !strings.Contains(build, part) {
			t.Fatalf("site image does not require pinned release input %q", part)
		}
	}
	if strings.Index(build, "verify-site-input") > strings.Index(build, "make site-headless") {
		t.Fatal("site input must be verified before the build")
	}
}
