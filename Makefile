GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
NATIVE := .cache/native/$(GOOS)_$(GOARCH)
SDL_VERSION := $(shell awk -F= '$$1=="version"{print $$2}' third_party/sdl/version.txt)
STAMP := $(NATIVE)/.sdl-$(SDL_VERSION)
SMOKE_FRAMES ?= 600
DEMO ?= basic

.PHONY: bootstrap build test test-nonative test-gl vet lint depcheck smoke run ci clean

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

ci: build vet lint depcheck test test-nonative test-gl smoke

clean:
	rm -rf .cache/build