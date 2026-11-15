# Build only from a reviewed, minimal context; see tools/ci/env/README.md.
FROM golang@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 AS go_toolchain
FROM node@sha256:01d566e8d94d8c619591d78652babfccd3c6992a3dffa9ed5ab39e11cf6c70e9 AS node_toolchain
FROM mcr.microsoft.com/playwright/python@sha256:330d401ae23c905896a0c26c66c36fc971b621519e7b11b435a8328d7bf6e031

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
ENV DEBIAN_FRONTEND=noninteractive \
    GOTOOLCHAIN=local \
    PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    PATH=/opt/aego/bin:/usr/local/go/bin:/opt/aego/node/bin:$PATH \
    SDL_AUDIO_DRIVER=dummy \
    SDL_AUDIODRIVER=dummy

# Browser libraries and revisions come from the pinned Playwright image.
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential cmake ninja-build pkg-config openssl curl ca-certificates \
      python3-venv xvfb xauth xdotool sudo xz-utils patch \
      fonts-dejavu-core fonts-liberation nsis squashfs-tools \
      libgl1-mesa-dev libegl1-mesa-dev libgles2-mesa-dev \
      libx11-dev libxext-dev libxrandr-dev libxcursor-dev libxfixes-dev libxi-dev libxss-dev libxtst-dev libxkbcommon-dev \
      libwayland-dev wayland-protocols libdecor-0-dev \
      libasound2-dev libpulse-dev libdrm-dev libgbm-dev libudev-dev libdbus-1-dev libibus-1.0-dev \
      glslang-tools=15.1.0-2~ubuntu0.24.04.2 && \
    rm -rf /var/lib/apt/lists/*

COPY --from=go_toolchain /usr/local/go /usr/local/go
COPY --from=node_toolchain /usr/local/bin/node /opt/aego/node/bin/node
COPY --from=node_toolchain /usr/local/lib/node_modules/npm /opt/aego/node/lib/node_modules/npm
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /opt/aego/node/bin/npm && \
    ln -s ../lib/node_modules/npm/bin/npx-cli.js /opt/aego/node/bin/npx

COPY tools/ci/env/requirements.txt /opt/aego/ci-env/requirements.txt
RUN python3 -m venv /opt/aego/python && \
    /opt/aego/python/bin/pip install --no-cache-dir --require-hashes -r /opt/aego/ci-env/requirements.txt

RUN mkdir -p /opt/aego/bin && GOBIN=/opt/aego/bin go install honnef.co/go/tools/cmd/staticcheck@v0.8.2

# Keep SDL inputs separate from application source and from the Node lock layer.
COPY third_party/sdl/version.txt /opt/aego/seed/third_party/sdl/version.txt
COPY tools/bootstrap-native.sh /opt/aego/seed/tools/bootstrap-native.sh
RUN cd /opt/aego/seed && GOOS=linux GOARCH=amd64 bash tools/bootstrap-native.sh && \
    mkdir -p /opt/aego/native && cp -a .cache/native/linux_amd64 /opt/aego/native/ && \
    rm -rf /opt/aego/seed/.cache

COPY tools/websmoke/package.json tools/websmoke/package-lock.json /opt/aego/websmoke/
RUN cd /opt/aego/websmoke && npm ci --no-audit --no-fund

COPY tools/ci/env/Dockerfile tools/ci/env/lock.json tools/ci/env/preflight.py tools/ci/env/browser_smoke.py /opt/aego/ci-env/
RUN python3 -B /opt/aego/ci-env/preflight.py --image-only && \
    timeout --kill-after=10s 180s xvfb-run -a -s '-screen 0 1440x1050x24' \
      /opt/aego/python/bin/python -u -B /opt/aego/ci-env/browser_smoke.py

WORKDIR /workspace
CMD ["bash"]
