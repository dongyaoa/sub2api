#!/usr/bin/env python3
"""Create a stateless Google replicated voice and synthesize speech via Interactions.

Official reference: https://aistudio.google.com/docs/voice-replication
This calls Google directly with the user's Google project key, not Sub2API.
"""

from __future__ import annotations

import argparse
import base64
import binascii
import getpass
import json
import os
from pathlib import Path
import sys

from test_audio_speech import SpeechClientError, configure_console, post_json, save_wav, validate_wav


GOOGLE_API_BASE = "https://generativelanguage.googleapis.com/v1beta"
CONSENT_TEXT = "我是此声音的拥有者并授权谷歌使用此声音创建语音合成模型"


def read_recording(path: Path, *, reference: bool) -> dict[str, str]:
    if path.stat().st_size > 16 * 1024 * 1024:
        raise SpeechClientError(f"录音超过本脚本的 16 MiB 限制：{path}")
    data = path.read_bytes()
    channels, width, rate, frames = validate_wav(data)
    if reference and not (10 <= frames / rate <= 30):
        raise SpeechClientError("参考录音应为同一位成年说话人的 10—30 秒 WAV 录音。")
    if (channels, width, rate) != (1, 2, 24000):
        print(f"录音 {path.name}：{rate} Hz / {channels} 声道 / {width * 8} bit；推荐导出为 24 kHz 单声道 16-bit PCM WAV。")
    return {"mime_type": "audio/wav", "data": base64.b64encode(data).decode("ascii")}


def replication_payload(model: str, reference: dict, consent: dict) -> dict:
    return {
        "store": False,
        "voice": {
            "model": model,
            "type": "replicated",
            "replicated": {"source_audio": reference, "consent_audio": consent},
        },
    }


def interaction_payload(model: str, text: str, voice_key: str, style: str) -> dict:
    content = {"type": "text", "text": text}
    if style:
        content["annotations"] = [{"type": "speech_metadata", "style": style}]
    return {
        "model": model,
        "input": [{"type": "user_input", "content": [content]}],
        "response_format": {"type": "audio"},
        "generation_config": {"speech_config": [{"voice": voice_key}]},
    }


def decode_json_response(data: bytes) -> dict:
    try:
        result = json.loads(data)
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise SpeechClientError("Google 返回的内容不是有效 JSON。") from exc
    if not isinstance(result, dict):
        raise SpeechClientError("Google 返回的 JSON 不是对象。")
    if result.get("error"):
        raise SpeechClientError("Google 返回错误：" + json.dumps(result["error"], ensure_ascii=False)[:3000])
    return result


def replicated_voice_key(response: dict) -> str:
    # The SDK example names its returned Voice object `replicated_voice`;
    # that variable name does not imply a JSON wrapper in the REST response.
    for voice in (response, response.get("replicated_voice"), response.get("replicatedVoice"), response.get("voice")):
        key = voice.get("key") if isinstance(voice, dict) else None
        if isinstance(key, str) and key.startswith("voicekey_") and not any(char.isspace() for char in key):
            return key
    raise SpeechClientError("创建响应的 Voice 对象缺少 key（voicekey_...）；未继续合成。")


def interaction_wav(response: dict) -> bytes:
    steps = response.get("steps")
    if not isinstance(steps, list):
        raise SpeechClientError("Interactions 响应缺少 steps，未取得音频。")
    audio = None
    for step in steps:
        if not isinstance(step, dict) or step.get("type") != "model_output":
            continue
        content = step.get("content")
        if not isinstance(content, list):
            continue
        for item in content:
            if isinstance(item, dict) and item.get("type") == "audio":
                audio = item
    if audio is None or not isinstance(audio.get("data"), str):
        raise SpeechClientError("Interactions 响应没有 model_output/content/audio.data。")
    try:
        decoded = base64.b64decode("".join(audio["data"].split()), validate=True)
    except (ValueError, binascii.Error) as exc:
        raise SpeechClientError("音频 data 不是有效 Base64。") from exc
    validate_wav(decoded)
    return decoded


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="使用 Google 项目 Key，创建录音克隆音色并通过官方 Interactions 合成。该请求不经过本站、不走本站扣费。",
        epilog="consent.wav 必须由参考录音中的本人朗读：" + CONSENT_TEXT,
    )
    parser.add_argument("--reference", required=True, type=Path, help="同一位成年说话人的 10—30 秒参考 WAV")
    parser.add_argument("--consent", required=True, type=Path, help="本人朗读授权句的 WAV")
    parser.add_argument("--model", default="gemini-3.8-flash-tts")
    parser.add_argument("--text", default="你好，这是使用我的录音创建的声音。欢迎收听今天的节目。")
    parser.add_argument("--style", default="warm and conversational")
    parser.add_argument("--output", type=Path, default=Path("cloned-voice.wav"))
    parser.add_argument("--timeout", type=float, default=180)
    args = parser.parse_args(argv)
    try:
        if not args.text.strip():
            raise SpeechClientError("朗读文本不能为空。")
        source = read_recording(args.reference, reference=True)
        consent = read_recording(args.consent, reference=False)
        key = os.environ.get("GEMINI_API_KEY", "").strip() or getpass.getpass("请输入 Google 项目的 Gemini API Key（隐藏输入）：").strip()
        if not key:
            raise SpeechClientError("Google API Key 不能为空。")
        auth = {"x-goog-api-key": key}
        print("正在直连 Google 创建录音克隆音色……")
        data, _, status = post_json(GOOGLE_API_BASE + "/voices", replication_payload(args.model, source, consent), auth, args.timeout)
        voice_key = replicated_voice_key(decode_json_response(data))
        print(f"音色创建成功（HTTP {status}）。正在使用同一 Google 项目合成音频……")
        data, _, status = post_json(GOOGLE_API_BASE + "/interactions", interaction_payload(args.model, args.text, voice_key, args.style), auth, args.timeout)
        wav_data = interaction_wav(decode_json_response(data))
        output = save_wav(args.output, wav_data)
        print(f"合成成功（HTTP {status}）\n已保存：{output}")
        print("本次直接使用 Google 的接口和项目额度；没有经过本站鉴权或扣费。")
        return 0
    except (SpeechClientError, OSError, EOFError, KeyboardInterrupt) as exc:
        print(f"录音克隆测试失败：{exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    configure_console()
    raise SystemExit(main())
