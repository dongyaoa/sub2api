"""Offline regression checks; no external API calls or API credentials are used."""

import argparse
import contextlib
import io
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock
import urllib.error
import wave

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import test_audio_speech as speech


def wav_bytes(seconds=0.02, rate=24000):
    buffer = io.BytesIO()
    with wave.open(buffer, "wb") as writer:
        writer.setnchannels(1)
        writer.setsampwidth(2)
        writer.setframerate(rate)
        writer.writeframes(b"\x01\x00" * int(rate * seconds))
    return buffer.getvalue()


class FakeResponse(io.BytesIO):
    def __init__(self, body, content_type="audio/wav", length=None):
        super().__init__(body)
        self.headers = {"Content-Type": content_type, "Content-Length": str(len(body) if length is None else length)}
        self.status = 200


class SpeechClientTests(unittest.TestCase):
    def test_url_and_utf8_request(self):
        expected = "https://proxy.example/v1/audio/speech"
        for base in ("https://proxy.example", "https://proxy.example/v1/", expected):
            self.assertEqual(expected, speech.speech_url(base))
        with self.assertRaises(speech.SpeechClientError):
            speech.speech_url("https://proxy.example/v1?key=secret")
        opener = mock.Mock()
        opener.open.return_value = FakeResponse(wav_bytes())
        with mock.patch.object(speech.urllib.request, "build_opener", return_value=opener):
            data, headers, status = speech.post_json(expected, {"input": "你好"}, {"Authorization": "Bearer test-key"}, 120)
        request = opener.open.call_args.args[0]
        self.assertEqual("POST", request.get_method())
        self.assertEqual("Bearer test-key", request.get_header("Authorization"))
        self.assertEqual({"input": "你好"}, json.loads(request.data))
        self.assertEqual(200, status)
        self.assertEqual("audio/wav", headers["content-type"])
        self.assertEqual(wav_bytes(), data)

    def test_error_and_truncated_response(self):
        opener = mock.Mock()
        opener.open.side_effect = urllib.error.HTTPError("https://proxy.example", 400, "Bad Request", {}, io.BytesIO(b'{"error":"voice unavailable"}'))
        with mock.patch.object(speech.urllib.request, "build_opener", return_value=opener):
            with self.assertRaisesRegex(speech.SpeechClientError, "HTTP 400.*voice unavailable"):
                speech.post_json("https://proxy.example", {}, {}, 1)
        opener.open.side_effect = None
        opener.open.return_value = FakeResponse(b"short", length=100)
        with mock.patch.object(speech.urllib.request, "build_opener", return_value=opener):
            with self.assertRaisesRegex(speech.SpeechClientError, "响应被截断"):
                speech.post_json("https://proxy.example", {}, {}, 1)

    def test_wav_validation_and_pcm_wrapping(self):
        wav = wav_bytes()
        self.assertEqual(wav, speech.audio_to_wav(wav, "audio/wav", "wav")[0])
        with self.assertRaises(speech.SpeechClientError):
            speech.audio_to_wav(b"<html>failure</html>", "text/html", "wav")
        with self.assertRaises(speech.SpeechClientError):
            speech.validate_wav(wav[:-2])
        pcm = b"\x01\x00" * 500
        wrapped, _ = speech.audio_to_wav(pcm, "application/octet-stream", "pcm")
        self.assertEqual((1, 2, 24000, 500), speech.validate_wav(wrapped))
        with self.assertRaises(speech.SpeechClientError):
            speech.audio_to_wav(pcm + b"\x00", "audio/pcm", "pcm")

    def test_pcm_rejects_error_documents_with_audio_content_type(self):
        for data in (b'{"error":"unavailable"}', b"<html>failure</html>", b"  <!DOCTYPE html><html></html>", b'\xef\xbb\xbf {"error": "bad key"}'):
            with self.subTest(data=data):
                with self.assertRaises(speech.SpeechClientError):
                    speech.audio_to_wav(data, "audio/pcm", "pcm")
        # A PCM sample may legitimately start with '<'; it is not an HTML tag.
        wrapped, _ = speech.audio_to_wav(b"<\x00" * 100, "audio/pcm", "pcm")
        self.assertEqual((1, 2, 24000, 100), speech.validate_wav(wrapped))

    def test_extra_parameters_and_output_protection(self):
        with tempfile.TemporaryDirectory() as directory:
            extra = Path(directory) / "extra.json"
            extra.write_text(json.dumps({"voice": "Zephyr", "style_prompt": "warm"}), encoding="utf-8-sig")
            args = argparse.Namespace(model="model", text="你好", voice="Kore", format="wav", instructions="warm", extra_json=str(extra))
            payload = speech.build_payload(args)
            self.assertEqual("Zephyr", payload["voice"])
            self.assertEqual("warm", payload["style_prompt"])
            self.assertEqual("你好", payload["input"])
            extra.write_text('{"response_format":"pcm"}', encoding="utf-8")
            with self.assertRaises(speech.SpeechClientError):
                speech.build_payload(args)
            output = Path(directory) / "speech.wav"
            output.write_bytes(wav_bytes())
            with self.assertRaises(speech.SpeechClientError):
                speech.save_wav(output, b'{"error":"bad request"}')
            self.assertEqual(wav_bytes(), output.read_bytes())

    def test_main_saves_only_valid_audio(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "speech.wav"
            with mock.patch.dict(speech.os.environ, {"SUB2API_API_KEY": "test-key"}), contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                with mock.patch.object(speech, "post_json", return_value=(wav_bytes(), {"content-type": "audio/wav"}, 200)) as post:
                    self.assertEqual(0, speech.main(["--output", str(output), "--instructions", "slow"]))
                    self.assertEqual("slow", post.call_args.args[1]["instructions"])
                with mock.patch.object(speech, "post_json", return_value=(b"<html>page</html>", {"content-type": "text/html"}, 200)):
                    self.assertEqual(1, speech.main(["--output", str(output)]))
            self.assertEqual(wav_bytes(), output.read_bytes())


if __name__ == "__main__":
    unittest.main()
