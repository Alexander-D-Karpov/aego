GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
NATIVE := .cache/native/$(GOOS)_$(GOARCH)
SDL_VERSION := $(shell awk -F= '$$1=="version"{print $$2}' third_party/sdl/version.txt)
STAMP := $(NATIVE)/.sdl-$(SDL_VERSION)
SMOKE_FRAMES ?= 600
DEMO ?= basic
UI_FONT ?= $(CURDIR)/third_party/fonts/Inter-Regular.ttf
FUZZTIME ?= 15s

.PHONY: bootstrap build test test-nonative test-gl vet lint depcheck smoke run genfont genicons golden golden-update bench bench-scripts cli example example-platformer check-all ci clean fuzz

XVFB := $(shell command -v xvfb-run 2>/dev/null)
ifneq ($(XVFB),)
GL_ENV = $(XVFB) -a -s "-screen 0 1280x720x24" env SDL_VIDEODRIVER=x11 LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe
else
GL_ENV = env SDL_VIDEODRIVER=offscreen LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe
endif

NONATIVE_PKGS = $$(go list ./... | grep -v -e '/platform/sdl$$' -e '/gpu/gl33$$' -e '/gpu/gl33/gl$$')
LINT_PKGS = $$(go list ./... | grep -v '/gpu/gl33/gl$$')

bootstrap: $(STAMP)

$(STAMP): third_party/sdl/version.txt tools/bootstrap-native.sh
	bash tools/bootstrap-native.sh

build: bootstrap
	go build ./...

test: bootstrap
	go test ./...

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
	cd tools/genicons && go mod tidy && go run . -init

golden: bootstrap
	$(GL_ENV) go test -tags aego_golden ./tests/golden/...

golden-update: bootstrap
	$(GL_ENV) go test -tags aego_golden ./tests/golden/... -update

bench: bootstrap
	go run ./tools/bench

bench-scripts:
	go test -run '^$$' -bench . -benchmem ./script/runtime

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

ci: build vet lint depcheck test test-nonative smoke fuzz golden test-gl cli example example-platformer

clean:
	rm -rf .cache/build