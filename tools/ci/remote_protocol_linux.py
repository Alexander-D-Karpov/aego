#!/usr/bin/env python3
"""Dispatch the installed handler through GIO using a private XDG profile.

This tests the OS entry and actual launcher, not a hand-parsed desktop file.
The confirmation remains pending; separate SDL tests exercise confirmation.
"""
import argparse
import ctypes
import ctypes.util
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
from urllib.parse import urlencode


def own_process(pid, start, binary):
    # Never signal a recycled PID, or a different executable that inherited it.
    try:
        proc = Path('/proc') / str(pid)
        stat = (proc / 'stat').read_text()
        tail = stat[stat.rfind(')') + 2:].split()
        executable = (proc / 'exe').resolve(strict=True)
        return tail[19] == start and executable in (binary, Path(sys.executable).resolve(strict=True))
    except (OSError, IndexError):
        return False


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--out', required=True, type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    out = args.out.absolute()
    out.mkdir(parents=True, exist_ok=False)
    out = out.resolve(strict=True)
    repo = Path(__file__).resolve().parents[2]
    script = (repo / 'tools/release/install.sh').read_text()
    begin = script.index('install_desktop() {')
    end = script.index('\n}\n', begin) + 2
    install_desktop = script[begin:end]
    pid, start = None, None
    with tempfile.TemporaryDirectory(prefix='aego-os-protocol-', dir=out) as temporary:
        sandbox = Path(temporary)
        for variable, subdir in (('HOME', 'home'), ('XDG_DATA_HOME', 'data'), ('XDG_CONFIG_HOME', 'config'), ('XDG_CACHE_HOME', 'cache'), ('XDG_STATE_HOME', 'state'), ('XDG_RUNTIME_DIR', 'runtime')):
            directory = sandbox / subdir
            directory.mkdir(mode=0o700)
            os.environ[variable] = str(directory)
        os.environ['DBUS_SESSION_BUS_ADDRESS'] = 'disabled:'
        os.environ['GSETTINGS_BACKEND'] = 'memory'
        os.environ.setdefault('SDL_VIDEODRIVER', 'offscreen')
        os.environ['SDL_VIDEO_DRIVER'] = os.environ['SDL_VIDEODRIVER']
        os.environ['SDL_AUDIO_DRIVER'] = 'dummy'
        os.environ['SDL_AUDIODRIVER'] = 'dummy'
        os.environ.setdefault('LIBGL_ALWAYS_SOFTWARE', '1')
        os.environ.setdefault('GALLIUM_DRIVER', 'llvmpipe')
        preferences = sandbox / 'config/aego/prefs.aetext'
        preferences.parent.mkdir(parents=True)
        preferences.write_text('prefs version=1 { language="en" updates { mode=off channel=stable } }\n')
        prefix = sandbox / 'install with spaces'
        (prefix / 'bin').mkdir(parents=True)
        record = sandbox / 'argv.json'
        child_log = out / 'linux-protocol-editor.log'
        launcher = prefix / 'bin/aego'
        # The wrapper records argv and a Linux process start time without
        # interpreting the URI; execv preserves argument boundaries and PID.
        launcher.write_text(
            f'#!{sys.executable}\nimport json, os, sys\n'
            "s=open('/proc/self/stat').read(); start=s[s.rfind(')')+2:].split()[19]\n"
            f"with open({str(record)+'.tmp'!r}, 'x') as f: json.dump({{'pid': os.getpid(), 'start': start, 'argv': sys.argv[1:]}}, f)\n"
            f'os.replace({str(record)+".tmp"!r}, {str(record)!r})\n'
            f'fd=os.open({str(child_log)!r}, os.O_WRONLY|os.O_CREAT|os.O_EXCL, 0o600)\n'
            'os.dup2(fd, 1); os.dup2(fd, 2); os.close(fd)\n'
            f'os.execv({str(binary)!r}, [{str(binary)!r}]+sys.argv[1:])\n'
        )
        launcher.chmod(0o755)
        env = dict(os.environ, AEGO_PROTOCOL_TEST_PREFIX=str(prefix))
        subprocess.run(['sh', '-c', 'set -eu\nprefix="$AEGO_PROTOCOL_TEST_PREFIX"\nroot="$prefix/share/aego"\n' + install_desktop + '\ninstall_desktop\n'], env=env, check=True, cwd=sandbox)
        desktop = sandbox / 'data/applications/aego-example.desktop'
        (out / 'aego-example.desktop').write_text(desktop.read_text())
        if '--example-link %u' not in desktop.read_text():
            raise RuntimeError('installer did not register the restricted URL entry')

        gio = ctypes.CDLL(ctypes.util.find_library('gio-2.0'))
        glib = ctypes.CDLL(ctypes.util.find_library('glib-2.0'))
        gobject = ctypes.CDLL(ctypes.util.find_library('gobject-2.0'))
        pointer = ctypes.c_void_p
        gio.g_desktop_app_info_new_from_filename.argtypes = [ctypes.c_char_p]
        gio.g_desktop_app_info_new_from_filename.restype = pointer
        gio.g_app_info_set_as_default_for_type.argtypes = [pointer, ctypes.c_char_p, pointer]
        gio.g_app_info_set_as_default_for_type.restype = ctypes.c_int
        gio.g_app_info_get_default_for_uri_scheme.argtypes = [ctypes.c_char_p]
        gio.g_app_info_get_default_for_uri_scheme.restype = pointer
        gio.g_app_info_get_id.argtypes = [pointer]
        gio.g_app_info_get_id.restype = ctypes.c_char_p
        gio.g_app_info_launch_uris.argtypes = [pointer, pointer, pointer, pointer]
        gio.g_app_info_launch_uris.restype = ctypes.c_int
        glib.g_list_append.argtypes = [pointer, pointer]
        glib.g_list_append.restype = pointer
        glib.g_list_free.argtypes = [pointer]
        gobject.g_object_unref.argtypes = [pointer]
        app = gio.g_desktop_app_info_new_from_filename(os.fsencode(desktop))
        if not app or not gio.g_app_info_set_as_default_for_type(app, b'x-scheme-handler/aego', None):
            raise RuntimeError('GIO could not register the isolated protocol handler')
        selected = gio.g_app_info_get_default_for_uri_scheme(b'aego')
        if not selected or gio.g_app_info_get_id(selected) != b'aego-example.desktop':
            raise RuntimeError('GIO did not select the installed aego handler')
        uri = 'aego://example?' + urlencode({'manifest': 'https://examples.invalid/manifest.json?literal=$(echo untouched)&quoted="value"', 'mode': 'scene'})
        encoded = ctypes.create_string_buffer(uri.encode())
        uris = glib.g_list_append(None, ctypes.cast(encoded, pointer))
        try:
            if not gio.g_app_info_launch_uris(selected, uris, None, None):
                raise RuntimeError('GIO URI launch failed')
            boot_file = sandbox / 'cache/aego/logs/boot.log'
            deadline = time.monotonic() + 30
            boot, log, argv = '', '', None
            while time.monotonic() < deadline:
                if record.exists():
                    argv = json.loads(record.read_text())
                    pid, start = argv['pid'], argv['start']
                    if argv['argv'] != ['edit', '--example-link', uri]:
                        raise RuntimeError('OS dispatch changed the URI argument boundaries')
                if boot_file.exists():
                    boot = boot_file.read_text()
                if child_log.exists():
                    log = child_log.read_text()
                if 'launcher: open' in boot and 'window logical_w=' in log:
                    break
                time.sleep(0.05)
            else:
                raise RuntimeError('editor did not reach the protocol confirmation launcher')
            if 'editor:' in boot or 'fatal:' in boot or 'panic:' in boot:
                raise RuntimeError('protocol request bypassed the launcher or failed startup')
            remote = sandbox / 'config/aego/remote-examples/projects'
            if remote.exists() and any(remote.iterdir()):
                raise RuntimeError('project was installed before confirmation')
            (out / 'linux-protocol-boot.log').write_text(boot)
            result = {'result': 'PASS', 'runner': 'Linux GIO → installed desktop entry → Aego CLI → SDL launcher',
                      'binary': str(binary), 'binarySHA256': hashlib.sha256(binary.read_bytes()).hexdigest(),
                      'argv': argv['argv'], 'registeredHandler': 'aego-example.desktop', 'isolatedXDG': True,
                      'audio': 'dummy', 'projectStarted': False,
                      'confirmation': 'pending; synthetic SDL test separately confirms and opens the project'}
            (out / 'linux-protocol-result.json').write_text(json.dumps(result, indent=2) + '\n')
            print(json.dumps(result))
        finally:
            glib.g_list_free(uris)
            if selected:
                gobject.g_object_unref(selected)
            gobject.g_object_unref(app)
            if pid and start and own_process(pid, start, binary):
                try:
                    os.kill(pid, signal.SIGTERM)
                    deadline = time.monotonic() + 5
                    while time.monotonic() < deadline and own_process(pid, start, binary):
                        time.sleep(0.05)
                    if own_process(pid, start, binary):
                        os.kill(pid, signal.SIGKILL)
                        deadline = time.monotonic() + 5
                        while time.monotonic() < deadline and own_process(pid, start, binary):
                            time.sleep(0.05)
                        if own_process(pid, start, binary):
                            raise RuntimeError('the isolated test launcher did not stop')
                except ProcessLookupError:
                    pass


if __name__ == '__main__':
    main()
