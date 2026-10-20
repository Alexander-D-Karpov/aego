---
id: install
icon: file.reveal
group: start
title: Install and run
order: 0
---
# Install and run

Aego is one program, `aego`: the editor, the command line tools and the game runner. Install a prebuilt build on Linux with one command or on Windows with an installer, or build it from source on Linux, macOS and Windows.

![The Aego editor with the platformer example open](shots/editor.png)

## Linux

On x86-64:

```sh
curl -fsSL https://aego.akarpov.ru/install.sh | sh
```

The script downloads the [latest release](https://git.akarpov.ru/sanspie/aego/releases/latest), checks its SHA-256 sum, unpacks it to `~/.local/share/aego` and links `~/.local/bin/aego`. Run it again to update. Options go after `sh -s --`:

| Option | Installs |
|---|---|
| (none) | the latest release |
| `--nightly` | the [nightly pre-release](https://git.akarpov.ru/sanspie/aego/releases/tag/nightly), rebuilt from every green commit on master |
| `--version v0.9.0` | one exact release |
| `--prefix DIR` | into `DIR/share/aego` and `DIR/bin` instead of `~/.local` |
| `--no-deps` | without installing system libraries |

```sh
curl -fsSL https://aego.akarpov.ru/install.sh | sh -s -- --nightly
```

To install by hand, download `aego-<version>-linux-amd64.tar.gz` and its `.sha256` from the [releases page](https://git.akarpov.ru/sanspie/aego/releases), check the sum with `sha256sum -c`, unpack the archive anywhere and put its `bin` directory on `PATH`.

The prebuilt editor needs glibc 2.39 or newer (Ubuntu 24.04, Debian 13, Fedora 40 and later) and OpenGL 3.3 on X11 or Wayland. The script installs the system libraries it needs when they are missing (Mesa OpenGL, X11, Wayland and xkbcommon) with apt, dnf, pacman or zypper, using `sudo` when it is not run as root; `--no-deps` skips that. When `glxinfo` is present it also warns if the driver offers less than OpenGL 3.3. It edits, runs and exports projects that use scripts only. Projects with native Go modules, and exports to other operating systems, need Go and a source checkout: see below.

## Windows

Download `aego-<version>-windows-amd64-setup.exe` from the [latest release](https://git.akarpov.ru/sanspie/aego/releases/latest) and run it. It installs for your user without administrator rights, into `%LOCALAPPDATA%\Programs\Aego`: the editor with Start menu and desktop shortcuts, and `aego` on `PATH` for terminals. Run a newer installer to update; uninstall from Settings → Apps. The editor needs a GPU driver with OpenGL 3.3, which every driver from the last ten years has.

The same from PowerShell, also for updating:

```powershell
irm https://aego.akarpov.ru/install.ps1 | iex
```

Set `$env:AEGO_CHANNEL = "nightly"` first for the [nightly build](https://git.akarpov.ru/sanspie/aego/releases/tag/nightly). The release also has a `.zip` with the same two programs for unpacking by hand.

Prebuilt macOS builds are not published yet; build from source there.

## Build from source

You need [Go 1.27](https://go.dev/dl/), Git, a C compiler, CMake and Ninja. The first build downloads and compiles SDL 3 into `.cache/native`; later builds reuse it.

### Linux

On Debian and Ubuntu:

```sh
sudo apt-get install -y build-essential cmake ninja-build pkg-config \
  libgl1-mesa-dev libegl1-mesa-dev libgles2-mesa-dev \
  libx11-dev libxext-dev libxrandr-dev libxcursor-dev libxfixes-dev libxi-dev libxss-dev libxtst-dev libxkbcommon-dev \
  libwayland-dev wayland-protocols libdecor-0-dev \
  libasound2-dev libpulse-dev libdrm-dev libgbm-dev libudev-dev libdbus-1-dev libibus-1.0-dev
```

Other distributions need the same libraries under their own package names.

### macOS

Install the Xcode command line tools with `xcode-select --install`, then `brew install go cmake ninja`.

### Windows

Only needed to build Aego itself; the installer above is enough to make games. Go needs a C compiler for the engine's native parts, and on Windows that comes from [MSYS2](https://www.msys2.org/). In its UCRT64 shell, install the compiler, CMake and Ninja with MSYS2's package manager: `pacman -S mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-cmake mingw-w64-ucrt-x86_64-ninja`. Put `C:\msys64\ucrt64\bin` on `PATH` and build from Git Bash.

### Build

```sh
git clone https://git.akarpov.ru/sanspie/aego.git
cd aego
bash tools/bootstrap-native.sh
go build -o aego ./cmd/aego
```

`make cli` does the same and leaves the binary in `.cache/bin/aego`. A binary built this way remembers the checkout it was built from. A prebuilt one does not: set `AEGO_ROOT` to a checkout, or pass `--engine DIR`, when a native project or an export needs the engine sources.

## First project

```sh
aego new mygame
aego edit mygame
```

`aego new` creates a 2D project with a scene holding a camera and a sprite, and a script to start from; `--template 3d` and the other templates are listed by `aego help`. `aego edit` opens the editor; its Getting Started panel walks through the first six steps: a node, a script, Play, a live tweak, hot reload and fixing an error. Without a project, `aego edit` opens the launcher with recent projects and templates.

Other commands you will use early:

| Command | Does |
|---|---|
| `aego run mygame` | runs the game without the editor |
| `aego check mygame` | type-checks scripts and audits every file |
| `aego test mygame` | runs the project's script tests |
| `aego export mygame --target web` | builds a web version, like the demo on the home page |

The [CLI reference](ref:cli) lists every command and flag.
