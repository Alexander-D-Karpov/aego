#!/usr/bin/env python3
"""Verify the prepared Aego CI environment; never install or repair it."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys
import tempfile


IMAGE = Path('/opt/aego')
INPUTS = {
    'third_party/sdl/version.txt': 'seed/third_party/sdl/version.txt',
    'tools/bootstrap-native.sh': 'seed/tools/bootstrap-native.sh',
    'tools/websmoke/package.json': 'websmoke/package.json',
    'tools/websmoke/package-lock.json': 'websmoke/package-lock.json',
    'tools/ci/env/Dockerfile': 'ci-env/Dockerfile',
    'tools/ci/env/requirements.txt': 'ci-env/requirements.txt',
    'tools/ci/env/preflight.py': 'ci-env/preflight.py',
    'tools/ci/env/browser_smoke.py': 'ci-env/browser_smoke.py',
}
APT_PACKAGES = (
    'build-essential', 'cmake', 'ninja-build', 'pkg-config', 'openssl',
    'python3-venv', 'xvfb', 'xauth', 'xdotool',
    'libgl1-mesa-dev', 'libegl1-mesa-dev', 'libgles2-mesa-dev',
    'libx11-dev', 'libxext-dev', 'libxrandr-dev', 'libxcursor-dev', 'libxfixes-dev',
    'libxi-dev', 'libxss-dev', 'libxtst-dev', 'libxkbcommon-dev',
    'libwayland-dev', 'wayland-protocols', 'libdecor-0-dev',
    'libasound2-dev', 'libpulse-dev', 'libdrm-dev', 'libgbm-dev', 'libudev-dev',
    'libdbus-1-dev', 'libibus-1.0-dev', 'glslang-tools',
    'fonts-dejavu-core', 'fonts-liberation', 'nsis', 'squashfs-tools',
)


def fail(message):
    raise ValueError(message)


def digest(path):
    if path.is_symlink() or not path.is_file():
        fail('expected a regular file: ' + str(path))
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for part in iter(lambda: stream.read(1024 * 1024), b''):
            h.update(part)
    return h.hexdigest()


def command(args, *, cwd=None, env=None):
    result = subprocess.run(args, cwd=cwd, env=env, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, text=True, timeout=30)
    if result.returncode:
        fail(f'{args[0]} failed ({result.returncode}): {result.stderr.strip()[:500]}')
    return result.stdout.strip()


def require(value, expected, name):
    if value != expected:
        fail(f'{name}: expected {expected!r}, got {value!r}')


def check_input_set(base, lock, image=False):
    inputs = lock.get('inputs')
    if not isinstance(inputs, dict) or set(inputs) != set(INPUTS):
        fail('environment lock does not specify the exact input set')
    for source, image_name in INPUTS.items():
        expected = inputs[source]
        if not isinstance(expected, str) or not re.fullmatch(r'[0-9a-f]{64}', expected):
            fail('invalid input SHA-256: ' + source)
        path = base / (image_name if image else source)
        require(digest(path), expected, source)
    if not image:
        patches = base / 'third_party/sdl/patches'
        if patches.is_symlink() or (patches.exists() and (not patches.is_dir() or any(patches.iterdir()))):
            fail('SDL patch inputs changed; rebuild and review the environment')


def check_go_directives(root, go_version):
    if (root / 'go.work').exists() or (root / 'go.work').is_symlink():
        fail('go.work requires an explicitly reviewed CI environment')
    text = (root / 'go.mod').read_text(encoding='utf-8')
    go = re.findall(r'^go\s+([^\s]+).*$', text, re.MULTILINE)
    tools = re.findall(r'^toolchain\s+([^\s]+).*$', text, re.MULTILINE)
    if len(go) != 1 or len(tools) > 1:
        fail('unrecognized Go or toolchain directive')
    current = tuple(map(int, go_version.removeprefix('go').split('.')))
    for directive in go + [value.removeprefix('go') for value in tools if value.startswith('go')]:
        match = re.fullmatch(r'(\d+)\.(\d+)(?:\.(\d+))?', directive)
        if not match:
            fail('unrecognized Go or toolchain directive')
        required = tuple(int(part or '0') for part in match.groups())
        if required > current:
            fail('Go directive requires a newer toolchain than the prepared image')
    if any(not value.startswith('go') for value in tools):
        fail('unrecognized Go or toolchain directive')


def check_go_workspace(root):
    workspace = command(['go', 'env', 'GOWORK'], cwd=root)
    if workspace not in ('', 'off'):
        fail('ambient GOWORK changes the dependency graph: ' + workspace)


def check_platform():
    require(platform.system(), 'Linux', 'host OS')
    if platform.machine() not in ('x86_64', 'amd64'):
        fail('host architecture is not amd64: ' + platform.machine())
    require(command(['dpkg', '--print-architecture']), 'amd64', 'package architecture')
    values = json.loads(command(['go', 'env', '-json', 'GOOS', 'GOARCH', 'GOHOSTOS', 'GOHOSTARCH', 'CGO_ENABLED']))
    for name, expected in (('GOOS', 'linux'), ('GOARCH', 'amd64'), ('GOHOSTOS', 'linux'),
                           ('GOHOSTARCH', 'amd64'), ('CGO_ENABLED', '1')):
        require(values.get(name), expected, name)
    node = json.loads(command(['node', '-p', 'JSON.stringify({platform:process.platform,arch:process.arch})']))
    require(node, {'platform': 'linux', 'arch': 'x64'}, 'Node platform')
    return values


def check_apt(lock):
    fmt = '${binary:Package}|${db:Status-Status}|${Version}\n'
    lines = command(['dpkg-query', '-W', '-f=' + fmt, *APT_PACKAGES]).splitlines()
    installed = {}
    for line in lines:
        parts = line.split('|')
        if len(parts) != 3:
            fail('invalid dpkg-query output')
        name, status, version = parts
        name = name.split(':', 1)[0]
        if name in installed or status != 'installed' or not version:
            fail('missing, duplicate or unconfigured apt package: ' + name)
        installed[name] = version
    if set(installed) != set(APT_PACKAGES):
        fail('installed apt packages do not match the required set')
    require(installed['glslang-tools'], lock['tools']['glslang_apt'], 'glslang apt package')
    return installed


def check_tools(lock):
    expected = lock['tools']
    require(os.environ.get('GOTOOLCHAIN'), 'local', 'GOTOOLCHAIN')
    require(os.environ.get('PLAYWRIGHT_BROWSERS_PATH'), '/ms-playwright', 'PLAYWRIGHT_BROWSERS_PATH')
    for name in ('go', 'node', 'npm', 'pkg-config', 'gcc', 'cmake', 'ninja', 'xvfb-run', 'git', 'glslangValidator', 'dpkg', 'dpkg-query'):
        if not shutil.which(name):
            fail('missing required tool: ' + name)
    require(command(['go', 'env', 'GOVERSION']), expected['go'], 'Go')
    require(command(['node', '--version']), expected['node'], 'Node')
    require(command(['npm', '--version']), expected['npm'], 'npm')
    glslang = command(['glslangValidator', '--version'])
    match = re.search(r'^Glslang Version:\s*\d+:([0-9.]+)', glslang, re.MULTILINE)
    require(match.group(1) if match else None, expected['glslang'], 'glslang')
    info = command(['go', 'version', '-m', str(IMAGE / 'bin/staticcheck')])
    if not re.search(r'(?m)^\s*mod\s+honnef\.co/go/tools\s+' + re.escape(expected['staticcheck']) + r'(?:\s|$)', info):
        fail('Staticcheck build info does not match the lock')
    python = str(IMAGE / 'python/bin/python')
    script = ('import importlib.metadata as m,json,sys; '
              'print(json.dumps({"python":"%d.%d" % sys.version_info[:2],'
              '"playwright":m.version("playwright"),"pyee":m.version("pyee"),'
              '"greenlet":m.version("greenlet"),"typing-extensions":m.version("typing-extensions")}))')
    versions = json.loads(command([python, '-B', '-c', script]))
    for name in ('python', 'playwright', 'pyee', 'greenlet', 'typing-extensions'):
        require(versions[name], expected[name], name)
    return versions


def check_browsers(web_root, lock):
    node_script = '''const p=require('playwright-core');
const v=require('playwright-core/package.json').version;
console.log(JSON.stringify({version:v,chromium:p.chromium.executablePath(),firefox:p.firefox.executablePath()}));'''
    node = json.loads(command(['node', '-e', node_script], cwd=web_root))
    require(node['version'], lock['tools']['playwright'], 'Node Playwright')
    python_script = '''import json
from playwright.sync_api import sync_playwright
with sync_playwright() as p:
 print(json.dumps({'chromium':p.chromium.executable_path,'firefox':p.firefox.executable_path}))'''
    py = json.loads(command([str(IMAGE / 'python/bin/python'), '-B', '-c', python_script]))
    for name in ('chromium', 'firefox'):
        require(node[name], py[name], name + ' Python/Node browser')
        executable = Path(node[name])
        if not executable.is_file() or not os.access(executable, os.X_OK):
            fail('missing ' + name + ' executable: ' + str(executable))
    return {name: node[name] for name in ('chromium', 'firefox')}


def check_sdl(prefix, version):
    include = prefix / 'include'
    library = prefix / 'lib/libSDL3.a'
    if not (include / 'SDL3/SDL.h').is_file() or not library.is_file():
        fail('prepared SDL headers or static library are missing: ' + str(prefix))
    env = dict(os.environ, PKG_CONFIG_PATH=str(prefix / 'lib/pkgconfig'))
    require(command(['pkg-config', '--define-prefix', '--modversion', 'sdl3'], env=env), version, 'SDL pkg-config')
    with tempfile.TemporaryDirectory(prefix='aego-ci-sdl-preflight-') as temp:
        source = Path(temp) / 'probe.c'
        binary = Path(temp) / 'probe'
        source.write_text('''#include <SDL3/SDL.h>
#include <stdio.h>
int main(void) {
    if (SDL_GetVersion() != SDL_VERSION) return 1;
    if (!SDL_Init(SDL_INIT_VIDEO | SDL_INIT_AUDIO)) {
        fprintf(stderr, "SDL_Init: %s\\n", SDL_GetError());
        return 2;
    }
    SDL_Quit();
    return 0;
}
''')
        command(['gcc', '-I' + str(include), str(source), str(library), '-lm', '-ldl', '-lpthread', '-lrt', '-o', str(binary)])
        command([str(binary)], env=dict(os.environ, SDL_VIDEODRIVER='dummy', SDL_VIDEO_DRIVER='dummy',
                                        SDL_AUDIO_DRIVER='dummy', SDL_AUDIODRIVER='dummy'))


def verify(root=None):
    image_lock = IMAGE / 'ci-env/lock.json'
    lock = json.loads(image_lock.read_text(encoding='utf-8'))
    if lock.get('schema') != 1 or lock.get('platform') != 'linux/amd64':
        fail('unsupported environment lock')
    check_input_set(IMAGE, lock, image=True)
    platform_info = check_platform()
    apt_packages = check_apt(lock)
    versions = check_tools(lock)
    check_sdl(IMAGE / 'native/linux_amd64', lock['tools']['sdl'])
    browsers = check_browsers(IMAGE / 'websmoke', lock)
    if root is not None:
        root = root.resolve(strict=True)
        source_lock = root / 'tools/ci/env/lock.json'
        if source_lock.is_symlink() or source_lock.read_bytes() != image_lock.read_bytes():
            fail('checkout and prepared image locks differ')
        check_input_set(root, lock)
        check_go_directives(root, lock['tools']['go'])
        check_go_workspace(root)
        node_modules = root / 'tools/websmoke/node_modules'
        if not node_modules.is_symlink() or node_modules.resolve(strict=True) != (IMAGE / 'websmoke/node_modules').resolve(strict=True):
            fail('workspace does not use the prepared Node dependency tree')
        check_sdl(root / '.cache/native/linux_amd64', lock['tools']['sdl'])
        require(check_browsers(root / 'tools/websmoke', lock), browsers, 'workspace browser resolution')
    return {'status': 'PASS', 'platform': lock['platform'], 'platform_info': platform_info,
            'apt_packages': apt_packages, 'versions': versions,
            'go': lock['tools']['go'], 'node': lock['tools']['node'], 'browsers': browsers,
            'scope': 'image and workspace' if root else 'image only',
            'image_digest': None, 'note': 'digest must be established by the runner/image reference'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument('--image-only', action='store_true')
    mode.add_argument('--root', type=Path)
    args = parser.parse_args()
    try:
        print(json.dumps(verify(args.root), indent=2))
    except (OSError, ValueError, KeyError, TypeError, json.JSONDecodeError, subprocess.SubprocessError) as error:
        print('CI environment preflight failed: ' + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
