GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
NATIVE := .cache/native/$(GOOS)_$(GOARCH)
SDL_VERSION := $(shell awk -F= '$$1=="version"{print $$2}' third_party/sdl/version.txt)
STAMP := $(NATIVE)/.sdl-$(SDL_VERSION)
SMOKE_FRAMES ?= 600
DEMO ?= basic
UI_FONT ?= $(CURDIR)/third_party/fonts/Inter-Regular.ttf

.PHONY: bootstrap build test test-nonative test-gl vet lint depcheck smoke run genfont golden golden-update bench cli example check-all ci clean

GOLDEN_ENV = SDL_VIDEODRIVER=offscreen LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe

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
	SDL_VIDEODRIVER=offscreen LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe \
		go test -tags aego_gltest ./tests/gl/

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

golden: bootstrap
	$(GOLDEN_ENV) go test -tags aego_golden ./tests/golden/...

golden-update: bootstrap
	$(GOLDEN_ENV) go test -tags aego_golden ./tests/golden/... -update

bench: bootstrap
	go run ./tools/bench

cli: bootstrap
	go build -o .cache/bin/aego ./cmd/aego

example: cli
	.cache/bin/aego run examples/native-rotator --headless --frames 120

check-all: cli
	.cache/bin/aego check examples/native-rotator

ci: build vet lint depcheck test test-nonative smoke golden test-gl cli example

clean:
	rm -rf .cache/build