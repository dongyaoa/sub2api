"""Offline local-proxy checks: all upstream responses are mocked."""

import io
import json
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import threading
import unittest
from unittest import mock
import urllib.error
import urllib.request

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import serve_audio_playground as playground


class UpstreamResponse(io.BytesIO):
    def __init__(self, body, content_type="audio/wav"):
        super().__init__(body)
        self.status = 200
        self.headers = {"Content-Type": content_type, "Content-Length": str(len(body)), "X-Request-Id": "mock-request"}


class AudioPlaygroundProxyTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = playground.AudioPlaygroundServer(("127.0.0.1", 0), "https://fixed.example/prefix/v1")
        cls.worker = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.worker.start()
        cls.origin = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.worker.join(timeout=2)

    def request(self, path, *, body=None, headers=None):
        request = urllib.request.Request(self.origin + path, data=body, headers=headers or {}, method="POST" if body is not None else "GET")
        try:
            response = urllib.request.urlopen(request, timeout=3)
        except urllib.error.HTTPError as exc:
            response = exc
        with response:
            return response.status, response.headers, response.read()

    def post_headers(self):
        return {"Origin": self.origin, "X-Audio-Session": self.server.session_nonce, "Authorization": "Bearer local-test-key", "Content-Type": "application/json"}

    def test_page_and_config_no_cache_or_external_scripts(self):
        status, headers, page = self.request("/")
        self.assertEqual(200, status)
        self.assertEqual("no-store", headers["Cache-Control"])
        self.assertEqual("DENY", headers["X-Frame-Options"])
        self.assertIn("frame-ancestors 'none'", headers["Content-Security-Policy"])
        self.assertIn(b"source_audio", page)
        self.assertNotIn(b"localStorage", page)
        self.assertNotIn(b"innerHTML", page)
        self.assertNotIn(b"<script src=", page)
        status, _, body = self.request("/config")
        self.assertEqual(200, status)
        config = json.loads(body)
        self.assertEqual("https://fixed.example/prefix/v1", config["base_url"])
        self.assertEqual(self.server.session_nonce, config["session_nonce"])
        self.assertNotIn("key", config)

    def test_rejects_bad_host_foreign_origin_and_missing_nonce(self):
        for headers in ({"Host": "attacker.example"}, {"Origin": "https://attacker.example"}, {"Sec-Fetch-Site": "cross-site"}):
            with self.subTest(headers=headers):
                self.assertEqual(403, self.request("/config", headers=headers)[0])
        for changed in ({"Origin": "https://attacker.example"}, {"Origin": "null"}, {"X-Audio-Session": "invalid"}, {"Sec-Fetch-Site": "cross-site"}):
            with self.subTest(changed=changed):
                headers = self.post_headers()
                headers.update(changed)
                with mock.patch.object(self.server.upstream, "open") as upstream:
                    self.assertEqual(403, self.request("/proxy/v1/audio/speech", body=b"{}", headers=headers)[0])
                    upstream.assert_not_called()
        headers = self.post_headers()
        del headers["X-Audio-Session"]
        self.assertEqual(403, self.request("/proxy/v1/audio/speech", body=b"{}", headers=headers)[0])

    def test_allows_public_page_navigation_but_not_cross_site_config_reads(self):
        navigation = {"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document"}
        self.assertEqual(200, self.request("/", headers=navigation)[0])
        self.assertEqual(403, self.request("/config", headers=navigation)[0])

    def test_only_fixed_allowlisted_paths_are_forwarded(self):
        for path, suffix, content_type in (("/proxy/v1/audio/speech", "/audio/speech", "audio/wav"), ("/proxy/v1/audio/voices", "/audio/voices", "application/json")):
            with self.subTest(path=path):
                body = b'{"model":"test-model","input":"test"}'
                with mock.patch.object(self.server.upstream, "open", return_value=UpstreamResponse(b"test-response", content_type)) as upstream:
                    status, headers, result = self.request(path, body=body, headers=self.post_headers())
                self.assertEqual(200, status)
                self.assertEqual(b"test-response", result)
                self.assertEqual("mock-request", headers["X-Request-Id"])
                request = upstream.call_args.args[0]
                self.assertEqual("https://fixed.example/prefix/v1" + suffix, request.full_url)
                self.assertEqual(body, request.data)
                self.assertEqual("Bearer local-test-key", request.get_header("Authorization"))
                self.assertIsNone(request.get_header("X-audio-session"))
                self.assertIsNone(request.get_header("Origin"))
        for path in ("/proxy/https://evil.example", "/proxy/v1/audio/speech?url=https://evil.example", "/proxy/v1/chat/completions"):
            with mock.patch.object(self.server.upstream, "open") as upstream:
                self.assertEqual(404, self.request(path, body=b"{}", headers=self.post_headers())[0])
                upstream.assert_not_called()

    def test_json_authorization_and_size_limits(self):
        headers = self.post_headers()
        headers["Content-Type"] = "text/plain"
        self.assertEqual(415, self.request("/proxy/v1/audio/speech", body=b"{}", headers=headers)[0])
        headers = self.post_headers()
        del headers["Authorization"]
        self.assertEqual(401, self.request("/proxy/v1/audio/speech", body=b"{}", headers=headers)[0])
        self.assertEqual(400, self.request("/proxy/v1/audio/speech", body=b"[]", headers=self.post_headers())[0])
        with mock.patch.object(playground, "MAX_REQUEST_BYTES", 4):
            self.assertEqual(413, self.request("/proxy/v1/audio/speech", body=b'{"a":1}', headers=self.post_headers())[0])

    def test_upstream_errors_and_redirects_not_followed(self):
        response = urllib.error.HTTPError("https://fixed.example", 400, "Bad Request", {"Content-Type": "application/json"}, io.BytesIO(b'{"error":{"message":"unsupported voice"}}'))
        with mock.patch.object(self.server.upstream, "open", side_effect=response):
            status, _, body = self.request("/proxy/v1/audio/voices", body=b"{}", headers=self.post_headers())
        self.assertEqual(400, status)
        self.assertIn(b"unsupported voice", body)
        redirect_handler = next(handler for handler in self.server.upstream.handlers if isinstance(handler, playground.NoRedirectHandler))
        self.assertIsNone(redirect_handler.redirect_request(None, None, 302, "redirect", {}, "https://evil.example"))
        redirect = urllib.error.HTTPError("https://fixed.example", 302, "Found", {"Location": "https://evil.example", "Content-Type": "text/plain"}, io.BytesIO(b"redirect rejected"))
        with mock.patch.object(self.server.upstream, "open", side_effect=redirect):
            status, headers, _ = self.request("/proxy/v1/audio/speech", body=b"{}", headers=self.post_headers())
        self.assertEqual(302, status)
        self.assertNotIn("Location", headers)


@unittest.skipUnless(shutil.which("node"), "Node.js is only needed for the offline JavaScript checks")
class AudioPlaygroundJavaScriptTests(unittest.TestCase):
    def test_script_syntax_and_audio_validation(self):
        page = playground.PAGE_PATH.read_text(encoding="utf-8")
        scripts = re.findall(r"<script>([\s\S]*?)</script>", page)
        self.assertEqual(1, len(scripts))
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "playground.js"
            path.write_text(scripts[0], encoding="utf-8")
            result = subprocess.run(["node", "--check", str(path)], capture_output=True, text=True, encoding="utf-8", errors="replace")
            self.assertEqual(0, result.returncode, result.stderr)
            runner = Path(directory) / "check.cjs"
            runner.write_text(r'''
const fs = require("node:fs"), vm = require("node:vm"), assert = require("node:assert/strict");
const elements = new Map();
const element = id => { if (!elements.has(id)) elements.set(id, {value: "", textContent: "", hidden: false, dataset: {}, classList: {remove() {}, add() {}}, addEventListener() {}, dispatchEvent() {}, setAttribute() {}, pause() {}}); return elements.get(id); };
const context = vm.createContext({document: {getElementById: element}, window: {addEventListener() {}}, location: {protocol: "file:"}, Event, TextDecoder, TextEncoder, Blob, URL, setTimeout, clearTimeout, console, performance});
vm.runInContext(fs.readFileSync(process.argv[2], "utf8"), context);
assert.equal(vm.runInContext('normalizeBase("https://example.test/v1/audio/speech")', context), "https://example.test/v1");
assert.equal(vm.runInContext('normalizeBase("https://example.test")', context), "https://example.test/v1");
assert.throws(() => vm.runInContext('normalizeBase("https://user:password@example.test/v1")', context));
assert.equal(vm.runInContext('encodeWav(new Float32Array(24000)).byteLength', context), 48044);
vm.runInContext('validateWav(encodeWav(new Float32Array(24000)))', context);
assert.throws(() => vm.runInContext('validateWav(encodeWav(new Float32Array(24000)).slice(0, 45))', context));
assert.throws(() => vm.runInContext('validAudio(new TextEncoder().encode("<html>failed</html>").buffer, "audio/pcm", "pcm")', context), /网页/);
assert.throws(() => vm.runInContext('validAudio(new TextEncoder().encode(JSON.stringify({error:"failed"})).buffer, "audio/pcm", "pcm")', context), /JSON/);
assert.throws(() => vm.runInContext('validAudio(new Uint8Array([1]).buffer, "audio/pcm", "pcm")', context), /PCM/);
assert.equal(vm.runInContext('validAudio(new Uint8Array(48000).buffer, "audio/pcm", "pcm").size', context), 48044);
''', encoding="utf-8")
            result = subprocess.run(["node", str(runner), str(path)], capture_output=True, text=True, encoding="utf-8", errors="replace")
            self.assertEqual(0, result.returncode, result.stderr)


if __name__ == "__main__":
    unittest.main()
