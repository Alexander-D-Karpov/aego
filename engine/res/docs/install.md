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
- **macOS** — first check that the selected release actually includes a macOS download. If it includes a disk image, open it and drag Aego to Applications. The app is not notarized yet: on first launch, right-click → Open, or allow it in System Settings → Privacy & Security. A missing download is not an available installer; use the source-build instructions below instead.
- **Linux** — download the AppImage, make it executable and run it. Gear Lever or AppImageLauncher add it to your menu; the install script below does that by itself. A tarball is also available.

## Command line

For servers, CI and anyone who prefers a terminal:

```sh
curl -fsSL https://aego.akarpov.ru/install.sh | sh
curl -fsSL https://aego.akarpov.ru/install.sh | sh -s -- --nightly
curl -fsSL https://aego.akarpov.ru/install.sh | sh -s -- --version v0.11.0
```

```powershell
irm https://aego.akarpov.ru/install.ps1 | iex
```

On Windows, set `$env:AEGO_CHANNEL = "nightly"`, or a tag such as `"v0.11.0"`, before the command to pick another build. The script verifies the checksum, installs into `~/.local` (or `%LOCALAPPDATA%\Programs\Aego`) and prints what it did.

## Updates

A copy installed by the installer, `install.sh` or the AppImage updates itself. At every start, and every six hours of a long session, Aego looks for a newer version of its channel; the check takes at most three seconds and never holds up the window.

- **Download and install on restart**, the default, downloads the new version in the background, with its progress in the Task Center. The launcher and the status bar then offer **Restart to Update**, **Later**, **Skip This Version** and **Release Notes**. Nothing installs while you work: the update takes place when you restart, and Aego opens the same project on the new version. A skipped version is not offered again; the next one is.
- **Ask before downloading** offers the new version first; **Only when I check** waits for **Help → Check for Updates**.
- **Help → Nightly Updates**, or the channel in the launcher, follows a build of every master commit instead of stable versions.
- Every release is signed. A release without a valid signature, or a download whose checksum does not match, is never installed.
- The version before the update is kept: **Help → Restore Previous Version** or the launcher goes back to it. When a new version stops in its first ten seconds, the next start returns to the previous one by itself and says so.
- `aego update` does the same from a terminal: `aego update check`, `aego update apply`, `aego update restore` and `aego update status`.

On Windows the installer updates in place when the editor quits and opens it again; shortcuts and file associations stay. The installer of the replaced version is kept, and restoring runs it the same way. Only one previous version is kept; older downloads are removed when the new version starts. On macOS the new `.app` replaces the old one next to it. A copy built from source, or installed by a distribution's package manager, does not update itself: the launcher points to this page.

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

The desktop renderer requires OpenGL 3.3 and a driver that exposes it to the application. Browser builds require WebGL2 enabled in the browser. Hardware age alone does not establish compatibility: virtual machines, remote desktops and disabled browser acceleration may not provide the required API. Check the release's OS and CPU architecture before downloading. If startup reports an OpenGL error, check the driver and graphics environment; do not assume the project is damaged.

## Native Go extensions

Optional. Projects that add their own Go code need Go installed; the editor says so when it finds none. Script-only projects work without Go.

Next: [Your first project →](ref:quickstart)
