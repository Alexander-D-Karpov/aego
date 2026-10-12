GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
NATIVE := .cache/native/$(GOOS)_$(GOARCH)
SDL_VERSION := $(shell awk -F= '$$1=="version"{print $$2}' third_party/sdl/version.txt)
STAMP := $(NATIVE)/.sdl-$(SDL_VERSION)
SMOKE_FRAMES ?= 600
DEMO ?= basic
UI_FONT ?= $(CURDIR)/third_party/fonts/Inter-Regular.ttf
FUZZTIME ?= 15s

.PHONY: bootstrap build test test-nonative test-editor test-runtime test-gl vet lint depcheck smoke run genfont genicons golden golden-update bench bench-scripts bench-build bench-compare bench-profile playground playground-size site site-check cli example example-platformer check-all ci clean fuzz

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

RUNTIME_PKGS = ./engine/... ./scene/... ./render/... ./render2d/... ./render3d/... ./script/vm/... ./script/bytecode/... ./script/verify/... ./script/bind ./script/coroutine/... ./script/signal/... ./script/module/... ./sdk/...

test: bootstrap
	go test -race ./...

test-editor: bootstrap
	go build -tags aego_editor ./...
	go test -tags aego_editor ./editor/... ./app/... ./sdk/...

test-runtime: bootstrap
	go build -tags aego_runtime ./...
	go test -tags aego_runtime $(RUNTIME_PKGS)

test-nonative:
	go test -tags aego_nonative $(NONATIVE_PKGS)

# Tests needing a real GL context. Offscreen SDL + llvmpipe: no display server,
# no GPU variance, so goldens compare byte-for-byte across machines.
test-gl: bootstrap
	$(GL_ENV) go test -tags aego_gltest ./tests/gl/

vet: bootstrap
	go vet -cgocall=false -unsafeptr=false $(LINT_PKGS)
	go vet ./platform/sdl

lint: bootstrap
	go run honnef.co/go/tools/cmd/staticcheck@latest $(LINT_PKGS)

depcheck:
	go run ./tools/depcheck .

smoke: bootstrap
	go run ./cmd/sandbox --demo headless --frames $(SMOKE_FRAMES) --strict-allocs

run: bootstrap
	go run ./cmd/sandbox --demo $(DEMO) $(ARGS)

# Regenerate the embedded bitmap font atlas into engine/res/fonts.
genfont:
	cd tools/genfont && go mod tidy && go run . -ttf $(UI_FONT)

# Regenerate the embedded icon atlas into engine/res/icons.
genicons:
	cd tools/genicons && go mod tidy && go run . -init -src "$$(cd ../.. && go run ./tools/iconsrc)"

golden: bootstrap
	$(GL_ENV) go test -tags aego_golden ./tests/golden/...

golden-update: bootstrap
	$(GL_ENV) go test -tags aego_golden ./tests/golden/... -update

bench: bootstrap
	go run ./tools/bench

bench-scripts:
	go test -run '^$$' -bench . -benchmem ./script/vm ./script/coroutine ./script/signal ./script/runtime ./script/bind

BENCH_PKGS = ./script/vm ./script/coroutine ./script/signal ./script/runtime ./script/bind
BENCH_COUNT ?= 10
BENCH_BASE ?= stage5
BENCH_DIR = $(CURDIR)/.cache/bench

BENCH_CPU ?= 2
BENCH_PIN = $(shell command -v taskset >/dev/null && echo taskset -c $(BENCH_CPU))

bench-build:
	rm -rf $(BENCH_DIR) && mkdir -p $(BENCH_DIR)/base-bin $(BENCH_DIR)/head-bin
	git worktree prune
	git worktree add -f --detach $(BENCH_DIR)/base $(BENCH_BASE)
	cd $(BENCH_DIR)/base && for p in $(BENCH_PKGS); do [ -d $$p ] && go test -c -o $(BENCH_DIR)/base-bin/$$(basename $$p).test $$p; done; true
	git worktree remove --force $(BENCH_DIR)/base
	for p in $(BENCH_PKGS); do go test -c -o $(BENCH_DIR)/head-bin/$$(basename $$p).test $$p; done

bench-compare: bench-build
	for i in $$(seq $(BENCH_COUNT)); do \
		for v in base head; do \
			for t in $(BENCH_DIR)/$$v-bin/*.test; do \
				$(BENCH_PIN) $$t -test.run '^$$' -test.bench . -test.benchmem >> $(BENCH_DIR)/$$v.txt; \
			done; \
		done; \
	done
	go run golang.org/x/perf/cmd/benchstat@latest $(BENCH_DIR)/base.txt $(BENCH_DIR)/head.txt

bench-profile:
	[ -x $(BENCH_DIR)/base-bin/runtime.test ] || $(MAKE) bench-build
	go test -c -o $(BENCH_DIR)/head-bin/runtime.test ./script/runtime
	$(BENCH_PIN) $(BENCH_DIR)/base-bin/runtime.test -test.run '^$$' -test.bench 'Update2k$$' -test.benchtime 20000x -test.cpuprofile $(BENCH_DIR)/base.prof
	$(BENCH_PIN) $(BENCH_DIR)/head-bin/runtime.test -test.run '^$$' -test.bench 'Update2k$$' -test.benchtime 20000x -test.cpuprofile $(BENCH_DIR)/head.prof
	go tool pprof -top -nodecount 40 -diff_base $(BENCH_DIR)/base.prof $(BENCH_DIR)/head.prof

site: cli
	.cache/bin/aego docs build --out site
	gzip -9 -k -f site/play/aego.wasm
	@size=$$(stat -c %s site/play/aego.wasm.gz); echo "aego.wasm.gz: $$size bytes"; \
	if [ $$size -gt 4194304 ]; then echo "playground is over the 4 MB gzip budget, see make playground-size"; exit 1; fi

playground:
	@if GOOS=js GOARCH=wasm go list -deps ./cmd/aego-playground | grep -E '^aego/(platform/sdl|gpu/gl33|asset/pipeline|asset/importers|render3d)'; then echo "playground graph pulls a native or pipeline package"; exit 1; fi
	$(MAKE) site

site-check: cli
	.cache/bin/aego docs check
	.cache/bin/aego docs export --out $(CURDIR)/.cache/docs.json
	cd tools/gendocs && AEGO_DOCS_EXPORT=$(CURDIR)/.cache/docs.json go test ./...

playground-size:
	GOOS=js GOARCH=wasm go list -export -deps -f '{{if .Export}}{{.ImportPath}} {{.Export}}{{end}}' ./cmd/aego-playground | while read p f; do echo "$$(stat -c %s $$f) $$p"; done | sort -rn | head -40

cli: bootstrap
	go build -o .cache/bin/aego ./cmd/aego

example: cli
	.cache/bin/aego run examples/native-rotator --headless --frames 120

example-platformer: cli
	.cache/bin/aego run examples/platformer --headless --frames 600

check-all: cli
	.cache/bin/aego check examples/native-rotator

# Fuzz the lexer and parser for FUZZTIME each. Plain `go test` already runs the
# seed corpus; this exercises the mutators. Wired into ci.
fuzz:
	go test -run '^$$' -fuzz '^FuzzTokenize$$' -fuzztime $(FUZZTIME) ./script/lexer
	go test -run '^$$' -fuzz '^FuzzParse$$' -fuzztime $(FUZZTIME) ./script/parser
	go test -run '^$$' -fuzz '^FuzzDecodeVerify$$' -fuzztime $(FUZZTIME) ./script/verify
	go test -run '^$$' -fuzz '^FuzzDecodeVerifyV3$$' -fuzztime $(FUZZTIME) ./script/verify

ci: build vet lint depcheck test test-nonative test-editor test-runtime smoke fuzz golden test-gl cli playground site-check example example-platformer check-all bench-scripts

clean:
	rm -rf .cache/build