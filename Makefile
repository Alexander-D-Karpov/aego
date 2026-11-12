GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
NATIVE := .cache/native/$(GOOS)_$(GOARCH)
SDL_VERSION := $(shell awk -F= '$$1=="version"{print $$2}' third_party/sdl/version.txt)
STAMP := $(NATIVE)/.sdl-$(SDL_VERSION)
SMOKE_FRAMES ?= 600
DEMO ?= basic
UI_FONT ?= $(CURDIR)/third_party/fonts/Inter-Regular.ttf
DOCS_REPO ?= https://git.akarpov.ru/sanspie/aego
DOCS_BUILD_ARGS ?=
FUZZTIME ?= 15s
STATICCHECK_VERSION ?= v0.8.2
# Benchmarks run pinned to one CPU.
BENCH_COUNT ?= 10
BENCH_CPU ?= 2
BENCH_PIN = $(shell command -v taskset >/dev/null && echo taskset -c $(BENCH_CPU))
EDITOR_VERSION ?= 0.1.0
EDITOR_ICON ?= app/brand/png
EXPORT_SMOKE = $(CURDIR)/.cache/export-smoke
EXPORT_HOME = $(CURDIR)/.cache/export-home
CHROMIUM ?= $(shell command -v chromium || command -v chromium-browser || command -v google-chrome)
WEB_SMOKE = cd tools/websmoke && npm ci --no-audit --no-fund && CHROMIUM=$(CHROMIUM) node smoke.mjs --site=$(CURDIR)/site
WASM_BUDGET = 8388608
WEB_DEMO_PROJECT ?= examples/platformer
WEB_DEMO = cd tools/websmoke && CHROMIUM=$(CHROMIUM) node demo.mjs --site=$(CURDIR)/site
WEB_DEMO_EXPORT = sh tools/site-demo.sh site/demo
WEB_DEMO_BUDGET = @size=$$(stat -c %s site/demo/game.wasm.gz); echo "demo game.wasm.gz: $$size bytes"; \
	if [ $$size -gt $(WASM_BUDGET) ]; then echo "the web demo is over the 8 MB gzip budget"; exit 1; fi
TRY_BUDGET = 10485760
WEB_TRY = cd tools/websmoke && CHROMIUM=$(CHROMIUM) node try.mjs --site=$(CURDIR)/site
WEB_AUDIO = cd tools/websmoke && CHROMIUM=$(CHROMIUM) node audio.mjs --site=$(CURDIR)/site
WEB_THEME = cd tools/websmoke && CHROMIUM=$(CHROMIUM) node theme.mjs --site=$(CURDIR)/site --out=$(CURDIR)/.cache/web-theme
WEB_BENCH_BROWSERS ?= chromium,firefox
WEB_BENCH_ARGS ?=
WEB_BENCH_BASELINE ?= $(CURDIR)/.cache/web-bench.base.json
WEB_STAGE12_BENCH_RUNS ?= 1
WEB_BENCH = cd tools/websmoke && CHROMIUM=$(CHROMIUM) node bench.mjs --site=$(CURDIR)/site \
	--replay=$(CURDIR)/tests/integration/testdata/platformer_run.aeinput \
	--out=$(CURDIR)/.cache/web-bench.json --baseline=$(WEB_BENCH_BASELINE)
TRY_SIZE_CHECK = size=$$(stat -c %s site/try/aego-docs.wasm.gz); echo "aego-docs.wasm.gz: $$size bytes"; \
	if [ $$size -gt $(TRY_BUDGET) ]; then echo "the docs runtime is over the 10 MB gzip budget, see make try-size"; exit 1; fi
# Module targets, appended by modules/*/module.mk.
MODULE_CI :=
MODULE_BENCH_ALLOCS :=
MODULE_BENCH_BASELINE :=
# Extra bench-compare scenarios, "package:pattern".
# Scenarios of one package share a .bench file, joined into one -test.bench regexp.
MODULE_BENCH_COMPARE := ./tests/integration:RewindScrub
BENCH_PHYSICS_PKGS :=
.DEFAULT_GOAL := bootstrap
-include modules/*/module.mk
include tools/artstudio/release/release.mk

.PHONY: materials materials-check dmg nsis-check test-update clean-root appimage bootstrap build test test-nonative test-editor test-runtime test-gl vet lint depcheck smoke run genfont genicons icons icons-doc res genpreviews golden golden-update site-headless bench bench-baseline bench-scripts bench-build bench-compare bench-profile bench-scripts-run bench-allocs playground playground-size site site-check cli example example-platformer check-all ci clean fuzz runtime export-smoke editor-app shaders test-wasm web-smoke web-smoke-update web-bench web-bench-baseline try-size shots shots-check affected quick check-camera check-audio check-gameplay check-mutate check-editor check-web check-docs i18n-check pkgsize

# Run in a protected snapshot: site and npm ci replace generated output.
COMPONENT_TRY_SITE ?= $(CURDIR)/site
COMPONENT_TRY_REPORT ?= $(CURDIR)/.cache/component-try.json
COMPONENT_TRY_EVIDENCE ?= $(CURDIR)/.cache/component-try-evidence
.PHONY: component-try
component-try: site
	cd tools/websmoke && npm ci --no-audit --no-fund
	CHROMIUM=$(CHROMIUM) AEGO_TRY_SITE=$(COMPONENT_TRY_SITE) AEGO_DEMOCHECK_REPORT=$(COMPONENT_TRY_REPORT) AEGO_DEMOCHECK_EVIDENCE=$(COMPONENT_TRY_EVIDENCE) $(GL_ENV) bash tools/stage125-check.sh component-try

# Real Linux GIO dispatch, in a temporary HOME/XDG profile, without confirming
# or installing a remote project. The native UI confirmation is a separate gate.
REMOTE_EXAMPLES_OUT ?= $(CURDIR)/.cache/remote-examples/os
.PHONY: remote-examples-check
remote-examples-check: cli
	python3 -B -m unittest discover -s tools/ci -p test_protocol_install.py
	$(GL_ENV) python3 -B tools/ci/remote_protocol_linux.py --binary "$(CURDIR)/.cache/bin/aego" --out "$(REMOTE_EXAMPLES_OUT)"

XVFB := $(shell command -v xvfb-run 2>/dev/null)
GL_OVERRIDE = MESA_GL_VERSION_OVERRIDE=3.3 MESA_GLSL_VERSION_OVERRIDE=330
ifneq ($(XVFB),)
GL_ENV = $(XVFB) -a -s "-screen 0 1280x720x24" env SDL_VIDEODRIVER=x11 SDL_AUDIO_DRIVER=dummy LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe $(GL_OVERRIDE)
else
GL_ENV = env SDL_VIDEODRIVER=offscreen SDL_AUDIO_DRIVER=dummy LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe $(GL_OVERRIDE)
endif

NONATIVE_PKGS = $$(go list ./... | grep -v -e '/platform/sdl$$' -e '/gpu/gl33$$' -e '/gpu/gl33/gl$$')
LINT_PKGS = $$(go list ./... | grep -v -e '/gpu/gl33/gl$$' -e '/audio/codec/vorbis/internal/')

bootstrap: $(STAMP)

$(STAMP): third_party/sdl/version.txt tools/bootstrap-native.sh
	bash tools/bootstrap-native.sh

build: bootstrap
	go build ./...

# Every package minus the dev-only ones, as defined by depcheck runtimeForbidden.
RUNTIME_PKGS = $$(go run -tags aego_nonative ./tools/depcheck -runtime-pkgs .)
WASM_EXEC = $(shell go env GOROOT)/lib/wasm/go_js_wasm_exec
WASM_TEST_PKGS = ./tests/wasm ./platform/input ./gpu/internal/glstate ./audio/workerproto

# The full integration package can exceed a cumulative 30-minute -race limit
# on smaller runners. Keep that timeout per sequential group, compile once,
# and verify every ordinary top-level test/Example/Fuzz seed is selected once.
.PHONY: test-race-packages test-race-integration
.NOTPARALLEL: test
test: export SDL_AUDIO_DRIVER := dummy
test: export SDL_AUDIODRIVER := dummy
test: test-race-packages test-race-integration

test-race-packages: bootstrap
	@set -eu; \
	packages="$$(go list ./...)"; \
	printf '%s\n' "$$packages" | grep -qx 'aego/tests/integration'; \
	others="$$(printf '%s\n' "$$packages" | grep -vx 'aego/tests/integration')"; \
	[ -n "$$others" ]; \
	go test -race -timeout 30m $$others

test-race-integration: bootstrap
	python3 -B -m unittest discover -s tools/ci -p test_integration_race.py
	python3 -B tools/ci/integration_race.py $(if $(filter 1,$(AEGO_INTEGRATION_ALLOW_UNBORN)),--allow-unborn,)

test-editor: bootstrap
	go build -tags aego_editor ./...
	go test -tags aego_editor ./editor/... ./app/... ./sdk/... ./modules/...

test-runtime: bootstrap
	go build -tags aego_runtime $(RUNTIME_PKGS)
	go test -tags aego_runtime $(RUNTIME_PKGS)

# Engine on gpu/null under Node, plus the js packages compile (D289).
test-wasm:
	GOOS=js GOARCH=wasm go build ./cmd/aego-playground ./platform/web ./gpu/webgl2 ./engine
	GOOS=js GOARCH=wasm go build -tags aego_runtime -o /dev/null ./cmd/aego-runtime
	@if GOOS=js GOARCH=wasm go list -tags aego_runtime -deps ./cmd/aego-runtime | grep -E '^aego/(platform/sdl|gpu/gl33|asset/pipeline|asset/importers|editor)'; then echo "the web runtime pulls a native, pipeline or editor package"; exit 1; fi
	GOOS=js GOARCH=wasm go build -tags aego_editor -o /dev/null ./cmd/aego-try
	@if GOOS=js GOARCH=wasm go list -tags aego_editor -deps ./cmd/aego-try | grep -E '^aego/(platform/sdl|gpu/gl33|editor/shell|editor/panels/|asset/importers/script)' | grep -vE '^aego/editor/panels/(inspector|gameview)$$'; then echo "the docs runtime pulls a native, shell or extra panel package"; exit 1; fi
	GOOS=js GOARCH=wasm go vet ./platform/web ./gpu/webgl2
	GOOS=js GOARCH=wasm go test -exec=$(WASM_EXEC) $(WASM_TEST_PKGS)
	GOOS=js GOARCH=wasm go test -exec=$(WASM_EXEC) -run "^TestWebAudio" ./platform/web

test-nonative:
	go test -tags aego_nonative $(NONATIVE_PKGS)

# Tests needing a real GL context (offscreen SDL + llvmpipe).
test-gl: bootstrap
	$(GL_ENV) go test -tags aego_gltest ./tests/gl/
	$(GL_ENV) go test -tags aego_gltest -count=1 ./ui/richtext ./editor/panels/agent
	$(GL_ENV) go test -tags aego_gltest ./modules/render2d/editor -run '^TestArtAlphaPickerRenderedPixels$$' -count=1 -v
	$(GL_ENV) go test -tags aego_gltest ./modules/audio/editor -run '^Test(StereoWaveRenderedPixels|AudioTransportRenderedLayout)$$' -count=1 -v
	$(GL_ENV) go test -tags aego_gltest ./app/edit -count=1 -run '^Test((Material|Lighting|ImageAsset|ArtScene)AuthoringWorkflowNative|ArtTimelineDurationNative|ArtEffectTemplateNative|ArtWorkingAlphaShadowNative|AuthoredFlipbookParticleGameViewNative|VFXCanvasAndParticlePresetNative|PlatformerEditSourceArtNative|ArtPNGDoubleClickWithInspectorInAssetsDock|LinkedSourcePromptNativeLayout|PlatformerSlicedPNGCopyNative|PlatformerSlicedPNGGameUndoExportNative|RemoteProtocolLauncherNativeConfirmation|RemoteLauncherNativeConfirmation|RemoteLauncherClonesOverLocalTLS|CreateMenuFilePromptKeepsNativeKeyboardFocus|CreatePixelArtNativeEnterWaitsForConfirmation|AssetsEmptyGridContextMenuSDL|PlatformerVignetteGameView|PostFXEditPreviewWithoutPlay|CameraInspectorShowsRenderedPostStatus|PostFXAndParticlePresetThumbnails|ColdPostMaterialPreviewAndThumbnail|DiagnosticComposerConsoleProfilerNative|SceneViewCaptureAfterEndFrameGL|SceneViewCaptureOrientationMatchesDisplayGL|SceneView3DShows2DSpritePixels)$$'
	$(GL_ENV) go test -tags aego_editor,aego_gltest -count=1 -run '^(Test(Mesh2D|SpriteRenderer)TryNativeVisibleOrderAndReset|TestComponentContractProbeRunsNativeAndRejectsUnrunWeb|TestComponentContractObservationWarmupNative|TestTryGameViewportExcludesInspectorNative|TestPhysicsBackedComponentDemosNative|TestCameraShakeComponentDemoDrivesRealCamera|TestBrowserProcessDeadlineFailsClosed|TestControlledGuideRealNativeViewportAndReset)$$' ./app/docsview
	$(GL_ENV) SDL_AUDIO_DRIVER=dummy AEGO_PLATFORMER_SHOTS=$(CURDIR)/.cache/stage12/lighting-route AEGO_PARTICLE_SHOTS=$(CURDIR)/.cache/stage12/particle-route AEGO_POSTFX_SHOTS=$(CURDIR)/.cache/stage12/postfx go test -tags aego_gltest -count=1 -run GL\|Screenshots ./tests/integration/

vet: bootstrap
	go vet -cgocall=false -unsafeptr=false $(LINT_PKGS)
	go vet ./platform/sdl

lint: bootstrap
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) $(LINT_PKGS)
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) -tags aego_editor ./editor/... ./app/... ./tests/...

# All graph variants, the semantic pass and the negative rule tests.
depcheck:
	go run -tags aego_nonative ./tools/depcheck -v .
	go test -tags aego_nonative -count=1 ./tools/depcheck

smoke: bootstrap
	go run ./cmd/sandbox --demo headless --frames $(SMOKE_FRAMES) --strict-allocs --alloc-profile $(CURDIR)/.cache/traces

run: bootstrap
	go run ./cmd/sandbox --demo $(DEMO) $(ARGS)

# Regenerate the embedded bitmap font atlas into engine/res/fonts.
genfont:
	cd tools/genfont && go mod tidy && go run . -ttf $(UI_FONT)

# Regenerate the committed resources: the icon atlas and the template and example previews (D280).
res: genicons genpreviews shaders materials

# GLSL ES 3.00 copies of engine/res/shaders for WebGL2 (D290).
shaders:
	go run ./tools/genshaders

# Builtin materials use the same source importers and reference validator as project assets.
materials:
	go run -tags aego_nonative ./tools/genmaterials

materials-check:
	go run -tags aego_nonative ./tools/genmaterials -check

.PHONY: shader-validator
shader-validator:
	bash tools/check-glslang.sh

genpreviews: bootstrap
	$(GL_ENV) go run ./tools/genpreviews -root $(CURDIR)

# Regenerate the embedded icon atlas into engine/res/icons.
genicons:
	cd tools/genicons && go mod tidy && go run . -init -src "$$(cd ../.. && go run ./tools/iconsrc)"

golden: bootstrap
	$(GL_ENV) go test -tags aego_golden ./tests/golden/...

golden-update: bootstrap
	$(GL_ENV) go test -tags aego_golden ./tests/golden/... -update

BENCH_DIR = $(CURDIR)/.cache/bench
# Committed baselines; bench-allocs compares allocation counts only, so they hold on any machine.
BENCH_BASE_DIR = $(CURDIR)/tools/bench/baseline
BENCH_BASELINE = $(BENCH_BASE_DIR)/baseline.json
BENCH_OUT ?= $(BENCH_DIR)/out.txt
BENCH_GATE = go run ./tools/benchgate
BENCH_PKGS = ./script/vm ./script/coroutine ./script/signal ./script/runtime ./script/bind
BENCH_BASE ?= stage5

# Benchmark gate (D213).
bench: bootstrap
	go build -o .cache/bin/bench ./tools/bench
	$(BENCH_PIN) .cache/bin/bench -baseline $(BENCH_BASELINE) -count $(BENCH_COUNT)
	$(BENCH_PIN) .cache/bin/bench -demo stress-scene -baseline $(BENCH_BASELINE) -count $(BENCH_COUNT)

bench-baseline: bootstrap $(MODULE_BENCH_BASELINE)
	mkdir -p $(BENCH_DIR) $(BENCH_BASE_DIR)
	go build -o .cache/bin/bench ./tools/bench
	$(BENCH_PIN) .cache/bin/bench -baseline $(BENCH_BASELINE) -count $(BENCH_COUNT) -update
	$(BENCH_PIN) .cache/bin/bench -demo stress-scene -baseline $(BENCH_BASELINE) -count $(BENCH_COUNT) -update
	$(MAKE) bench-scripts-run BENCH_OUT=$(BENCH_BASE_DIR)/scripts.base.txt

bench-scripts-run:
	mkdir -p $(BENCH_DIR)
	$(BENCH_PIN) go test -run '^$$' -bench . -benchmem -count=$(BENCH_COUNT) $(BENCH_PKGS) > $(BENCH_OUT)

bench-scripts:
	$(MAKE) bench-scripts-run BENCH_OUT=$(BENCH_DIR)/scripts.txt
	$(BENCH_GATE) -base $(BENCH_BASE_DIR)/scripts.base.txt -head $(BENCH_DIR)/scripts.txt

# CI gate: allocations only, equal to the saved baseline.
bench-allocs: bootstrap $(MODULE_BENCH_ALLOCS)
	go build -o .cache/bin/bench ./tools/bench
	$(GL_ENV) .cache/bin/bench -baseline $(BENCH_BASELINE) -count 1 -time=false
	$(GL_ENV) .cache/bin/bench -demo stress-scene -baseline $(BENCH_BASELINE) -count 1 -time=false
	$(MAKE) bench-scripts-run BENCH_OUT=$(BENCH_DIR)/scripts.allocs.txt BENCH_COUNT=1
	$(BENCH_GATE) -time=false -base $(BENCH_BASE_DIR)/scripts.base.txt -head $(BENCH_DIR)/scripts.allocs.txt
	go test -run '^$$' -bench '^BenchmarkSpritesMaterials$$' -benchmem -count=1 ./render | tee $(BENCH_DIR)/materials.allocs.txt
	@awk '/^BenchmarkSpritesMaterials/ { seen++; for (i = 2; i <= NF; i++) if ($$i == "allocs/op" && $$(i-1) + 0 != 0) bad = 1 } END { if (seen != 1 || bad) { print "BenchmarkSpritesMaterials allocates or did not run"; exit 1 } }' $(BENCH_DIR)/materials.allocs.txt
	go test -run '^$$' -bench '^BenchmarkLightingRender$$' -benchmem -count=1 ./render | tee $(BENCH_DIR)/lighting.allocs.txt
	@awk '/^BenchmarkLightingRender\// { seen++; for (i = 2; i <= NF; i++) if ($$i == "allocs/op" && $$(i-1) + 0 != 0) bad = 1 } END { if (seen != 3 || bad) { print "BenchmarkLightingRender allocates or did not run all three scenarios"; exit 1 } }' $(BENCH_DIR)/lighting.allocs.txt


bench-build:
	rm -rf $(BENCH_DIR)/base $(BENCH_DIR)/base-bin $(BENCH_DIR)/head-bin $(BENCH_DIR)/base.txt $(BENCH_DIR)/head.txt
	mkdir -p $(BENCH_DIR)/base-bin $(BENCH_DIR)/head-bin
	git worktree prune
	git worktree add -f --detach $(BENCH_DIR)/base $(BENCH_BASE)
	cd $(BENCH_DIR)/base && for p in $(BENCH_PKGS); do [ -d $$p ] && go test -c -o $(BENCH_DIR)/base-bin/$$(basename $$p).test $$p; done; true
	cd $(BENCH_DIR)/base && for e in $(MODULE_BENCH_COMPARE); do p=$${e%%:*}; [ -d $$p ] && go test -c -o $(BENCH_DIR)/base-bin/$$(basename $$p).test $$p && f=$(BENCH_DIR)/base-bin/$$(basename $$p).bench && b=$${e#*:} && { [ ! -f $$f ] || { read ob od < $$f; b="$$ob|$$b"; }; } && echo "$$b $$p" > $$f; done; true
	git worktree remove --force $(BENCH_DIR)/base
	for p in $(BENCH_PKGS); do go test -c -o $(BENCH_DIR)/head-bin/$$(basename $$p).test $$p; done
	for e in $(MODULE_BENCH_COMPARE); do p=$${e%%:*}; go test -c -o $(BENCH_DIR)/head-bin/$$(basename $$p).test $$p && f=$(BENCH_DIR)/head-bin/$$(basename $$p).bench && b=$${e#*:} && { [ ! -f $$f ] || { read ob od < $$f; b="$$ob|$$b"; }; } && echo "$$b $$p" > $$f; done

bench-compare: bench-build
	for i in $$(seq $(BENCH_COUNT)); do \
		for v in base head; do \
			for t in $(BENCH_DIR)/$$v-bin/*.test; do \
				b=.; d=.; [ -f $${t%.test}.bench ] && read b d < $${t%.test}.bench; \
				(cd $$d && $(BENCH_PIN) $$t -test.run '^$$' -test.bench "$$b" -test.benchmem) >> $(BENCH_DIR)/$$v.txt; \
			done; \
		done; \
	done
	go run golang.org/x/perf/cmd/benchstat@latest $(BENCH_DIR)/base.txt $(BENCH_DIR)/head.txt
	$(BENCH_GATE) -base $(BENCH_DIR)/base.txt -head $(BENCH_DIR)/head.txt

bench-profile:
	[ -x $(BENCH_DIR)/base-bin/runtime.test ] || $(MAKE) bench-build
	go test -c -o $(BENCH_DIR)/head-bin/runtime.test ./script/runtime
	$(BENCH_PIN) $(BENCH_DIR)/base-bin/runtime.test -test.run '^$$' -test.bench 'Update2k$$' -test.benchtime 20000x -test.cpuprofile $(BENCH_DIR)/base.prof
	$(BENCH_PIN) $(BENCH_DIR)/head-bin/runtime.test -test.run '^$$' -test.bench 'Update2k$$' -test.benchtime 20000x -test.cpuprofile $(BENCH_DIR)/head.prof
	go tool pprof -top -nodecount 40 -diff_base $(BENCH_DIR)/base.prof $(BENCH_DIR)/head.prof

site: cli
	.cache/bin/aego docs build --out site --repo $(DOCS_REPO) --demo $(DOCS_BUILD_ARGS)
	$(WEB_DEMO_EXPORT) .cache/bin/aego export $(WEB_DEMO_PROJECT) --target web --release
	@size=$$(stat -c %s site/play/aego.wasm.gz); echo "aego.wasm.gz: $$size bytes"; \
	if [ $$size -gt $(WASM_BUDGET) ]; then echo "playground is over the 8 MB gzip budget, see make playground-size"; exit 1; fi
	$(WEB_DEMO_BUDGET)
	@$(TRY_SIZE_CHECK)

playground:
	@if GOOS=js GOARCH=wasm go list -deps ./cmd/aego-playground | grep -E '^aego/(platform/sdl|gpu/gl33|asset/pipeline|asset/importers)'; then echo "playground graph pulls a native or pipeline package"; exit 1; fi
	$(MAKE) site

site-headless:
	CGO_ENABLED=0 go build -tags aego_nonative -o .cache/bin/aego-docs ./cmd/aego
	.cache/bin/aego-docs docs build --out site --engine $(CURDIR) --repo $(DOCS_REPO) --demo $(DOCS_BUILD_ARGS)
	$(WEB_DEMO_EXPORT) .cache/bin/aego-docs export $(WEB_DEMO_PROJECT) --target web --release --engine $(CURDIR)
	@size=$$(stat -c %s site/play/aego.wasm.gz); echo "aego.wasm.gz: $$size bytes"; \
	if [ $$size -gt $(WASM_BUDGET) ]; then echo "playground is over the 8 MB gzip budget, see make playground-size"; exit 1; fi
	$(WEB_DEMO_BUDGET)
	@$(TRY_SIZE_CHECK)

site-check: cli
	.cache/bin/aego docs check --strict --lang ru
	.cache/bin/aego docs export --out $(CURDIR)/.cache/docs.json
	cd tools/gendocs && AEGO_DOCS_EXPORT=$(CURDIR)/.cache/docs.json go test -count=1 ./...

playground-size:
	mkdir -p .cache
	GOOS=js GOARCH=wasm go build -trimpath -o .cache/aego-playground.wasm ./cmd/aego-playground
	go run ./tools/wasmsize -top 40 .cache/aego-playground.wasm
	go run ./tools/wasmsize -top 15 -depth 2 -prev .cache/aego-playground.size.json .cache/aego-playground.wasm

try-size:
	mkdir -p .cache
	GOOS=js GOARCH=wasm go build -tags aego_editor -trimpath -o .cache/aego-try.wasm ./cmd/aego-try
	go run ./tools/wasmsize -top 40 .cache/aego-try.wasm
	go run ./tools/wasmsize -top 15 -depth 2 -prev .cache/aego-try.size.json .cache/aego-try.wasm

web-smoke: site
	sh tools/webbudget.sh site
	$(WEB_SMOKE) --report=$(CURDIR)/.cache/web-sprites.json
	$(WEB_DEMO)
	$(WEB_TRY)
	$(WEB_AUDIO)
	$(WEB_THEME)
	$(WEB_BENCH) --browsers=chromium --runs=1 --gate

# The old Stage 12 workload remains a separate gate against its unchanged
# baseline.  A single CI sample checks counts/allocations; use five to compare
# timing as well, just like web-bench.  It never changes the user's project.
.PHONY: web-bench-stage12 web-stage12-fixture-test
web-bench-stage12: cli
	python3 tools/websmoke/stage12_bench.py --runs=$(WEB_STAGE12_BENCH_RUNS)

web-stage12-fixture-test:
	PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tools/websmoke -p stage12_bench_test.py

web-bench: site
	cd tools/websmoke && npm ci --no-audit --no-fund
	$(WEB_BENCH) --browsers=$(WEB_BENCH_BROWSERS) --gate $(WEB_BENCH_ARGS)

web-bench-baseline: site
	cd tools/websmoke && npm ci --no-audit --no-fund
	$(WEB_BENCH) --browsers=$(WEB_BENCH_BROWSERS) --update-baseline $(WEB_BENCH_ARGS)

web-smoke-update: site
	$(WEB_SMOKE) --update
	$(WEB_DEMO) --update
	$(WEB_TRY) --update

cli: bootstrap
	go build -o .cache/bin/aego ./cmd/aego

icons-doc: cli
	.cache/bin/aego icons list --out docs/icons.md

icons: cli
	.cache/bin/aego icons check

example: cli
	.cache/bin/aego run examples/native-rotator --headless --frames 120

example-platformer: cli
	.cache/bin/aego run examples/platformer --headless --frames 600

check-all: cli
	.cache/bin/aego check examples/native-rotator
	.cache/bin/aego check examples/platformer
	.cache/bin/aego test examples/platformer
	.cache/bin/aego check examples/topdown
	.cache/bin/aego check examples/native-settings
	.cache/bin/aego check examples/native-editor
	.cache/bin/aego check app/playground/examples
	cd examples/native-editor && go vet -tags aego_editor ./...

# Fuzz the lexer and parser for FUZZTIME each.
fuzz:
	go test -run '^$$' -fuzz '^FuzzTokenize$$' -fuzztime $(FUZZTIME) ./script/lexer
	go test -run '^$$' -fuzz '^FuzzParse$$' -fuzztime $(FUZZTIME) ./script/parser
	go test -run '^$$' -fuzz '^FuzzDecodeVerify$$' -fuzztime $(FUZZTIME) ./script/verify
	go test -run '^$$' -fuzz '^FuzzDecodeVerifyV3$$' -fuzztime $(FUZZTIME) ./script/verify
	go test -run '^$$' -fuzz '^FuzzWAV$$' -fuzztime $(FUZZTIME) ./audio/codec
	go test -run '^$$' -fuzz '^FuzzVorbis$$' -fuzztime $(FUZZTIME) ./audio/codec


affected:
	@pkgs=$$(sh tools/affected.sh $(BASE)); if [ -z "$$pkgs" ]; then echo "nothing affected"; else go vet $$pkgs && go test $$pkgs; fi

quick:
	@pkgs=$$(sh tools/affected.sh $(BASE)); if [ -n "$$pkgs" ]; then go build -o $(CURDIR)/.cache/bin/affected/ $$pkgs && go test -short $$pkgs; fi

check-camera:
	go test ./modules/camera/... ./tests/camera/...

check-audio:
	go test ./audio/... ./modules/audio/... ./tests/audio/...

check-gameplay:
	go test ./modules/gameplay/... ./tests/gameplay/... ./modules/physics/...

check-mutate:
	go test ./scene/mutate/... ./tests/mutate/... ./app/agent/...

check-editor:
	go test ./editor/... ./app/edit/...

check-web: test-wasm web-smoke

check-docs: site-check

# The en catalog matches the code and every translation has the code's keys and placeholders (D348).
i18n-check:
	go run ./tools/i18n check

pkgsize:
	@sh tools/pkgsize.sh

# ci requires glslangValidator: shader imports and the shader tests fail without it instead of skipping (D367).
ci: export AEGO_REQUIRE_GLSLANG := 1
ci: export SDL_AUDIO_DRIVER := dummy
ci: export SDL_AUDIODRIVER := dummy
# Two known environment-sensitive web checks remain visible as warnings in CI;
# direct web-smoke and stage12-release-check runs stay strict by default.
ci: export AEGO_CI_WEB_WARN := 1
ci: export AEGO_CI_REQUIRE_WEB_BASELINE := 1
# The present platformer is a different workload than the Stage 12 fixture.
# Gate both independently; never compare changed content to the old counts.
ci: WEB_BENCH_BASELINE := $(CURDIR)/tools/websmoke/bench-platformer-head-baseline.json
.PHONY: web-warning-policy-test
web-warning-policy-test:
	node --test tools/websmoke/ci-warning-policy.test.mjs

.PHONY: web-bench-baseline-test
web-bench-baseline-test:
	node --test tools/websmoke/bench-baseline.test.mjs

.PHONY: web-audio-policy-test
web-audio-policy-test:
	node --test tools/websmoke/audio-gate.test.mjs tools/websmoke/audio-silence.test.cjs

.PHONY: web-visibility-lifecycle-test
web-visibility-lifecycle-test:
	node --test tools/websmoke/visibility-browser.test.mjs

.PHONY: web-private-xvfb-test
web-private-xvfb-test:
	node --test tools/websmoke/private-xvfb.test.mjs

.PHONY: web-try-pulse-test
web-try-pulse-test:
	node --test tools/websmoke/try-pulse.test.mjs

ci: web-warning-policy-test web-bench-baseline-test web-audio-policy-test web-visibility-lifecycle-test web-private-xvfb-test web-try-pulse-test web-stage12-fixture-test shader-validator materials-check build vet lint depcheck i18n-check test test-nonative test-editor test-runtime test-wasm runtime smoke fuzz golden shots-check test-gl cli playground web-smoke web-bench-stage12 site-check example example-platformer check-all export-smoke test-update $(MODULE_CI) bench-allocs
ci: studio-web-test
	@sh tools/ci-clean-root.sh

# test-update builds two versions of the CLI and updates one to the other through a release server on loopback (D362).
test-update: bootstrap
	SDL_AUDIO_DRIVER=dummy go test -tags aego_update -count=1 ./tests/update/

# clean-root fails when a build left a binary or a test binary in the checkout instead of .cache/bin.
clean-root:
	@sh tools/ci-clean-root.sh


# Documentation screenshots of the real editor on the platformer example, under a clean user profile.
SHOTS_TMP = $(CURDIR)/.cache/shots
ifneq ($(XVFB),)
SHOTS_ENV = $(XVFB) -a -s "-screen 0 1600x900x24" env SDL_VIDEODRIVER=x11 SDL_AUDIO_DRIVER=dummy LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe $(GL_OVERRIDE)
else
SHOTS_ENV = $(GL_ENV)
endif
# A new private path per invocation isolates editor locks, native builds and saved
# state even when repeated captures or other workspace processes overlap.
SHOTS_RUN = set -eu; mkdir -p "$(SHOTS_TMP)"; \
	shots_work="$$(mktemp -d '$(SHOTS_TMP)/$(or $(2),dark).XXXXXX')"; \
	trap 'rm -rf "$$shots_work"' 0; \
	mkdir -p "$$shots_work/home"; cp -R examples/platformer "$$shots_work/project"; \
	rm -rf "$$shots_work/project/.aego" "$$shots_work/project/exports" "$$shots_work/project/scripts/platform"; \
	cp tools/shots/testdata/main.aescene "$$shots_work/project/scenes/main.aescene"; \
	cp tools/shots/testdata/player_hurt.aeanim "$$shots_work/project/assets/anim/player_hurt.aeanim"; \
	sed -i.bak 's|=> ../..|=> $(CURDIR)|' "$$shots_work/project/go.mod"; \
	test ! -e "$$shots_work/project/.aego/editor.lock"; \
	XDG_CONFIG_HOME="$$shots_work/home" XDG_DATA_HOME="$$shots_work/home" $(SHOTS_ENV) env AEGO_LANG=en AEGO_SHOTS=$(1) AEGO_EDITOR_THEME=$(or $(2),dark) .cache/bin/aego edit "$$shots_work/project" --hidden

shots: cli
	rm -rf docs/shots
	$(call SHOTS_RUN,$(CURDIR)/docs/shots)
	$(call SHOTS_RUN,$(CURDIR)/docs/shots,light)

shots-check: cli
	rm -rf $(SHOTS_TMP)/out
	$(call SHOTS_RUN,$(SHOTS_TMP)/out)
	$(call SHOTS_RUN,$(SHOTS_TMP)/out,light)
	go run ./tools/shotdiff -want docs/shots -got $(SHOTS_TMP)/out

runtime: bootstrap
	go build -tags aego_runtime -o .cache/bin/aego-runtime ./cmd/aego-runtime

# Export the platformer and run it without Go, sources or the project cache (D286).
export-smoke: cli
	rm -rf $(EXPORT_SMOKE) $(EXPORT_HOME)
	mkdir -p $(EXPORT_HOME)
	.cache/bin/aego export examples/platformer --development --out $(EXPORT_SMOKE)
	cd $(EXPORT_HOME) && env -i PATH=/usr/bin:/bin HOME=$(EXPORT_HOME) XDG_CONFIG_HOME=$(EXPORT_HOME) $(EXPORT_SMOKE)/platformer --headless --frames 120

# macOS only: Aego Editor.app with the native menu bar.
editor-app: bootstrap
	go build -o .cache/bin/aego-editor ./cmd/aego-editor
	go run ./tools/appbundle -bin .cache/bin/aego-editor -out .cache -version $(EDITOR_VERSION) $(if $(wildcard $(EDITOR_ICON)),-icon $(EDITOR_ICON))

dmg: editor-app
	tools/release/dmg.sh ".cache/Aego Editor.app" .cache/dist/Aego-$$(sed -n 's/^var Version = "\(.*\)"$$/\1/p' engine/config.go).dmg

nsis-check:
	sh tools/release/nsis-check.sh

appimage: cli
	mkdir -p .cache/stage/bin .cache/dist && cp .cache/bin/aego .cache/stage/bin/aego && printf 'dev\n' > .cache/stage/VERSION && cp tools/release/README.txt .cache/stage/
	sh tools/release/appimage.sh .cache/stage .cache/dist/Aego-dev-x86_64.AppImage

clean:
	rm -rf .cache/build .cache/bin

# Real WebGL2 material images, typed lanes, hot reload and context-loss integration.
# Missing WebGL2 or WEBGL_lose_context is a failed/unavailable check, never a skip.
WEB_MATERIAL_BROWSERS ?= chromium,firefox
.PHONY: web-materials
web-materials:
	go run ./tools/genwebmaterials/main.go
	mkdir -p .cache/web-materials
	GOOS=js GOARCH=wasm go build -trimpath -o .cache/web-materials/materials.wasm ./tests/webmaterials
	cp tests/webmaterials/index.html .cache/web-materials/index.html
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" .cache/web-materials/wasm_exec.js
	cd tools/websmoke && npm ci --no-audit --no-fund
	cd tools/websmoke && CHROMIUM=$(CHROMIUM) sh private-xvfb.sh node materials.mjs --site=$(CURDIR)/.cache/web-materials --browsers=$(WEB_MATERIAL_BROWSERS) --report=$(CURDIR)/.cache/web-materials/results.json

ci: web-materials

# GPU resource contract checks run independently of the material renderer.
WEB_GPU_BROWSERS ?= chromium,firefox
.PHONY: gpu-smoke web-gpu-smoke image-gpu-smoke
gpu-smoke: bootstrap
	$(GL_ENV) go test -tags aego_gltest ./gpu/gl33 -count=1

image-gpu-smoke: bootstrap
	$(GL_ENV) go test -tags aego_gltest ./modules/render2d/editor -run '^TestImagePreviewLitPixelsAndLifetime$$' -count=1 -v

web-gpu-smoke:
	mkdir -p .cache/web-gpu
	GOOS=js GOARCH=wasm go build -trimpath -o .cache/web-gpu/gpu.wasm ./tests/webgpu
	cp tests/webgpu/index.html .cache/web-gpu/index.html
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" .cache/web-gpu/wasm_exec.js
	cd tools/websmoke && npm ci --no-audit --no-fund
	cd tools/websmoke && CHROMIUM=$(CHROMIUM) sh private-xvfb.sh node gpu.mjs --site=$(CURDIR)/.cache/web-gpu --browsers=$(WEB_GPU_BROWSERS) --report=$(CURDIR)/.cache/web-gpu/results.json

ci: gpu-smoke web-gpu-smoke image-gpu-smoke

# The same independent CPU reference images run on desktop GL and real WebGL2.
# Browser reports distinguish forced fallback from a missing physical capability.
WEB_LIGHTING_BROWSERS ?= chromium,firefox
.PHONY: lighting-smoke web-lighting
lighting-smoke: bootstrap
	go run ./tools/genweblighting/main.go
	$(GL_ENV) AEGO_LIGHTING_REPORT_DIR=$(CURDIR)/.cache/lighting/native go test -tags aego_gltest ./tests/gl -run '^TestLightingGPUContractImages$$' -count=1 -v

web-lighting:
	go run ./tools/genweblighting/main.go
	mkdir -p .cache/web-lighting
	GOOS=js GOARCH=wasm go build -trimpath -o .cache/web-lighting/lighting.wasm ./tests/weblighting
	cp tests/weblighting/index.html .cache/web-lighting/index.html
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" .cache/web-lighting/wasm_exec.js
	cd tools/websmoke && npm ci --no-audit --no-fund
	cd tools/websmoke && CHROMIUM=$(CHROMIUM) sh private-xvfb.sh node lighting.mjs --site=$(CURDIR)/.cache/web-lighting --browsers=$(WEB_LIGHTING_BROWSERS) --report=$(CURDIR)/.cache/web-lighting/results.json

ci: lighting-smoke web-lighting

.PHONY: try-artifacts-check
try-artifacts-check:
	cd tools/gendocs && go run . -prepare-try ../../modules/render2d/res/docs/try/lighting/NormalMapping.assets -check-try
	cd tools/gendocs && go run . -prepare-try ../../modules/render2d/res/docs/try/postfx/CameraOutput.assets -check-try

ci site-check: try-artifacts-check

WEB_PARTICLE_BROWSERS ?= chromium,firefox
.PHONY: web-particle-goldens
web-particle-goldens: cli
	go run ./tools/genparticlefixtures -out .cache/particle-goldens/project
	.cache/bin/aego export .cache/particle-goldens/project --target web --release --out .cache/particle-goldens/web
	cd tools/websmoke && npm ci --no-audit --no-fund
	cd tools/websmoke && CHROMIUM=$(CHROMIUM) sh private-xvfb.sh node particle-goldens.mjs --root=$(CURDIR)/.cache/particle-goldens/web --browsers=$(WEB_PARTICLE_BROWSERS)

ci: web-particle-goldens


# GPU timings must be collected on a quiet machine outside the concurrent CI workload.
.PHONY: particle-smoke-profile
particle-smoke-profile: bootstrap
	$(GL_ENV) go test -tags aego_gltest ./tests/integration -run '^TestParticleSmokeOverdrawGPUProfile$$' -count=1 -v

# Functional acceptance uses release folders served without the source checkout.
# Visibility starts its own X server and requires full desktop browser executables.
STAGE12_RELEASE_DIR = $(CURDIR)/.cache/stage12/releases
STAGE12_WEB_BROWSERS ?= chromium,firefox
.PHONY: stage12-release-check
stage12-release-check: cli
	.cache/bin/aego export examples/platformer --target web --release --out "$(STAGE12_RELEASE_DIR)/platformer"
	.cache/bin/aego export examples/topdown --target web --release --out "$(STAGE12_RELEASE_DIR)/topdown"
	cd tools/websmoke && npm ci --no-audit --no-fund
	@cd tools/websmoke; failed=0; checks="$(STAGE12_RELEASE_DIR)/checks"; mkdir -p "$$checks"; \
	for scenario in lighting finish restore topdown bloom; do \
		project=platformer; if [ "$$scenario" = topdown ]; then project=topdown; fi; \
		xvfb-run -a -s "-screen 0 1600x1000x24 -nolisten tcp" env CHROMIUM="$(CHROMIUM)" node platformer-lighting.mjs \
			--scenario=$$scenario --root="$(STAGE12_RELEASE_DIR)/$$project" --out="$$checks/$$scenario" --browsers="$(STAGE12_WEB_BROWSERS)" >"$$checks/$$scenario.log" 2>&1 || failed=1; \
		cat "$$checks/$$scenario.log"; \
	done; \
	CHROMIUM="$(CHROMIUM)" sh release-visibility.sh --root="$(STAGE12_RELEASE_DIR)/platformer" --out="$$checks/visibility" --browsers="$(STAGE12_WEB_BROWSERS)" >"$$checks/visibility.log" 2>&1 || failed=1; \
	cat "$$checks/visibility.log"; exit $$failed

ci: stage12-release-check

# Full public registry gate; preserve the separate Stage 12 and platformer
# benchmark workloads and all pre-existing CI prerequisites above.
ci: component-try
