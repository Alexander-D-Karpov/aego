#!/usr/bin/env python3
"""Loopback-only test host for a freshly generated, pinned documentation site.

This is not the production docs server and never uploads or changes files.
The frame-src policy admits the specified loopback Studio origin and local
blob downloads used by Firefox's sandboxed iframe navigation checks.
"""
import argparse
import functools
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit


def make_handler(site, app_origin):
    class Handler(SimpleHTTPRequestHandler):
        def translate_path(self, request_path):
            resolved = super().translate_path(request_path)
            # A page and a directory of its child pages may share the name.
            # Prefer the page for /physics instead of redirecting to a listing.
            if not Path(resolved).is_file() and Path(resolved + '.html').is_file():
                return resolved + '.html'
            return resolved

        def send_head(self):
            request = urlsplit(self.path)
            if request.path != '/' and request.path.endswith('/'):
                target = '/' + request.path.lstrip('/').rstrip('/')
                article = super().translate_path(target) + '.html'
                if Path(article).is_file():
                    self.send_response(308)
                    self.send_header('Location', target + ('?' + request.query if request.query else ''))
                    self.send_header('Content-Length', '0')
                    self.end_headers()
                    return None
            return super().send_head()

        def list_directory(self, path):
            self.send_error(404, 'No document at this address')
            return None

        def end_headers(self):
            # The bundled Go Web Audio backend creates its AudioWorklet module
            # as a blob. Permit that module without enabling unsafe-eval, file:
            # access, wildcard origins or external network connections.
            self.send_header('Content-Security-Policy',
                "default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' blob:; "
                "worker-src 'self' blob:; "
                "style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; "
                "connect-src 'self'; frame-src " + app_origin + " blob:" +
                "; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
            self.send_header('X-Content-Type-Options', 'nosniff')
            self.send_header('Cache-Control', 'no-store')
            super().end_headers()

    return functools.partial(Handler, directory=str(site))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--site', required=True, type=Path)
    parser.add_argument('--port', required=True, type=int)
    parser.add_argument('--app-origin', required=True)
    args = parser.parse_args()
    site = args.site.resolve(strict=True)
    origin = urlsplit(args.app_origin)
    if (not site.is_dir() or not 1024 <= args.port <= 65535 or
            origin.scheme != 'http' or origin.hostname != '127.0.0.1' or
            origin.port is None or origin.username or origin.password or
            origin.path or origin.query or origin.fragment or
            origin.geturl() != args.app_origin):
        raise SystemExit('expected an existing site, loopback app origin and unprivileged port')

    handler = make_handler(site, args.app_origin)
    server = ThreadingHTTPServer(('127.0.0.1', args.port), handler)
    print(f'Local docs: http://127.0.0.1:{args.port}/ (frame-src {args.app_origin})', flush=True)
    server.serve_forever()


if __name__ == '__main__':
    main()
