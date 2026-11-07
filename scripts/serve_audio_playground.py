#!/usr/bin/env python3
"""Serve the audio playground and a fixed-target, same-origin local proxy."""

from __future__ import annotations

import argparse
import hmac
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import secrets
import urllib.error
import urllib.request

from test_audio_speech import NoRedirectHandler, configure_console, speech_url


PAGE_PATH = Path(__file__).with_name("audio-playground.html")
MAX_REQUEST_BYTES = 28 * 1024 * 1024
MAX_RESPONSE_BYTES = 128 * 1024 * 1024
ALLOWED_PATHS = {
    "/proxy/v1/audio/speech": "/audio/speech",
    "/proxy/v1/audio/voices": "/audio/voices",
}


class AudioPlaygroundServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, address, base_url: str, timeout: float = 180):
        # Resolve the destination once; browser requests cannot choose another host.
        self.base_url = speech_url(base_url).removesuffix("/audio/speech")
        self.upstream_timeout = timeout
        self.session_nonce = secrets.token_urlsafe(32)
        self.upstream = urllib.request.build_opener(NoRedirectHandler())
        super().__init__(address, AudioPlaygroundHandler)


class AudioPlaygroundHandler(BaseHTTPRequestHandler):
    server_version = "AudioPlayground"

    def log_message(self, format, *args):
        # Do not write requests, credentials, recordings, or response bodies to logs.
        pass

    def _same_origin(self, *, post: bool = False) -> bool:
        port = self.server.server_port
        host = self.headers.get("Host", "")
        if host not in {f"127.0.0.1:{port}", f"localhost:{port}"}:
            self._error(403, "不接受此 Host。请使用启动时显示的本地地址。")
            return False
        expected_origin = "http://" + host
        origin = self.headers.get("Origin")
        if (post and origin != expected_origin) or (origin is not None and origin != expected_origin):
            self._error(403, "只接受本地测试页的同源请求。")
            return False
        fetch_site = self.headers.get("Sec-Fetch-Site", "")
        public_document_navigation = (
            not post
            and self.path in {"/", "/audio-playground.html"}
            and self.headers.get("Sec-Fetch-Mode") == "navigate"
            and self.headers.get("Sec-Fetch-Dest") == "document"
        )
        if fetch_site and fetch_site not in {"same-origin", "none"} and not public_document_navigation:
            self._error(403, "不接受跨站请求。")
            return False
        if post and not hmac.compare_digest(self.headers.get("X-Audio-Session", ""), self.server.session_nonce):
            self._error(403, "本地代理会话无效，请刷新测试页。")
            return False
        return True

    def _respond(self, status: int, body: bytes, content_type: str, headers: dict | None = None):
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.send_header("X-Content-Type-Options", "nosniff")
        self.send_header("X-Frame-Options", "DENY")
        self.send_header("Referrer-Policy", "no-referrer")
        self.send_header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; media-src 'self' blob:; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
        for name, value in (headers or {}).items():
            if name.lower() in {"x-request-id", "request-id", "retry-after"}:
                self.send_header(name, value)
        self.end_headers()
        try:
            self.wfile.write(body)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def _error(self, status: int, message: str):
        body = json.dumps({"error": {"type": "local_proxy_error", "message": message}}, ensure_ascii=False).encode("utf-8")
        self._respond(status, body, "application/json; charset=utf-8")

    def do_GET(self):
        if not self._same_origin():
            return
        if self.path in {"/", "/audio-playground.html"}:
            self._respond(200, PAGE_PATH.read_bytes(), "text/html; charset=utf-8")
        elif self.path == "/config":
            body = json.dumps({"base_url": self.server.base_url, "session_nonce": self.server.session_nonce}).encode("utf-8")
            self._respond(200, body, "application/json; charset=utf-8")
        else:
            self._error(404, "未找到本地测试页资源。")

    def do_POST(self):
        if not self._same_origin(post=True):
            return
        suffix = ALLOWED_PATHS.get(self.path)
        if suffix is None:
            self._error(404, "本地代理仅支持语音合成和声音创建接口。")
            return
        if self.headers.get("Transfer-Encoding"):
            self._error(400, "不接受分块上传，请使用测试页发送 JSON。")
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            length = 0
        if length <= 0 or length > MAX_REQUEST_BYTES:
            self._error(413, "请求体为空或超过 28 MiB。")
            return
        if self.headers.get_content_type() != "application/json":
            self._error(415, "请求必须使用 application/json。")
            return
        authorization = self.headers.get("Authorization", "")
        if not authorization.startswith("Bearer ") or not authorization[7:].strip() or "\r" in authorization or "\n" in authorization:
            self._error(401, "请填写本站 API Key。")
            return
        self.connection.settimeout(30)
        try:
            body = self.rfile.read(length)
            if len(body) != length:
                self._error(400, "请求体不完整。")
                return
            if not isinstance(json.loads(body), dict):
                self._error(400, "请求 JSON 必须是对象。")
                return
        except (ValueError, UnicodeDecodeError, TimeoutError, OSError):
            self._error(400, "无法读取有效的 JSON 请求。")
            return
        request = urllib.request.Request(
            self.server.base_url + suffix,
            data=body,
            method="POST",
            headers={"Content-Type": "application/json", "Authorization": authorization, "Accept": "application/json, audio/*"},
        )
        try:
            try:
                response = self.server.upstream.open(request, timeout=self.server.upstream_timeout)
            except urllib.error.HTTPError as exc:
                response = exc
            with response:
                data = response.read(MAX_RESPONSE_BYTES + 1)
                if len(data) > MAX_RESPONSE_BYTES:
                    self._error(502, "上游响应超过本地测试页的 128 MiB 限制。")
                    return
                expected = response.headers.get("Content-Length", "")
                if expected.isdigit() and int(expected) != len(data):
                    self._error(502, "上游响应被截断，未返回音频。")
                    return
                status = response.code if isinstance(response, urllib.error.HTTPError) else response.status
                self._respond(status, data, response.headers.get("Content-Type", "application/octet-stream"), dict(response.headers.items()))
        except (TimeoutError, OSError, urllib.error.URLError, http.client.HTTPException) as exc:
            # Exceptions carry transport details but not the Authorization header.
            self._error(502, f"无法完成上游请求：{exc}")


def main(argv=None):
    parser = argparse.ArgumentParser(description="启动本地语音测试页；只监听 127.0.0.1，固定代理到指定站点，不改变服务端 CORS。")
    parser.add_argument("--base-url", default="https://api.zicc.cc/v1")
    parser.add_argument("--port", type=int, default=8765)
    parser.add_argument("--timeout", type=float, default=180)
    args = parser.parse_args(argv)
    if not 0 <= args.port <= 65535 or args.timeout <= 0:
        parser.error("port 应为 0—65535，timeout 应大于 0。")
    server = AudioPlaygroundServer(("127.0.0.1", args.port), args.base_url, args.timeout)
    print(f"测试页：http://127.0.0.1:{server.server_port}\n固定目标：{server.base_url}\nKey 和录音不会写入本地日志。按 Ctrl+C 停止。", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    configure_console()
    main()
