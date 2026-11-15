#!/usr/bin/env python3
"""Render a new isolated vhost configuration. Never edits installed nginx, DNS,
TLS credentials, or the Docs/Demo vhosts. Run nginx -t before installation."""
import argparse, json, re
from pathlib import Path
from urllib.parse import urlparse
p = argparse.ArgumentParser()
p.add_argument('--root', required=True, type=Path)
p.add_argument('--app-origin', required=True)
p.add_argument('--docs-origin', required=True)
p.add_argument('--listen', default='443')
p.add_argument('--certificate', type=Path)
p.add_argument('--key', type=Path)
p.add_argument('--development', action='store_true')
p.add_argument('--output', required=True, type=Path)
a = p.parse_args()
safe_path = re.compile('^/[A-Za-z0-9/._-]+$')
for path in [a.root] + ([a.certificate, a.key] if a.certificate and a.key else []):
    if not safe_path.fullmatch(str(path)):
        raise RuntimeError('Use an absolute path without nginx metacharacters')
if not a.root.resolve() == a.root:
    raise RuntimeError('Deployment root must not be a symlink')
marker = json.loads((a.root / '.aego-asset-studio-root.json').read_text())
if not (marker['schema'] == 1 and marker['development'] == a.development):
    raise RuntimeError('Deployment root mode mismatch')
origins = []
for raw in [a.app_origin, a.docs_origin]:
    u = urlparse(raw)
    if not (u.scheme in (['http', 'https'] if a.development else ['https']) and u.netloc):
        raise RuntimeError('Required validation failed')
    if not (not u.path and (not u.query) and (not u.fragment) and (not u.username) and (not u.password)):
        raise RuntimeError('Required validation failed')
    if not re.fullmatch('[A-Za-z0-9.:-]+', u.netloc):
        raise RuntimeError('Invalid origin')
    origins.append(u)
if not re.fullmatch('(?:127\\.0\\.0\\.1:)?[0-9]{1,5}', a.listen):
    raise RuntimeError('Invalid listen address')
if a.development:
    if not a.listen.startswith('127.0.0.1:'):
        raise RuntimeError('Development nginx must be loopback-only')
    tls = ''
    listen = a.listen
else:
    if not (a.certificate and a.key):
        raise RuntimeError('Reuse provisioned TLS certificate/key paths')
    if not (a.certificate.is_file() and a.key.is_file()):
        raise RuntimeError('TLS files must already exist')
    listen = a.listen + ' ssl'
    tls = f'ssl_certificate {a.certificate};\n    ssl_certificate_key {a.key};\n    ssl_protocols TLSv1.2 TLSv1.3;'
source = Path(__file__).with_name('nginx-site.conf.in').read_text()
for key, value in {'ROOT': str(a.root), 'APP_ORIGIN': a.app_origin, 'DOCS_ORIGIN': a.docs_origin, 'LISTEN': listen, 'SERVER_NAME': origins[0].hostname, 'TLS_DIRECTIVES': tls}.items():
    source = source.replace('@' + key + '@', value)
if not not re.search('@[A-Z_]+@', source):
    raise RuntimeError('Required validation failed')
with a.output.open('x') as f:
    f.write(source)
print('Rendered only:', a.output)
