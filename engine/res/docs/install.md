---
id: install
icon: file.download
group: start
title: Install Aego
order: 0
---
# Install Aego

Aego is one program: the editor, the command line and the runtime. Install it once, then create projects anywhere.

## Download

The buttons above take the files of the [latest release](https://git.akarpov.ru/sanspie/aego/releases/latest); every file has its SHA-256 sum in `SHA256SUMS.txt`. The [nightly pre-release](https://git.akarpov.ru/sanspie/aego/releases/tag/nightly) is rebuilt from every green commit.

- **Windows** — run the installer. It installs for your user only, adds `aego` to PATH, puts Aego in the Start menu and opens `.aego` projects on double click.
- **macOS** — open the disk image and drag Aego to Applications. The app is not notarized yet: on first launch, right-click → Open, or allow it in System Settings → Privacy & Security. A release may come without a disk image, because it is built on a Mac separately.
- **Linux** — download the AppImage, make it executable and run it. Gear Lever or AppImageLauncher add it to your menu; the install script below does that by itself. A tarball is also available.

## Command line

For servers, CI and anyone who prefers a terminal:

```sh
curl -fsSL https://aego.akarpov.ru/install.sh | sh
curl -fsSL https://aego.akarpov.ru/install.sh | sh -s -- --nightly
curl -fsSL https://aego.akarpov.ru/install.sh | sh -s -- --version v0.10.0
```

```powershell
irm https://aego.akarpov.ru/install.ps1 | iex
```

On Windows, set `$env:AEGO_CHANNEL = "nightly"`, or a tag such as `"v0.10.0"`, before the command to pick another build. The script verifies the checksum, installs into `~/.local` (or `%LOCALAPPDATA%\Programs\Aego`) and prints what it did.

## From source

Needs Go 1.27 and a C compiler for SDL3, with CMake and Ninja. The first build downloads and compiles SDL 3 into `.cache/native`.

On Debian and Ubuntu:

```sh
sudo apt-get install -y build-essential cmake ninja-build pkg-config \
  libgl1-mesa-dev libegl1-mesa-dev libgles2-mesa-dev \
  libx11-dev libxext-dev libxrandr-dev libxcursor-dev libxfixes-dev libxi-dev libxss-dev libxtst-dev libxkbcommon-dev \
  libwayland-dev wayland-protocols libdecor-0-dev \
  libasound2-dev libpulse-dev libdrm-dev libgbm-dev libudev-dev libdbus-1-dev libibus-1.0-dev
```

On macOS, `xcode-select --install`, then `brew install go cmake ninja`. On Windows, use the UCRT64 shell of [MSYS2](https://www.msys2.org/): `pacman -S mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-cmake mingw-w64-ucrt-x86_64-ninja`.

```sh
git clone https://git.akarpov.ru/sanspie/aego.git
cd aego
make bootstrap && make cli
```

`make cli` builds the editor and the command line into `.cache/bin/aego`; `make ci` runs what CI runs.

## Requirements

OpenGL 3.3 or WebGL2. Any computer from the last ten years qualifies; virtual machines and remote desktops often don't. If Aego stops with a message about OpenGL, update the graphics driver or run it on real hardware.

## Native Go extensions

Optional. Projects that add their own Go code need Go installed; the editor says so when it finds none. Script-only projects work without Go.

Next: [Your first project →](ref:quickstart)
