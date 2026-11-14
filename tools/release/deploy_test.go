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
	if strings.Count(build, "-app-origin https://aeditor.akarpov.ru") != 2 ||
		strings.Contains(build, "https://aestudio.akarpov.ru") || !strings.Contains(build, "test ! -e site/studio") {
		t.Fatal("site must verify the dedicated Studio origin both before and after the build, without a local copy")
	}
}

func TestStudioDeploymentUsesDedicatedOrigin(t *testing.T) {
	configs := map[string][]string{
		"../../deploy/aeditor.akarpov.ru.conf":  {"server_name aeditor.akarpov.ru;", "return 301 https://aeditor.akarpov.ru$request_uri;", "root /var/www/aego-asset-studio/current;", "frame-src https://aeditor.akarpov.ru blob:;"},
		"../../deploy/edge/aeditor-static.conf": {"server_name aeditor.akarpov.ru;", "root /var/www/aego-asset-studio/current;", "frame-src https://aeditor.akarpov.ru blob:;"},
		"../../deploy/edge/aeditor-edge.conf":   {"server_name aeditor.akarpov.ru;", "proxy_ssl_name aeditor.akarpov.ru;", "proxy_ssl_verify on;", "proxy_set_header Host aeditor.akarpov.ru;"},
		"../../deploy/nginx.conf":               {"frame-src https://aeditor.akarpov.ru blob:;"},
		"../../deploy/edge/aego-edge.conf":      {"frame-src https://aeditor.akarpov.ru blob:;"},
	}
	for name, required := range configs {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			config := string(data)
			for _, want := range required {
				if !strings.Contains(config, want) {
					t.Errorf("%s lacks %q", name, want)
				}
			}
			if strings.Contains(config, "aestudio.akarpov.ru") {
				t.Errorf("%s references the wrong Studio origin", name)
			}
		})
	}
}

func TestProductionSiteImagePackagesReviewedExampleLicense(t *testing.T) {
	dockerfile, err := os.ReadFile("../../deploy/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	makefile, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	_, recipe, ok := strings.Cut(string(makefile), "site-headless:\n")
	if !ok {
		t.Fatal("missing site-headless recipe")
	}
	recipe, _, _ = strings.Cut(recipe, "\n\n")
	if !strings.Contains(recipe, "docs build") || !strings.Contains(recipe, "$(DOCS_BUILD_ARGS)") {
		t.Fatal("site-headless does not forward documentation build arguments")
	}
	for _, part := range []string{"DOCS_BUILD_ARGS=", "--examples-production", "--examples-license MIT", "--examples-license-file /src/docs/try-examples-license.txt"} {
		if !strings.Contains(string(dockerfile), part) {
			t.Fatalf("site image does not require reviewed example license input %q", part)
		}
	}
}
