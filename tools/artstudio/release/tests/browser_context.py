"""Browser contexts for Studio acceptance, including an isolated TLS fixture.

The optional fixture mode seeds a fresh NSS database; it never changes the
system trust store or disables certificate verification. Loopback DNS checks
are not network isolation: the caller must separately establish and verify a
private network namespace without an external route.
"""
from contextlib import contextmanager
import hashlib
import ipaddress
import os
from pathlib import Path
import re
import socket
import ssl
import subprocess
import tempfile
from urllib.parse import urlsplit


_FIXTURE_ENV = ('AEGO_STUDIO_FIXTURE_CA', 'AEGO_STUDIO_FIXTURE_CA_SHA256',
                'AEGO_STUDIO_FIXTURE_PROFILE_ROOT')


def _fixture_ca(path, expected_sha256, urls):
    if not re.fullmatch('[0-9a-f]{64}', expected_sha256):
        raise ValueError('an independently supplied fixture CA SHA-256 is required')
    path = Path(path)
    if not path.is_absolute() or path.is_symlink() or not path.is_file():
        raise ValueError('fixture CA must be an existing absolute regular file')
    with path.open('rb') as source:
        raw = source.read(65537)
    if len(raw) > 65536 or hashlib.sha256(raw).hexdigest() != expected_sha256:
        raise ValueError('fixture CA bytes do not match the expected bounded input')
    text = raw.decode('ascii')
    if text.count('-----BEGIN CERTIFICATE-----') != 1 or text.count('-----END CERTIFICATE-----') != 1:
        raise ValueError('fixture CA must contain exactly one PEM certificate')
    if 'PRIVATE KEY' in text:
        raise ValueError('fixture CA file contains private key material')
    der = ssl.PEM_cert_to_DER_cert(text)
    if not urls:
        raise ValueError('fixture origins must be supplied')
    for raw_url in urls:
        url = urlsplit(raw_url)
        if (url.scheme != 'https' or not url.hostname or
                url.username is not None or url.password is not None):
            raise ValueError('fixture origin must use HTTPS without credentials')
        answers = socket.getaddrinfo(url.hostname, url.port or 443, type=socket.SOCK_STREAM)
        addresses = {item[4][0] for item in answers}
        if not addresses or any(not ipaddress.ip_address(address).is_loopback for address in addresses):
            raise ValueError(f'fixture origin is not loopback-only: {url.hostname}')
    return raw, der


@contextmanager
def browser_context(playwright, name, launch_options, context_options, *, urls):
    """Yield one context and its browser version, cleaning up on every exit.

    With none of the fixture variables set this uses the original ordinary
    Playwright launch/new_context path. Fixture mode requires all three
    variables, a loopback HTTPS URL list and an independently supplied CA hash.
    """
    if name not in ('chromium', 'firefox'):
        raise ValueError('unsupported browser')
    fixture = any(key in os.environ for key in _FIXTURE_ENV)
    values = [os.environ.get(key, '') for key in _FIXTURE_ENV]
    if fixture and not all(values):
        raise ValueError('all isolated Studio fixture variables are required')
    if not fixture:
        browser = getattr(playwright, name).launch(**launch_options)
        context = None
        try:
            context = browser.new_context(**context_options)
            yield context, browser.version
        finally:
            try:
                if context:
                    context.close()
            finally:
                browser.close()
        return

    if (context_options.get('ignore_https_errors', False) is not False or
            launch_options.get('ignore_https_errors', False) is not False):
        raise ValueError('TLS verification cannot be disabled in fixture mode')
    unsafe = ('--ignore-certificate-errors', '--allow-insecure-localhost', '--disable-web-security')
    if any(arg.startswith('--ignore-certificate-errors') or arg.split('=', 1)[0] in unsafe
           for arg in launch_options.get('args', ())):
        raise ValueError('insecure browser launch argument in fixture mode')
    ca_path, ca_sha256, parent = values
    raw, der = _fixture_ca(ca_path, ca_sha256, urls)
    if not Path(parent).is_absolute():
        raise ValueError('fixture profile parent must be an absolute path')
    parent = Path(parent).resolve(strict=True)
    if not parent.is_dir():
        raise ValueError('fixture profile parent must be a directory')

    with tempfile.TemporaryDirectory(prefix='aego-fixture-browser-', dir=parent) as work:
        work = Path(work)
        home = work / 'home'
        home.mkdir(mode=0o700)
        database = home / '.pki/nssdb' if name == 'chromium' else work / 'firefox-profile'
        database.mkdir(parents=True, mode=0o700)
        options = dict(launch_options)
        env = dict(options.pop('env', os.environ))
        env.update(HOME=str(home), XDG_CONFIG_HOME=str(home), XDG_DATA_HOME=str(home), XDG_CACHE_HOME=str(home))
        options['env'] = env
        browser = None
        context = None
        try:
            subprocess.run(['certutil', '-N', '--empty-password', '-d', 'sql:' + str(database)], check=True,
                           stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30)
            subprocess.run(['certutil', '-A', '-d', 'sql:' + str(database), '-n', 'Aego fixture CA', '-t', 'C,,',
                            '-i', ca_path], check=True, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                           stderr=subprocess.PIPE, timeout=30)
            imported = subprocess.check_output(['certutil', '-L', '-d', 'sql:' + str(database),
                                                '-n', 'Aego fixture CA', '-a'], stdin=subprocess.DEVNULL,
                                               stderr=subprocess.PIPE, timeout=30).decode('ascii')
            with Path(ca_path).open('rb') as source:
                current_ca = source.read(65537)
            if ssl.PEM_cert_to_DER_cert(imported) != der or current_ca != raw:
                raise ValueError('fixture CA or NSS import changed')
            if name == 'firefox':
                context = playwright.firefox.launch_persistent_context(str(database), **options, **context_options)
                browser = context.browser
            else:
                browser = playwright.chromium.launch(**options)
                context = browser.new_context(**context_options)
            if browser is None:
                raise RuntimeError('browser context does not expose its browser version')
            yield context, browser.version
        finally:
            try:
                if context:
                    context.close()
            finally:
                if browser and name == 'chromium':
                    browser.close()
