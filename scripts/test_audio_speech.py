#!/usr/bin/env python3
"""Test Sub2API's OpenAI-compatible speech endpoint using Python's standard library."""

from __future__ import annotations

import argparse
import getpass
import http.client
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
import wave


class SpeechClientError(Exception):
    """An actionable API or audio validation error."""


def configure_console() -> None:
    # Keep Chinese diagnostics readable when Windows redirects Python output.
    for stream in (sys.stdout, sys.stderr):
        if hasattr(stream, "reconfigure"):
            stream.reconfigure(encoding="utf-8", errors="replace")


class NoRedirectHandler(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        # Never forward the API key to a host selected by a redirect response.
        return None


def speech_url(base_url: str) -> str:
    base_url = base_url.strip().rstrip("/")
    parsed = urllib.parse.urlsplit(base_url)
    if (
        parsed.scheme not in ("http", "https")
        or not parsed.hostname
        or parsed.username is not None
        or parsed.password is not None
        or parsed.query
        or parsed.fragment
    ):
        raise SpeechClientError("Base URL 必须是有效的 HTTP(S) 地址，不能包含账号密码、查询参数或片段。")
    if parsed.path.endswith("/audio/speech"):
        return base_url
    if parsed.path.endswith("/v1"):
        return base_url + "/audio/speech"
    return base_url + "/v1/audio/speech"


def post_json(url: str, payload: dict, auth_headers: dict[str, str], timeout: float) -> tuple[bytes, dict[str, str], int]:
    body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=body,
        method="POST",
        headers={"Content-Type": "application/json", **auth_headers},
    )
    opener = urllib.request.build_opener(NoRedirectHandler())
    try:
        with opener.open(request, timeout=timeout) as response:
            chunks = []
            size = 0
            while chunk := response.read(64 * 1024):
                size += len(chunk)
                if size > 128 * 1024 * 1024:
                    raise SpeechClientError("响应超过 128 MiB，本地测试已停止保存。")
                chunks.append(chunk)
            headers = {k.lower(): v for k, v in response.headers.items()}
            expected_length = headers.get("content-length", "")
            if expected_length.isdigit() and int(expected_length) != size:
                raise SpeechClientError(f"响应被截断：声明 {expected_length} 字节，实际收到 {size} 字节。")
            return b"".join(chunks), headers, response.status
    except urllib.error.HTTPError as exc:
        detail = exc.read(64 * 1024).decode("utf-8", errors="replace")
        raise SpeechClientError(f"HTTP {exc.code}: {detail[:4000] or exc.reason}") from exc
    except (urllib.error.URLError, TimeoutError, OSError, http.client.HTTPException) as exc:
        raise SpeechClientError(f"请求未完成：{exc}") from exc


def validate_wav(data: bytes) -> tuple[int, int, int, int]:
    if len(data) < 12 or data[:4] != b"RIFF" or data[8:12] != b"WAVE":
        raise SpeechClientError("返回内容不是 WAV 文件，未写入输出文件。")
    try:
        with wave.open(io.BytesIO(data), "rb") as reader:
            channels, width, rate, frames = (
                reader.getnchannels(), reader.getsampwidth(), reader.getframerate(), reader.getnframes()
            )
            audio = reader.readframes(frames)
            if channels <= 0 or width <= 0 or rate <= 0 or frames <= 0 or len(audio) != frames * channels * width:
                raise SpeechClientError("WAV 音频为空或被截断，未写入输出文件。")
            return channels, width, rate, frames
    except (wave.Error, EOFError) as exc:
        raise SpeechClientError(f"WAV 格式校验失败：{exc}") from exc


def audio_to_wav(data: bytes, content_type: str, requested_format: str) -> tuple[bytes, str]:
    media_type = content_type.split(";", 1)[0].strip().lower()
    if not (media_type.startswith("audio/") or media_type == "application/octet-stream"):
        detail = data[:1000].decode("utf-8", errors="replace")
        raise SpeechClientError(f"收到非音频响应（{content_type or '无 Content-Type'}）：{detail}")
    probe = data.removeprefix(b"\xef\xbb\xbf").lstrip()
    if probe[:256].lower().startswith((b"<!doctype", b"<html", b"<head", b"<body", b"<script", b"<?xml", b"<error")):
        raise SpeechClientError("上游把 HTML/XML 页面标记成了音频，未写入输出文件。")
    if probe.startswith((b"{", b"[")):
        try:
            json.loads(probe)
        except (UnicodeDecodeError, json.JSONDecodeError):
            pass
        else:
            raise SpeechClientError("上游把 JSON 响应标记成了音频，未写入输出文件。")
    if data[:4] == b"RIFF":
        channels, width, rate, frames = validate_wav(data)
        return data, f"WAV，{rate} Hz，{channels} 声道，{width * 8} bit，{frames / rate:.2f} 秒"
    if requested_format != "pcm":
        raise SpeechClientError("请求 WAV，但返回内容没有 WAV 文件头；未保存。")
    if not data or len(data) % 2:
        raise SpeechClientError("24 kHz 单声道 16-bit PCM 数据为空或字节数不完整；未保存。")
    output = io.BytesIO()
    with wave.open(output, "wb") as writer:
        writer.setnchannels(1)
        writer.setsampwidth(2)
        writer.setframerate(24000)
        writer.writeframes(data)
    return output.getvalue(), f"PCM 已包装为 WAV，24000 Hz，单声道，16 bit，{len(data) / 48000:.2f} 秒"


def save_wav(path: Path, data: bytes) -> Path:
    # Validate before touching an existing output, then replace it atomically.
    validate_wav(data)
    path = path.expanduser().resolve()
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(dir=path.parent, prefix=".speech-", suffix=".tmp", delete=False) as stream:
            temporary = Path(stream.name)
            stream.write(data)
        os.replace(temporary, path)
    finally:
        if temporary is not None and temporary.exists():
            temporary.unlink()
    return path


def build_payload(args: argparse.Namespace) -> dict:
    if not args.text.strip():
        raise SpeechClientError("朗读文本不能为空。")
    payload = {"model": args.model, "input": args.text, "voice": args.voice, "response_format": args.format}
    if args.instructions:
        payload["instructions"] = args.instructions
    if args.extra_json:
        try:
            extra = json.loads(Path(args.extra_json).read_text(encoding="utf-8-sig"))
        except (OSError, json.JSONDecodeError) as exc:
            raise SpeechClientError(f"无法读取额外参数 JSON：{exc}") from exc
        if not isinstance(extra, dict):
            raise SpeechClientError("额外参数 JSON 必须是对象。")
        protected = {"model", "input", "response_format"}.intersection(extra)
        if protected:
            raise SpeechClientError("请通过命令行设置这些参数，不能在 extra-json 覆写：" + ", ".join(sorted(protected)))
        payload.update(extra)
    return payload


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="调用本站 /v1/audio/speech 并保存有效 WAV；预设音色选择不等于录音克隆。")
    parser.add_argument("--base-url", default=os.environ.get("SUB2API_BASE_URL", "https://api.zicc.cc/v1"))
    parser.add_argument("--model", default="gemini-3.8-flash-tts")
    parser.add_argument("--voice", default="Kore")
    parser.add_argument("--text", default="你好，这是一段语音接口测试。现在可以通过我的站点生成语音了。")
    parser.add_argument("--instructions", default="", help="自然语言语气或风格指令")
    parser.add_argument("--output", default="speech.wav")
    parser.add_argument("--format", choices=("wav", "pcm"), default="wav", help="请求的音频格式；PCM 会包装成 24 kHz 单声道 16-bit WAV")
    parser.add_argument("--extra-json", help="UTF-8 JSON 文件，透传上游已支持的额外字段，例如 speakers")
    parser.add_argument("--timeout", type=float, default=180, help="请求超时秒数，默认 180")
    args = parser.parse_args(argv)
    try:
        url = speech_url(args.base_url)
        payload = build_payload(args)
        key = os.environ.get("SUB2API_API_KEY", "").strip() or getpass.getpass("请输入本站 API Key（隐藏输入）：").strip()
        if not key:
            raise SpeechClientError("API Key 不能为空。")
        print(f"请求：{url}\n模型：{args.model}")
        data, headers, status = post_json(url, payload, {"Authorization": "Bearer " + key}, args.timeout)
        content_type = headers.get("content-type", "")
        print(f"HTTP {status}\nContent-Type: {content_type}")
        wav_data, summary = audio_to_wav(data, content_type, args.format)
        output = save_wav(Path(args.output), wav_data)
        print(f"{summary}\n已保存：{output}")
        if headers.get("x-request-id"):
            print("请求 ID：" + headers["x-request-id"])
        return 0
    except (SpeechClientError, OSError, EOFError, KeyboardInterrupt) as exc:
        print(f"测试失败：{exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    configure_console()
    raise SystemExit(main())
