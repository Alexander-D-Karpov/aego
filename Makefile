GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
NATIVE := .cache/native/$(GOOS)_$(GOARCH)
SDL_VERSION := $(shell awk -F= '$$1=="version"{print $$2}' third_party/sdl/version.txt)
STAMP := $(NATIVE)/.sdl-$(SDL_VERSION)
SMOKE_FRAMES ?= 600
DEMO ?= basic
UI_FONT ?= $(CURDIR)/third_party/fonts/Inter-Regular.ttf
DOCS_REPO ?= https://git.akarpov.ru/sanspie/aego
FUZZTIME ?= 15s
STATICCHECK_VERSION ?= v0.8.2
# Benchmarks run pinned to one CPU.
BENCH_COUNT ?= 10
BENCH_CPU ?= 2
BENCH_PIN = $(shell command -v taskset >/dev/null && echo taskset -c $(BENCH_CPU))
EDITOR_VERSION ?= 0.1.0
EDITOR_ICON ?= third_party/brand/aego-1024.png
EXPORT_SMOKE = $(CURDIR)/.cache/export-smoke
EXPORT_HOME = $(CURDIR)/.cache/export-home
CHROMIUM ?= $(shell command -v chromium || command -v chromium-browser || command -v google-chrome)
WEB_SMOKE = cd tools/websmoke && npm ci --no-audit --no-fund && CHROMIUM=$(CHROMIUM) node smoke.mjs --site=$(CURDIR)/site
WASM_BUDGET = 8388608
WEB_DEMO_PROJECT ?= examples/platformer
WEB_DEMO = cd tools/websmoke && CHROMIUM=$(CHROMIUM) node demo.mjs --site=$(CURDIR)/site
WEB_DEMO_BUDGET = @size=$$(stat -c %s site/demo/game.wasm.gz); echo "demo game.wasm.gz: $$size bytes"; \
	if [ $$size -gt $(WASM_BUDGET) ]; then echo "the web demo is over the 8 MB gzip budget"; exit 1; fi
# Module targets, appended by modules/*/module.mk.
MODULE_CI :=
MODULE_BENCH_ALLOCS :=
MODULE_BENCH_BASELINE :=
# Extra bench-compare scenarios, "package:pattern".
MODULE_BENCH_COMPARE :=
BENCH_PHYSICS_PKGS :=
.DEFAULT_GOAL := bootstrap
-include modules/*/module.mk

.PHONY: bootstrap build test test-nonative test-editor test-runtime test-gl vet lint depcheck smoke run genfont genicons icons icons-doc res genpreviews golden golden-update site-headless bench bench-baseline bench-scripts bench-build bench-compare bench-profile bench-scripts-run bench-allocs playground playground-size site site-check cli example example-platformer check-all ci clean fuzz runtime export-smoke editor-app shaders test-wasm web-smoke web-smoke-update

XVFB := $(shell command -v xvfb-run 2>/dev/null)
GL_OVERRIDE = MESA_GL_VERSION_OVERRIDE=3.3 MESA_GLSL_VERSION_OVERRIDE=330
ifneq ($(XVFB),)
GL_ENV = $(XVFB) -a -s "-screen 0 1280x720x24" env SDL_VIDEODRIVER=x11 LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe $(GL_OVERRIDE)
else
GL_ENV = env SDL_VIDEODRIVER=offscreen LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe $(GL_OVERRIDE)
endif

NONATIVE_PKGS = $$(go list ./... | grep -v -e '/platform/sdl$$' -e '/gpu/gl33$$' -e '/gpu/gl33/gl$$')
LINT_PKGS = $$(go list ./... | grep -v '/gpu/gl33/gl$$')

bootstrap: $(STAMP)

$(STAMP): third_party/sdl/version.txt tools/bootstrap-native.sh
	bash tools/bootstrap-native.sh

build: bootstrap
	go build ./...

# Every package minus the dev-only ones, as defined by depcheck runtimeForbidden.
RUNTIME_PKGS = $$(go run -tags aego_nonative ./tools/depcheck -runtime-pkgs .)
WASM_EXEC = $(shell go env GOROOT)/lib/wasm/go_js_wasm_exec
WASM_TEST_PKGS = ./tests/wasm ./platform/input ./gpu/internal/glstate

test: bootstrap
	go test -race ./...

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
	GOOS=js GOARCH=wasm go vet ./platform/web ./gpu/webgl2
	GOOS=js GOARCH=wasm go test -exec=$(WASM_EXEC) $(WASM_TEST_PKGS)

test-nonative:
	go test -tags aego_nonative $(NONATIVE_PKGS)

# Tests needing a real GL context (offscreen SDL + llvmpipe).
test-gl: bootstrap
	$(GL_ENV) go test -tags aego_gltest ./tests/gl/

vet: bootstrap
	go vet -cgocall=false -unsafeptr=false $(LINT_PKGS)
	go vet ./platform/sdl

lint: bootstrap
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) $(LINT_PKGS)

# All graph variants, the semantic pass and the negative rule tests.
depcheck:
	go run -tags aego_nonative ./tools/depcheck -v .
	go test -tags aego_nonative -count=1 ./tools/depcheck

smoke: bootstrap
	go run ./cmd/sandbox --demo headless --frames $(SMOKE_FRAMES) --strict-allocs

run: bootstrap
	go run ./cmd/sandbox --demo $(DEMO) $(ARGS)

# Regenerate the embedded bitmap font atlas into engine/res/fonts.
genfont:
	cd tools/genfont && go mod tidy && go run . -ttf $(UI_FONT)

# Regenerate the committed resources: the icon atlas and the template and example previews (D280).
res: genicons genpreviews shaders

# GLSL ES 3.00 copies of engine/res/shaders for WebGL2 (D290).
shaders:
	go run ./tools/genshaders

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
BENCH_BASELINE = $(BENCH_DIR)/baseline.json
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
	mkdir -p $(BENCH_DIR)
	go build -o .cache/bin/bench ./tools/bench
	$(BENCH_PIN) .cache/bin/bench -baseline $(BENCH_BASELINE) -count $(BENCH_COUNT) -update
	$(BENCH_PIN) .cache/bin/bench -demo stress-scene -baseline $(BENCH_BASELINE) -count $(BENCH_COUNT) -update
	$(MAKE) bench-scripts-run BENCH_OUT=$(BENCH_DIR)/scripts.base.txt

bench-scripts-run:
	mkdir -p $(BENCH_DIR)
	$(BENCH_PIN) go test -run '^$$' -bench . -benchmem -count=$(BENCH_COUNT) $(BENCH_PKGS) > $(BENCH_OUT)

bench-scripts:
	$(MAKE) bench-scripts-run BENCH_OUT=$(BENCH_DIR)/scripts.txt
	$(BENCH_GATE) -base $(BENCH_DIR)/scripts.base.txt -head $(BENCH_DIR)/scripts.txt

# CI gate: allocations only, equal to the saved baseline.
bench-allocs: bootstrap $(MODULE_BENCH_ALLOCS)
	go build -o .cache/bin/bench ./tools/bench
	$(GL_ENV) .cache/bin/bench -baseline $(BENCH_BASELINE) -count 1 -time=false
	$(GL_ENV) .cache/bin/bench -demo stress-scene -baseline $(BENCH_BASELINE) -count 1 -time=false
	$(MAKE) bench-scripts-run BENCH_OUT=$(BENCH_DIR)/scripts.allocs.txt BENCH_COUNT=1
	$(BENCH_GATE) -time=false -base $(BENCH_DIR)/scripts.base.txt -head $(BENCH_DIR)/scripts.allocs.txt


bench-build:
	rm -rf $(BENCH_DIR)/base $(BENCH_DIR)/base-bin $(BENCH_DIR)/head-bin $(BENCH_DIR)/base.txt $(BENCH_DIR)/head.txt
	mkdir -p $(BENCH_DIR)/base-bin $(BENCH_DIR)/head-bin
	git worktree prune
	git worktree add -f --detach $(BENCH_DIR)/base $(BENCH_BASE)
	cd $(BENCH_DIR)/base && for p in $(BENCH_PKGS); do [ -d $$p ] && go test -c -o $(BENCH_DIR)/base-bin/$$(basename $$p).test $$p; done; true
	cd $(BENCH_DIR)/base && for e in $(MODULE_BENCH_COMPARE); do p=$${e%%:*}; [ -d $$p ] && go test -c -o $(BENCH_DIR)/base-bin/$$(basename $$p).test $$p && echo "$${e#*:} $$p" > $(BENCH_DIR)/base-bin/$$(basename $$p).bench; done; true
	git worktree remove --force $(BENCH_DIR)/base
	for p in $(BENCH_PKGS); do go test -c -o $(BENCH_DIR)/head-bin/$$(basename $$p).test $$p; done
	for e in $(MODULE_BENCH_COMPARE); do p=$${e%%:*}; go test -c -o $(BENCH_DIR)/head-bin/$$(basename $$p).test $$p && echo "$${e#*:} $$p" > $(BENCH_DIR)/head-bin/$$(basename $$p).bench; done

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
	.cache/bin/aego docs build --out site --repo $(DOCS_REPO) --demo
	.cache/bin/aego export $(WEB_DEMO_PROJECT) --target web --release --out site/demo
	gzip -9 -k -f site/play/aego.wasm
	@size=$$(stat -c %s site/play/aego.wasm.gz); echo "aego.wasm.gz: $$size bytes"; \
	if [ $$size -gt $(WASM_BUDGET) ]; then echo "playground is over the 8 MB gzip budget, see make playground-size"; exit 1; fi
	$(WEB_DEMO_BUDGET)

playground:
	@if GOOS=js GOARCH=wasm go list -deps ./cmd/aego-playground | grep -E '^aego/(platform/sdl|gpu/gl33|asset/pipeline|asset/importers|modules/render3d)'; then echo "playground graph pulls a native or pipeline package"; exit 1; fi
	$(MAKE) site

site-headless:
	CGO_ENABLED=0 go build -tags aego_nonative -o .cache/bin/aego-docs ./cmd/aego
	.cache/bin/aego-docs docs build --out site --engine $(CURDIR) --repo $(DOCS_REPO) --demo
	.cache/bin/aego-docs export $(WEB_DEMO_PROJECT) --target web --release --engine $(CURDIR) --out site/demo
	gzip -9 -k -f site/play/aego.wasm
	@size=$$(stat -c %s site/play/aego.wasm.gz); echo "aego.wasm.gz: $$size bytes"; \
	if [ $$size -gt $(WASM_BUDGET) ]; then echo "playground is over the 8 MB gzip budget, see make playground-size"; exit 1; fi
	$(WEB_DEMO_BUDGET)

site-check: cli
	.cache/bin/aego docs check
	.cache/bin/aego docs export --out $(CURDIR)/.cache/docs.json
	cd tools/gendocs && AEGO_DOCS_EXPORT=$(CURDIR)/.cache/docs.json go test ./...

playground-size:
	mkdir -p .cache
	GOOS=js GOARCH=wasm go build -trimpath -o .cache/aego-playground.wasm ./cmd/aego-playground
	go run ./tools/wasmsize -top 40 .cache/aego-playground.wasm
	go run ./tools/wasmsize -top 15 -depth 2 .cache/aego-playground.wasm

web-smoke: site
	$(WEB_SMOKE) --report=$(CURDIR)/.cache/web-bench.json
	$(WEB_DEMO)

web-smoke-update: site
	$(WEB_SMOKE) --update
	$(WEB_DEMO) --update

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

ci: build vet lint depcheck test test-nonative test-editor test-runtime test-wasm runtime smoke fuzz golden test-gl cli playground web-smoke site-check example example-platformer check-all export-smoke $(MODULE_CI) bench-allocs

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

clean:
	rm -rf .cache/build
