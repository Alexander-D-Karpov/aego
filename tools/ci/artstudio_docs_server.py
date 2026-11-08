#!/usr/bin/env python3
"""Loopback-only test host for a freshly generated, pinned documentation site.

This is not the production docs server and never uploads or changes files.
The frame-src policy admits only the specified loopback Studio origin.
"""
import argparse
import functools
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit


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

    class Handler(SimpleHTTPRequestHandler):
        def translate_path(self, request_path):
            resolved = super().translate_path(request_path)
            # Match the existing docs site's `try_files $uri $uri.html $uri/`:
            # only serve an extensionless route when that exact HTML exists.
            if not Path(resolved).exists() and Path(resolved + '.html').is_file():
                return resolved + '.html'
            return resolved

        def end_headers(self):
            self.send_header('Content-Security-Policy',
                "default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval'; "
                "style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; "
                "connect-src 'self'; frame-src " + args.app_origin +
                "; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
            self.send_header('X-Content-Type-Options', 'nosniff')
            self.send_header('Cache-Control', 'no-store')
            super().end_headers()

    handler = functools.partial(Handler, directory=str(site))
    server = ThreadingHTTPServer(('127.0.0.1', args.port), handler)
    print(f'Local docs: http://127.0.0.1:{args.port}/ (frame-src {args.app_origin})', flush=True)
    server.serve_forever()


if __name__ == '__main__':
    main()
