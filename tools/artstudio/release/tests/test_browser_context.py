"""Focused tests for the Studio browser-context adapter; no browser required."""
import hashlib
import os
from pathlib import Path
import socket
import tempfile
import unittest
from unittest.mock import MagicMock, patch

from browser_context import _fixture_ca, browser_context


class BrowserContextTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory(prefix='studio-browser-context-test-')
        self.addCleanup(self.work.cleanup)
        self.root = Path(self.work.name)
        self.ca = self.root / 'ca.pem'
        # PEM decoding is tested here; certutil and real certificate chains are
        # exercised independently in the isolated browser TLS fixture.
        self.ca.write_text('-----BEGIN CERTIFICATE-----\nZm9v\n-----END CERTIFICATE-----\n')
        self.digest = hashlib.sha256(self.ca.read_bytes()).hexdigest()
        self.local = [(socket.AF_INET, socket.SOCK_STREAM, 6, '', ('127.0.0.1', 443))]
        self.fixture_env = {'AEGO_STUDIO_FIXTURE_CA': str(self.ca),
                            'AEGO_STUDIO_FIXTURE_CA_SHA256': self.digest,
                            'AEGO_STUDIO_FIXTURE_PROFILE_ROOT': str(self.root)}

    def test_default_context_closes_browser_and_context_on_failure(self):
        playwright = MagicMock()
        browser = playwright.firefox.launch.return_value
        browser.version = 'test-version'
        context = browser.new_context.return_value
        with patch.dict(os.environ, {}, clear=True):
            with self.assertRaisesRegex(RuntimeError, 'body failure'):
                with browser_context(playwright, 'firefox', {'headless': True},
                                     {'viewport': {'width': 123, 'height': 456}},
                                     urls=['http://127.0.0.1/']) as (actual, version):
                    self.assertIs(actual, context)
                    self.assertEqual(version, 'test-version')
                    raise RuntimeError('body failure')
        playwright.firefox.launch.assert_called_once_with(headless=True)
        browser.new_context.assert_called_once_with(viewport={'width': 123, 'height': 456})
        context.close.assert_called_once_with()
        browser.close.assert_called_once_with()

    def test_default_browser_is_closed_when_context_creation_fails(self):
        playwright = MagicMock()
        browser = playwright.chromium.launch.return_value
        browser.new_context.side_effect = RuntimeError('creation failed')
        with patch.dict(os.environ, {}, clear=True), self.assertRaisesRegex(RuntimeError, 'creation failed'):
            with browser_context(playwright, 'chromium', {}, {}, urls=[]):
                self.fail('context creation should have failed')
        browser.close.assert_called_once_with()

    def test_fixture_requires_all_environment_fields(self):
        playwright = MagicMock()
        with patch.dict(os.environ, {'AEGO_STUDIO_FIXTURE_CA': str(self.ca)}, clear=True):
            with self.assertRaisesRegex(ValueError, 'all isolated'):
                with browser_context(playwright, 'firefox', {}, {}, urls=['https://localhost/']):
                    self.fail('partial fixture configuration was accepted')
        playwright.firefox.launch.assert_not_called()
        playwright.firefox.launch_persistent_context.assert_not_called()

    def test_fixture_rejects_tls_bypasses_before_launch(self):
        playwright = MagicMock()
        cases = [({}, {'ignore_https_errors': True}),
                 ({'ignore_https_errors': True}, {}),
                 ({'args': ['--ignore-certificate-errors=all']}, {}),
                 ({'args': ['--ignore-certificate-errors-spki-list=anything']}, {}),
                 ({'args': ['--allow-insecure-localhost']}, {}),
                 ({'args': ['--disable-web-security']}, {})]
        with patch.dict(os.environ, self.fixture_env, clear=True):
            for launch, context in cases:
                with self.subTest(launch=launch, context=context):
                    with self.assertRaises(ValueError):
                        with browser_context(playwright, 'firefox', launch, context, urls=['https://localhost/']):
                            self.fail('insecure fixture was accepted')
        playwright.firefox.launch_persistent_context.assert_not_called()

    def test_ca_identity_and_all_origin_addresses_are_validated(self):
        with patch('browser_context.socket.getaddrinfo', return_value=self.local):
            raw, der = _fixture_ca(self.ca, self.digest, ['https://localhost/'])
            self.assertEqual(raw, self.ca.read_bytes())
            self.assertEqual(der, b'foo')
            for ca, digest, urls in (
                (self.ca, '0' * 64, ['https://localhost/']),
                (self.ca, self.digest, []),
                (self.ca, self.digest, ['http://localhost/']),
                (self.ca, self.digest, ['https://user@localhost/']),
                (self.ca, self.digest, ['https://@localhost/']),
                (Path('relative.pem'), self.digest, ['https://localhost/']),
            ):
                with self.subTest(ca=ca, urls=urls), self.assertRaises(ValueError):
                    _fixture_ca(ca, digest, urls)
            link = self.root / 'link.pem'
            link.symlink_to(self.ca)
            with self.assertRaises(ValueError):
                _fixture_ca(link, self.digest, ['https://localhost/'])
        mixed = self.local + [(socket.AF_INET, socket.SOCK_STREAM, 6, '', ('1.1.1.1', 443))]
        with patch('browser_context.socket.getaddrinfo', return_value=mixed), self.assertRaises(ValueError):
            _fixture_ca(self.ca, self.digest, ['https://localhost/'])

    def test_oversized_ca_is_rejected(self):
        self.ca.write_bytes(b'x' * 65537)
        with self.assertRaises(ValueError):
            _fixture_ca(self.ca, hashlib.sha256(self.ca.read_bytes()).hexdigest(), ['https://localhost/'])


if __name__ == '__main__':
    unittest.main()
