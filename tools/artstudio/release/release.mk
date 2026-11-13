# Use directly, or include in the repository's existing Makefile after review.
# This does not replace Aego's Makefile or install another CI provider.
ARTSTUDIO_OUT ?= .cache/artstudio-release
ARTSTUDIO_ID ?= local
ARTSTUDIO_APP_ORIGIN ?= http://127.0.0.1:8097
ARTSTUDIO_DOCS_ORIGIN ?= http://127.0.0.1:8098
.PHONY: studio-web-build studio-web-test studio-web-verify studio-web-run studio-web-ci
studio-web-build:
	go run ./tools/artstudio/release build -out '$(ARTSTUDIO_OUT)' -id '$(ARTSTUDIO_ID)' -app-origin '$(ARTSTUDIO_APP_ORIGIN)' -docs-origin '$(ARTSTUDIO_DOCS_ORIGIN)'
studio-web-test:
	go test -race -count=1 ./art ./tools/artstudio/host ./tools/artstudio/gpupreview ./tools/artstudio/release
	go vet ./art ./tools/artstudio/host ./tools/artstudio/gpupreview ./tools/artstudio/release
	node --test tools/artstudio/release/tests/*.test.cjs
	PYTHONDONTWRITEBYTECODE=1 "$${ARTSTUDIO_PYTHON:-python3}" -B -m unittest discover -s tools/artstudio/release/tests -p test_browser_context.py
	PYTHONDONTWRITEBYTECODE=1 "$${ARTSTUDIO_PYTHON:-python3}" -B -m unittest discover -s tools/artstudio/release/tests -p test_release_ci.py
studio-web-verify:
	go run ./tools/artstudio/release verify -dir '$(ARTSTUDIO_OUT)'
studio-web-run:
	go run ./tools/artstudio/release serve -dir '$(ARTSTUDIO_OUT)'
studio-web-ci:
	bash tools/artstudio/release/ci.sh
