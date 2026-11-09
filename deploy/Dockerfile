FROM golang:1.27-alpine AS toolchain
FROM ubuntu:24.04 AS build
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates make gzip brotli glslang-tools=15.1.0-2~ubuntu0.24.04.2
COPY --from=toolchain /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"
WORKDIR /src
COPY go.mod go.sum ./
COPY tools/gendocs/go.mod tools/gendocs/go.sum tools/gendocs/
RUN go mod download && cd tools/gendocs && go mod download
COPY . .
ARG AEGO_COMMIT=""
ARG ARTSTUDIO_MANIFEST_SHA256=""
ENV AEGO_COMMIT=$AEGO_COMMIT
# The build context must contain the *already built* immutable release at
# studio-artifact/. Fail closed rather than silently packaging local /studio.
# The caller must separately verify the full context against the commit and run
# all release gates against the same staged artifact and finished site image.
RUN go run ./tools/artstudio/release verify-site-input \
      -dir /src/studio-artifact -root /src -sha256 "$ARTSTUDIO_MANIFEST_SHA256" \
      -commit "$AEGO_COMMIT" -app-origin https://aeditor.akarpov.ru -docs-origin https://aego.akarpov.ru \
 && bash tools/check-glslang.sh \
 && ARTSTUDIO_ARTIFACT=/src/studio-artifact ARTSTUDIO_MANIFEST_SHA256="$ARTSTUDIO_MANIFEST_SHA256" make site-headless \
      DOCS_BUILD_ARGS='--examples-production --examples-license MIT --examples-license-file /src/docs/try-examples-license.txt' \
 && test -f site/asset-studio/integration.json && test ! -e site/studio \
 && go run ./tools/artstudio/release verify-site-input \
      -dir /src/studio-artifact -root /src -sha256 "$ARTSTUDIO_MANIFEST_SHA256" \
      -commit "$AEGO_COMMIT" -app-origin https://aeditor.akarpov.ru -docs-origin https://aego.akarpov.ru
RUN find site -type f \( -name '*.js' -o -name '*.css' -o -name '*.html' -o -name '*.json' -o -name '*.svg' \) -exec gzip -9 -k -f {} \;
RUN find site -type f -name '*.wasm' | while read -r f; do [ -f "$f.gz" ] || gzip -9 -k -f "$f"; [ -f "$f.br" ] || brotli -q 11 -k -f "$f"; done
COPY tools/release/install.sh tools/release/install.ps1 site/
RUN chmod 0644 site/install.sh site/install.ps1
# the edge proxy serves the site from its own copy: everything but the wasm binaries, which it fetches separately
RUN mkdir -p site/__edge && tar -czf site/__edge/site.tar.gz --exclude='./__edge' --exclude='*.wasm' --exclude='*.wasm.gz' --exclude='*.wasm.br' -C site . \
 && sha256sum site/__edge/site.tar.gz | cut -c1-16 > site/__edge/version

FROM nginx:1.27-alpine
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/site /usr/share/nginx/html
HEALTHCHECK --interval=10s --timeout=3s CMD wget -qO- http://127.0.0.1/healthz >/dev/null || exit 1
