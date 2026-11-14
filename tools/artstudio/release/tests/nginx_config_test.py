#!/usr/bin/env python3
"""Exercise the proposed nginx routes in isolated temporary servers.

Run on a machine with nginx and openssl; this never edits installed nginx or
deployment roots. Test certificates and loopback ports are disposable. Passing
this fixture is not a substitute for testing the exact production artifact.
"""
import argparse
import gzip
import http.client
import os
from pathlib import Path
import socket
import ssl
import subprocess
import tempfile
import time
import unittest


def replace_once(text, old, new):
    if text.count(old) != 1:
        raise RuntimeError("expected exactly one configuration fragment: " + repr(old))
    return text.replace(old, new, 1)


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def start_nginx(binary, directory, conf, port):
    config = directory / "nginx.conf"
    config.write_text(conf)
    subprocess.run([binary, "-t", "-p", str(directory) + "/", "-c", str(config)], check=True)
    process = subprocess.Popen([binary, "-p", str(directory) + "/", "-c", str(config), "-g", "daemon off;"],
                               stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError("fixture nginx stopped: " + process.stderr.read().decode(errors="replace"))
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=.2):
                return process
        except OSError:
            time.sleep(.02)
    process.terminate()
    process.wait(timeout=5)
    raise RuntimeError("fixture nginx did not listen")


def write_file(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)


class NginxFixture(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory(prefix="aego-nginx-fixture-")
        cls.root = Path(cls.temp.name)
        # A root-started nginx drops workers to its configured unprivileged
        # account, which must be able to traverse these disposable fixtures.
        os.chmod(cls.root, 0o755)
        cls.processes = []
        cls.addClassCleanup(cls.cleanup)
        cert, key = cls.root / "cert.pem", cls.root / "key.pem"
        subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
                        "-subj", "/CN=aeditor.akarpov.ru", "-addext", "subjectAltName=DNS:aeditor.akarpov.ru",
                        "-keyout", str(key), "-out", str(cert)], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        cls.client_tls = ssl.create_default_context(cafile=str(cert))
        # The test connects to loopback; nginx's edge-to-origin TLS still checks
        # the production hostname against the test CA and certificate.
        cls.client_tls.check_hostname = False
        cls.old_body, cls.new_body = b"old pinned editor", b"current editor"
        cls.home_body = b"current Studio home"
        cls.wasm = b"\x00asm" + bytes(range(200))
        cls.gz = gzip.compress(cls.wasm, mtime=0)
        release = cls.root / "release"
        write_file(release / "releases/old/editor/index.html", cls.old_body)
        write_file(release / "releases/new/editor/index.html", cls.new_body)
        write_file(release / "releases/new/index.html", cls.home_body)
        write_file(release / "releases/old/embed/index.html", b"embedded editor")
        write_file(release / "releases/old/docs/ru/first-sprite/index.html", b"studio lesson")
        write_file(release / "releases/old/examples/aseprite/PROVENANCE.md", b"provenance")
        write_file(release / "releases/old/assets/core.0123456789abcdef.wasm", cls.wasm)
        write_file(release / "releases/old/assets/core.0123456789abcdef.wasm.gz", cls.gz)
        write_file(release / "releases/.incoming-secret/private", b"not public")
        os.symlink("releases/new", release / "current")

        app = args.app_conf.read_text()
        redirect = ('server {\n    listen 80;\n    listen [::]:80;\n'
                    '    server_name aeditor.akarpov.ru;\n'
                    '    return 301 https://aeditor.akarpov.ru$request_uri;\n}\n')
        app = replace_once(app, redirect, "")
        app = replace_once(app, '    listen 443 ssl;\n    listen [::]:443 ssl;\n'
                           '    listen 443 quic;\n    listen [::]:443 quic;\n',
                           f'    listen 127.0.0.1:{args.origin_port} ssl;\n')
        app = replace_once(app, '/etc/nginx/ssl/akarpov-wildcard/fullchain.pem', str(cert))
        app = replace_once(app, '/etc/nginx/ssl/akarpov-wildcard/privkey.pem', str(key))
        app = app.replace('/var/www/aego-asset-studio', str(release))
        cls.origin = args.origin_port
        origin_dir = cls.root / "origin"
        origin_dir.mkdir()
        cls.processes.append(start_nginx(args.nginx, origin_dir, cls.wrapper(origin_dir, app), cls.origin))

        edge = args.edge_conf.read_text()
        edge = replace_once(edge, '    include snippets/maintenance.conf;\n', '')
        edge = replace_once(edge, '    listen 127.0.0.1:18443 ssl proxy_protocol;\n'
                            '    listen 443 quic;\n    listen [::]:443 quic;\n',
                            f'    listen 127.0.0.1:{args.edge_port} ssl;\n')
        edge = replace_once(edge, '/etc/letsencrypt/live/akarpov-wildcard/fullchain.pem', str(cert))
        edge = replace_once(edge, '/etc/letsencrypt/live/akarpov-wildcard/privkey.pem', str(key))
        edge = replace_once(edge, '/etc/ssl/certs/ca-certificates.crt', str(cert))
        cls.edge = args.edge_port
        edge_dir = cls.root / "edge"
        edge_dir.mkdir()
        upstream = f'upstream ru_origin {{ server 127.0.0.1:{cls.origin}; }}\n'
        cls.processes.append(start_nginx(args.nginx, edge_dir, cls.wrapper(edge_dir, upstream + edge), cls.edge))
        bad_edge = replace_once(edge, 'proxy_ssl_name aeditor.akarpov.ru;', 'proxy_ssl_name wrong.invalid;')
        bad_edge = replace_once(bad_edge, f'127.0.0.1:{args.edge_port}', f'127.0.0.1:{args.bad_edge_port}')
        cls.bad_edge = args.bad_edge_port
        bad_dir = cls.root / "bad-edge"
        bad_dir.mkdir()
        cls.processes.append(start_nginx(args.nginx, bad_dir, cls.wrapper(bad_dir, upstream + bad_edge), cls.bad_edge))

        # The static edge is an alternative to the proxy above. Test the same
        # release tree without requiring any changes to the installed nginx.
        static_edge = args.static_edge_conf.read_text()
        static_edge = replace_once(static_edge, redirect, "")
        static_edge = replace_once(static_edge, '    include snippets/maintenance.conf;\n', '')
        static_edge = replace_once(static_edge, '    listen 127.0.0.1:18443 ssl proxy_protocol;\n'
                                   '    listen 443 quic;\n    listen [::]:443 quic;\n',
                                   f'    listen 127.0.0.1:{args.static_edge_port} ssl;\n')
        static_edge = replace_once(static_edge, '/etc/letsencrypt/live/akarpov-wildcard/fullchain.pem', str(cert))
        static_edge = replace_once(static_edge, '/etc/letsencrypt/live/akarpov-wildcard/privkey.pem', str(key))
        static_edge = static_edge.replace('/var/www/aego-asset-studio', str(release))
        cls.static_edge = args.static_edge_port
        static_dir = cls.root / "static-edge"
        static_dir.mkdir()
        cls.processes.append(start_nginx(args.nginx, static_dir, cls.wrapper(static_dir, static_edge), cls.static_edge))

        docs = args.docs_conf.read_text()
        docs = replace_once(docs, '    listen 80;\n', f'    listen 127.0.0.1:{args.docs_port};\n')
        site = cls.root / "site"
        write_file(site / "asset-studio/ru/first-sprite/index.html", b"pinned lesson")
        docs = replace_once(docs, '/usr/share/nginx/html', str(site))
        cls.docs = args.docs_port
        docs_dir = cls.root / "docs"
        docs_dir.mkdir()
        cls.processes.append(start_nginx(args.nginx, docs_dir, cls.wrapper(docs_dir, docs), cls.docs))

        docs_edge = args.docs_edge_conf.read_text()
        docs_edge = replace_once(docs_edge, '    include snippets/maintenance.conf;\n', '')
        docs_edge = replace_once(docs_edge, '    listen 127.0.0.1:18443 ssl proxy_protocol;\n',
                                 f'    listen 127.0.0.1:{args.docs_edge_port} ssl;\n')
        docs_edge = replace_once(docs_edge, '/etc/letsencrypt/live/aego.akarpov.ru/fullchain.pem', str(cert))
        docs_edge = replace_once(docs_edge, '/etc/letsencrypt/live/aego.akarpov.ru/privkey.pem', str(key))
        docs_edge = replace_once(docs_edge, '/var/www/aego-site/current', str(site))
        docs_edge = replace_once(docs_edge, '/var/www/aego-wasm', str(cls.root / 'wasm'))
        docs_edge = replace_once(docs_edge, 'proxy_pass https://ru_origin;', 'proxy_pass http://ru_origin;')
        cls.docs_edge = args.docs_edge_port
        docs_edge_dir = cls.root / "docs-edge"
        docs_edge_dir.mkdir()
        cache = cls.root / "cache"
        cache.mkdir()
        preamble = (f'upstream ru_origin {{ server 127.0.0.1:{cls.docs}; }}\n'
                    f'proxy_cache_path {cache} keys_zone=akarpov:1m;\n')
        cls.processes.append(start_nginx(args.nginx, docs_edge_dir, cls.wrapper(docs_edge_dir, preamble + docs_edge), cls.docs_edge))

    @staticmethod
    def wrapper(directory, body):
        return (f'worker_processes 1;\npid {directory}/nginx.pid;\nerror_log stderr notice;\n'
                'events { worker_connections 64; }\nhttp {\n'
                'access_log off;\ninclude /etc/nginx/mime.types;\n' + body + '\n}\n')

    @classmethod
    def cleanup(cls):
        for process in reversed(cls.processes):
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
            process.stderr.close()
        cls.temp.cleanup()

    def request(self, port, path, method="GET", headers=None):
        tls = port != self.docs
        c = (http.client.HTTPSConnection("127.0.0.1", port, context=self.client_tls, timeout=5) if tls
             else http.client.HTTPConnection("127.0.0.1", port, timeout=5))
        c.request(method, path, headers={"Host": "aeditor.akarpov.ru", **(headers or {})})
        r = c.getresponse()
        result = r.status, dict(r.getheaders()), r.read()
        c.close()
        return result

    def test_routes_headers_and_immutable_bytes_on_origin_and_edge(self):
        for port in (self.origin, self.edge, self.static_edge):
            with self.subTest(port=port):
                status, _, body = self.request(port, "/")
                self.assertEqual((status, body), (200, self.home_body))
                status, _, body = self.request(port, "/editor/")
                self.assertEqual((status, body), (200, self.new_body))
                status, headers, body = self.request(port, "/r/old/editor/?session=private")
                self.assertEqual((status, body), (200, self.old_body))
                self.assertIn("frame-ancestors 'none'", headers["Content-Security-Policy"])
                status, headers, _ = self.request(port, "/r/old/embed/")
                self.assertEqual(status, 200)
                self.assertIn("frame-ancestors https://aego.akarpov.ru https://aeditor.akarpov.ru", headers["Content-Security-Policy"])
                self.assertNotIn("https://aestudio.akarpov.ru", headers["Content-Security-Policy"])
                status, headers, body = self.request(port, "/r/old/assets/core.0123456789abcdef.wasm", headers={"Accept-Encoding": "gzip"})
                self.assertEqual((status, body), (200, self.gz))
                self.assertEqual(headers["Content-Type"], "application/wasm")
                self.assertEqual(headers["Content-Encoding"], "gzip")
                self.assertIn("immutable", headers["Cache-Control"])
                status, headers, body = self.request(port, "/r/old/examples/aseprite/PROVENANCE.md")
                self.assertEqual((status, body), (200, b"provenance"))
                self.assertEqual(headers["Content-Type"], "application/octet-stream")
                self.assertEqual(self.request(port, "/r/.incoming-secret/private")[0], 404)
                self.assertEqual(self.request(port, "/r/%2eincoming-secret/private")[0], 404)
                self.assertEqual(self.request(port, "/no-such-file")[0], 404)
                self.assertEqual(self.request(port, "/editor/", method="POST")[0], 403)
                status, _, body = self.request(port, "/r/old/editor/", method="HEAD")
                self.assertEqual((status, body), (200, b""))
        self.assertEqual(self.request(self.bad_edge, "/editor/")[0], 502, "edge must reject an origin with the wrong TLS name")

    def test_studio_guide_csp_allows_only_the_editor_and_local_blob_frames(self):
        for port in (self.origin, self.edge, self.static_edge):
            with self.subTest(port=port):
                status, headers, body = self.request(port, "/r/old/docs/ru/first-sprite/")
                self.assertEqual((status, body), (200, b"studio lesson"))
                policy = headers["Content-Security-Policy"]
                self.assertIn("frame-src https://aeditor.akarpov.ru blob:;", policy)
                self.assertNotIn("https://aestudio.akarpov.ru", policy)
                self.assertIn("script-src 'self';", policy)
                self.assertIn("frame-ancestors 'none'", policy)
                self.assertNotIn("'unsafe-inline'", policy)

    def test_docs_guide_csp(self):
        for port in (self.docs, self.docs_edge):
            with self.subTest(port=port):
                status, headers, body = self.request(port, "/asset-studio/ru/first-sprite/")
                self.assertEqual((status, body), (200, b"pinned lesson"))
                self.assertIn("frame-src https://aeditor.akarpov.ru blob:;", headers["Content-Security-Policy"])
                self.assertNotIn("https://aestudio.akarpov.ru", headers["Content-Security-Policy"])
                self.assertIn("script-src 'self'", headers["Content-Security-Policy"])
                self.assertNotIn("'unsafe-inline'", headers["Content-Security-Policy"])

    def test_lesson_without_trailing_slash_redirects(self):
        for port in (self.docs, self.docs_edge):
            with self.subTest(port=port):
                status, headers, _ = self.request(port, "/asset-studio/ru/first-sprite")
                self.assertEqual(status, 301)
                self.assertTrue(headers["Location"].endswith("/asset-studio/ru/first-sprite/"))


if __name__ == "__main__":
    p = argparse.ArgumentParser()
    p.add_argument("--nginx", default="nginx")
    p.add_argument("--app-conf", type=Path, required=True)
    p.add_argument("--edge-conf", type=Path, required=True)
    p.add_argument("--static-edge-conf", type=Path, default=Path(__file__).resolve().parent / "../../../../deploy/edge/aeditor-static.conf")
    p.add_argument("--docs-conf", type=Path, required=True)
    p.add_argument("--docs-edge-conf", type=Path, required=True)
    args = p.parse_args()
    ports = set()
    while len(ports) < 6:
        ports.add(free_port())
    args.origin_port, args.edge_port, args.bad_edge_port, args.docs_port, args.docs_edge_port, args.static_edge_port = ports
    unittest.main(argv=[__file__], verbosity=2)
