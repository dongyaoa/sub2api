#!/usr/bin/env python3
"""Sub2API 视频任务示例（Python 3.10+，仅使用标准库）。

环境变量：SUB2API_BASE_URL=https://api.example.com、SUB2API_API_KEY=你的平台密钥。
创建并下载：python video_client.py --payload payload.json --output video.mp4
恢复任务：  python video_client.py --task-id 已有任务ID --output video.mp4

只提交一次 POST。提交超时或断线时，请先查询任务历史，勿直接再次创建。
"""

import argparse
import json
import os
from pathlib import Path
import sys
import tempfile
import time
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urlsplit, urlunsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener


COMPLETE = {"done", "completed", "succeeded", "success"}
FAILED = {"failed", "cancelled", "canceled", "expired", "error"}
PENDING = {"", "pending", "queued", "processing", "in_progress", "running", "in_queue"}
RETRYABLE = {429, 500, 502, 503, 504}
MAX_VIDEO_BYTES = 512 * 1024 * 1024


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        # 不将平台密钥转发至重定向地址。
        return None


def api_root(value):
    parsed = urlsplit(value.strip())
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise ValueError("SUB2API_BASE_URL 必须是 HTTP(S) 站点地址")
    if parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ValueError("SUB2API_BASE_URL 不能包含用户信息、查询参数或片段")
    path = parsed.path.rstrip("/")
    if path.endswith("/v1"):
        path = path[:-3]
    return urlunsplit((parsed.scheme, parsed.netloc, path + "/v1", "", ""))


def task_id(payload):
    return str(payload.get("request_id") or payload.get("id") or payload.get("task_id") or "").strip()


def error_text(raw):
    try:
        data = json.loads(raw)
        error = data.get("error", data)
        if isinstance(error, dict):
            return str(error.get("message") or error.get("code") or error)
        return str(error)
    except (ValueError, AttributeError):
        return raw.decode("utf-8", errors="replace")[:500]


class VideoClient:
    def __init__(self, base, key, timeout):
        self.root = api_root(base)
        self.key = key
        self.deadline = time.monotonic() + timeout
        self.opener = build_opener(NoRedirect())

    def remaining(self):
        remaining = self.deadline - time.monotonic()
        if remaining <= 0:
            raise TimeoutError("等待超时；可用已打印的任务 ID 和 --task-id 恢复查询，不要重新提交")
        return remaining

    def pause(self, seconds):
        time.sleep(min(seconds, self.remaining()))
        self.remaining()

    def request(self, method, path, payload=None, accept="application/json"):
        data = None if payload is None else json.dumps(payload, ensure_ascii=False).encode("utf-8")
        headers = {"Authorization": "Bearer " + self.key, "Accept": accept}
        if data is not None:
            headers["Content-Type"] = "application/json"
        attempts = 3 if method == "GET" else 1
        request_timeout = 600 if path.endswith("/content") else 120
        for attempt in range(attempts):
            request = Request(self.root + path, data=data, headers=headers, method=method)
            try:
                return self.opener.open(request, timeout=min(request_timeout, self.remaining()))
            except HTTPError as exc:
                body = exc.read(8192)
                retry_after = exc.headers.get("Retry-After", "")
                exc.close()
                if exc.code in RETRYABLE and attempt + 1 < attempts:
                    delay = min(30, max(1, int(retry_after))) if retry_after.isdigit() else 2 ** attempt
                    self.pause(delay)
                    continue
                raise RuntimeError(f"HTTP {exc.code}: {error_text(body)}") from None
            except (URLError, TimeoutError, ConnectionError) as exc:
                if attempt + 1 < attempts:
                    self.pause(2 ** attempt)
                    continue
                hint = "；创建未自动重试，请先检查任务历史" if method == "POST" else ""
                raise RuntimeError(f"请求失败：{exc}{hint}") from None

    def json(self, method, path, payload=None):
        with self.request(method, path, payload) as response:
            result = json.load(response)
        if not isinstance(result, dict):
            raise RuntimeError("接口返回了非对象 JSON")
        return result

    def download(self, identifier, output):
        target = Path(output).expanduser().resolve()
        target.parent.mkdir(parents=True, exist_ok=True)
        temporary = None
        try:
            with self.request("GET", "/videos/" + quote(identifier, safe="") + "/content",
                              accept="video/*,application/octet-stream") as response:
                content_type = response.headers.get("Content-Type", "").split(";")[0].lower()
                content_length = response.headers.get("Content-Length", "")
                if not (content_type.startswith("video/") or content_type == "application/octet-stream"):
                    raise RuntimeError("内容接口未返回视频：" + error_text(response.read(8192)))
                first = response.read(64)
                if not ((len(first) >= 12 and first[4:8] == b"ftyp") or first.startswith(b"\x1aE\xdf\xa3")):
                    raise RuntimeError("内容不是预期的 MP4/WebM，未写入输出文件")
                with tempfile.NamedTemporaryFile(prefix=target.name + ".", suffix=".part", dir=target.parent, delete=False) as file:
                    temporary = Path(file.name)
                    file.write(first)
                    count = len(first)
                    while True:
                        self.remaining()
                        chunk = response.read(1024 * 1024)
                        if not chunk:
                            break
                        count += len(chunk)
                        if count > MAX_VIDEO_BYTES:
                            raise RuntimeError("视频超过示例客户端的 512 MiB 下载限制")
                        file.write(chunk)
                if content_length.isdigit() and count != int(content_length):
                    raise RuntimeError("下载内容不完整，未替换输出文件；可用原任务 ID 重试")
                self.remaining()
            os.replace(temporary, target)
            temporary = None
            print(f"已保存：{target}（{count} 字节）")
        finally:
            if temporary is not None:
                temporary.unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--payload", help="JSON 请求体文件；创建一个新任务")
    source.add_argument("--task-id", help="查询并下载已有任务，不创建新任务")
    parser.add_argument("--output", default="video.mp4")
    parser.add_argument("--timeout", type=int, default=900, help="本次调用总等待秒数，默认 900")
    args = parser.parse_args()
    if args.timeout <= 0:
        parser.error("--timeout 必须大于 0")
    base = os.environ.get("SUB2API_BASE_URL", "").strip()
    key = os.environ.get("SUB2API_API_KEY", "").strip()
    if not base or not key:
        parser.error("请设置 SUB2API_BASE_URL 和 SUB2API_API_KEY")
    client = VideoClient(base, key, args.timeout)
    try:
        identifier, current = args.task_id, None
        if args.payload:
            with open(args.payload, encoding="utf-8-sig") as file:
                payload = json.load(file)
            if not isinstance(payload, dict) or not payload.get("model") or not payload.get("prompt"):
                raise ValueError("请求体必须是包含 model 和 prompt 的 JSON 对象")
            current = client.json("POST", "/videos", payload)
            identifier = task_id(current)
            if not identifier:
                raise RuntimeError("创建响应没有任务 ID；请先检查历史，勿立即重新创建")
        identifier = identifier.strip()
        if not identifier:
            raise ValueError("任务 ID 不能为空")
        print("任务 ID：" + identifier, flush=True)
        last_status = None
        while True:
            client.remaining()
            if current is None:
                current = client.json("GET", "/videos/" + quote(identifier, safe=""))
            status = str(current.get("status") or "").strip().lower()
            if status != last_status:
                print("状态：" + (status or "已受理，等待查询"), flush=True)
                last_status = status
            if status in FAILED:
                raise RuntimeError("任务失败：" + str(current.get("error") or status))
            if status in COMPLETE:
                client.download(identifier, args.output)
                return
            if status not in PENDING:
                raise RuntimeError("未知任务状态：" + status + "；保留任务 ID 后联系服务方")
            client.pause(5)
            current = None
    except (RuntimeError, ValueError, OSError) as exc:
        print(str(exc).replace(key, "[REDACTED]"), file=sys.stderr)
        raise SystemExit(1) from None


if __name__ == "__main__":
    main()
