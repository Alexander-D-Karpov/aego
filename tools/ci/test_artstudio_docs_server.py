import http.client
from http.server import ThreadingHTTPServer
from pathlib import Path
import tempfile
import threading
import unittest

from artstudio_docs_server import make_handler


class DocsServerTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        root = Path(self.directory.name)
        (root / 'physics').mkdir()
        (root / 'physics.html').write_text('<h1>Physics</h1>')
        (root / 'physics' / 'units.html').write_text('<h1>Units</h1>')
        (root / 'private').mkdir()
        (root / 'private' / 'secret.txt').write_text('not a directory listing')
        (root / 'play').mkdir()
        (root / 'play' / 'index.html').write_text('<h1>Playground</h1>')
        self.server = ThreadingHTTPServer(('127.0.0.1', 0), make_handler(root, 'http://127.0.0.1:18648'))
        self.thread = threading.Thread(target=self.server.serve_forever)
        self.thread.start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()
        self.directory.cleanup()

    def request(self, path, method='GET'):
        connection = http.client.HTTPConnection(*self.server.server_address)
        connection.request(method, path)
        response = connection.getresponse()
        result = response.status, dict(response.getheaders()), response.read().decode()
        connection.close()
        return result

    def test_article_wins_over_child_directory(self):
        status, headers, body = self.request('/physics')
        self.assertEqual(status, 200)
        self.assertIn('<h1>Physics</h1>', body)
        status, headers, body = self.request('/physics/?topic=units')
        self.assertEqual(status, 308)
        self.assertEqual(headers['Location'], '/physics?topic=units')
        self.assertEqual(body, '')
        self.assertEqual(self.request('/physics/units')[0], 200)
        self.assertEqual(self.request('/physics/', 'HEAD')[0], 308)

    def test_directory_indexes_only(self):
        self.assertEqual(self.request('/play/')[0], 200)
        status, headers, body = self.request('/private/')
        self.assertEqual(status, 404)
        self.assertNotIn('secret.txt', body)
        self.assertNotIn('Directory listing', body)

    def test_policy_allows_worklet_without_file_or_external_access(self):
        status, headers, body = self.request('/physics')
        policy = headers['Content-Security-Policy']
        self.assertIn("script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' blob:;", policy)
        self.assertIn("connect-src 'self';", policy)
        self.assertNotIn('file:', policy)
        self.assertNotIn("'unsafe-eval'", policy)
        self.assertNotIn('*', policy)


if __name__ == '__main__':
    unittest.main()
