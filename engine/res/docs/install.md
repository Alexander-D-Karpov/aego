---
id: install
icon: file.reveal
group: start
title: Install and run
order: 0
---
# Install and run

Aego is one program: the editor with its launcher, the command line tools and the game runner. Download it, open it, make a project; no terminal is needed.

![The Aego editor with the platformer example open](shots/editor.png)

## Download

Every file is on the [latest release](https://git.akarpov.ru/sanspie/aego/releases/latest), with its SHA-256 sum in `SHA256SUMS.txt`. The [nightly pre-release](https://git.akarpov.ru/sanspie/aego/releases/tag/nightly) is rebuilt from every green commit.

| System | File | Then |
|---|---|---|
| Windows 10 and 11, x86-64 | `aego-<version>-windows-amd64-setup.exe` | run it; Aego appears in the Start menu and on the desktop |
| macOS 11 and later | `Aego-<version>.dmg` | open it and drag Aego to Applications |
| Linux, x86-64 | `Aego-<version>-x86_64.AppImage` | make it executable (`chmod +x`, or Properties → Allow executing) and open it |

Opening Aego shows the launcher: recent projects, **New Project** with templates, and **Examples**. New projects go to `Documents/Aego Projects` unless you choose another folder with **Browse…**. Drop a project folder, or its `project.aego`, on the launcher to open it. The launcher checks for a newer release once a day and shows a banner with a download link; it never updates by itself, and the check can be turned off at the bottom of the launcher.

### Windows

The installer works without administrator rights and installs into `%LOCALAPPDATA%\Programs\Aego`. Double-clicking a `project.aego` opens the project, and a `.aescene` or `.aeprefab` opens the editor on that file. The optional "Open with Aego" item in folder menus opens a project folder. Run a newer installer to update; uninstall from Settings → Apps, which also removes the file associations. The editor needs a GPU driver with OpenGL 3.3, which every driver from the last ten years has.

### macOS

The app is signed ad hoc, not notarized, so on first launch macOS says it cannot check it. Open System Settings → Privacy & Security and press **Open Anyway** under the message about Aego, or right-click Aego in Applications and choose Open. A release may come without a `.dmg`, because it is built on a Mac separately, and its sum is not in `SHA256SUMS.txt`; without one, build from source.

### Linux

The AppImage needs glibc 2.39 or newer (Ubuntu 24.04, Debian 13, Fedora 40) and OpenGL 3.3 on X11 or Wayland. Tools such as Gear Lever or AppImageLauncher add it to the application menu; the install script below does that by itself.

## Native code

Projects edited with scripts need nothing else. The **Native Go code** option of New Project, and exports to other operating systems, need [Go 1.27](https://go.dev/dl/) and an engine source checkout set in `AEGO_ROOT`. When a native project is opened without Go, the editor still opens it for scripts and says in the status bar that native code is not built.

## In the editor

File → New Project, Open Project, Open Recent and Close Project switch projects. Play → Run Script Tests runs the `*_test.aes` files with the results in Console. The Export panel builds a game into the folder you choose, then **Open Folder** shows it and **Run** starts it (a web build opens in the browser from a local server). Help → Open Logs Folder, Crash Reports and Report a Bug collect what a bug report needs.

## Command line and CI

The same program works from a terminal, for scripts, agents and CI.

Linux, also for updating:

```sh
curl -fsSL https://aego.akarpov.ru/install.sh | sh
```

It checks the archive's SHA-256 sum, unpacks it to `~/.local/share/aego`, links `~/.local/bin/aego`, adds Aego to the application menu with `project.aego` files opening in it, and installs missing system libraries (Mesa OpenGL, X11, Wayland and xkbcommon) with apt, dnf, pacman or zypper, using `sudo` when it is not run as root. Options go after `sh -s --`:

| Option | Does |
|---|---|
| `--nightly` | installs the nightly pre-release |
| `--version v0.10.0` | installs one exact release |
| `--prefix DIR` | installs into `DIR/share/aego` and `DIR/bin` |
| `--no-deps` | skips system libraries |
| `--no-desktop` | skips the application menu entry and file types |

To install by hand, download `aego-<version>-linux-amd64.tar.gz` and `SHA256SUMS.txt`, check the archive with `sha256sum -c --ignore-missing SHA256SUMS.txt`, unpack it anywhere and put its `bin` directory on `PATH`.

Windows, also for updating:

```powershell
irm https://aego.akarpov.ru/install.ps1 | iex
```

Set `$env:AEGO_CHANNEL = "nightly"` first for the nightly build. The release also has a `.zip` with the two programs for unpacking by hand.

```sh
aego new mygame
aego edit mygame
aego edit mygame/scenes/main.aescene
aego edit --launcher new
```

`aego new` creates a 2D project with a scene holding a camera and a sprite, and a script to start from; `--template 3d` and the other templates are listed by `aego help`. `aego new mygame --example platformer` copies the platformer (a character on Motion 2D with stomps, a pit, a checkpoint and save games) and `--example topdown` the top-down game (a sword, enemies that patrol and chase with a State Machine, coins and lava); the launcher lists the same examples under **Examples**. In the editor, the Getting Started panel walks through the first six steps: a node, a script, Play, a live tweak, hot reload and fixing an error.

| Command | Does |
|---|---|
| `aego run mygame` | runs the game without the editor |
| `aego check mygame` | type-checks scripts and audits every file |
| `aego test mygame` | runs the project's script tests |
| `aego export mygame --target web` | builds a web version, like the demo on the home page |

The [CLI reference](ref:cli) lists every command and flag.

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

Other distributions need the same libraries under their own package names. `make appimage` packs the result into an AppImage.

### macOS

Install the Xcode command line tools with `xcode-select --install`, then `brew install go cmake ninja`. `make editor-app` builds `.cache/Aego Editor.app` with the native menu bar, and `make dmg` packs it into a disk image.

### Windows

Only needed to build Aego itself; the installer above is enough to make games. In the UCRT64 shell of [MSYS2](https://www.msys2.org/): `pacman -S mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-cmake mingw-w64-ucrt-x86_64-ninja`, put `C:\msys64\ucrt64\bin` on `PATH` and build from Git Bash.

### Build

```sh
git clone https://git.akarpov.ru/sanspie/aego.git
cd aego
bash tools/bootstrap-native.sh
go build -o aego ./cmd/aego
```

`make cli` does the same and leaves the binary in `.cache/bin/aego`. A binary built this way remembers the checkout it was built from; a prebuilt one needs `AEGO_ROOT`, or `--engine DIR`, for native projects and exports to other systems.

### Contributing

[docs/contributing.md](https://git.akarpov.ru/sanspie/aego/src/branch/master/docs/contributing.md) describes the check levels (`make quick`, `make affected`, `make ci`) and the rules for engine code: contracts between modules, Try it scenes, diagnostics pages, history for every edit and allocation-free hot paths.
