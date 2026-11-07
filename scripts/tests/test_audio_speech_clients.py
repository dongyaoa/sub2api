"""Offline regression checks; no external API calls or API credentials are used."""

import argparse
import base64
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
import test_google_voice_replication as replication


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
            extra.write_text(json.dumps({"voice": {"id": "provider-voice"}, "speakers": [{"name": "Speaker1", "voice": "Kore"}]}), encoding="utf-8-sig")
            args = argparse.Namespace(model="model", text="你好", voice="Kore", format="wav", instructions="warm", extra_json=str(extra))
            payload = speech.build_payload(args)
            self.assertEqual({"id": "provider-voice"}, payload["voice"])
            self.assertIn("speakers", payload)
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


class GoogleReplicationTests(unittest.TestCase):
    def test_official_flow_and_project_key(self):
        wav = wav_bytes()
        response = {"steps": [{"type": "model_output", "content": [{"type": "audio", "data": base64.b64encode(wav).decode()}]}]}
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "source.wav"
            consent = Path(directory) / "consent.wav"
            output = Path(directory) / "cloned.wav"
            source.write_bytes(wav_bytes(10))
            consent.write_bytes(wav_bytes(2))
            mocked_responses = [
                (b'{"key":"voicekey_test"}', {"content-type": "application/json"}, 200),
                (json.dumps(response).encode(), {"content-type": "application/json"}, 200),
            ]
            with mock.patch.dict(replication.os.environ, {"GEMINI_API_KEY": "google-project-key"}), mock.patch.object(replication, "post_json", side_effect=mocked_responses) as post, contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(0, replication.main(["--reference", str(source), "--consent", str(consent), "--output", str(output)]))
            first, second = post.call_args_list
            self.assertEqual(replication.GOOGLE_API_BASE + "/voices", first.args[0])
            self.assertFalse(first.args[1]["store"])
            self.assertEqual("replicated", first.args[1]["voice"]["type"])
            self.assertEqual(source.read_bytes(), base64.b64decode(first.args[1]["voice"]["replicated"]["source_audio"]["data"]))
            self.assertEqual(consent.read_bytes(), base64.b64decode(first.args[1]["voice"]["replicated"]["consent_audio"]["data"]))
            self.assertEqual({"x-goog-api-key": "google-project-key"}, first.args[2])
            self.assertEqual(first.args[2], second.args[2])
            self.assertEqual(replication.GOOGLE_API_BASE + "/interactions", second.args[0])
            self.assertEqual([{"voice": "voicekey_test"}], second.args[1]["generation_config"]["speech_config"])
            self.assertEqual("speech_metadata", second.args[1]["input"][0]["content"][0]["annotations"][0]["type"])
            self.assertEqual(wav, output.read_bytes())

    def test_voice_response_and_last_audio(self):
        for response in ({"key": "voicekey_test"}, {"replicated_voice": {"key": "voicekey_test"}}, {"replicatedVoice": {"key": "voicekey_test"}}, {"voice": {"key": "voicekey_test"}}):
            self.assertEqual("voicekey_test", replication.replicated_voice_key(response))
        self.assertEqual("voicekey_top", replication.replicated_voice_key({"key": "voicekey_top", "voice": {"key": "voicekey_nested"}}))
        with self.assertRaises(speech.SpeechClientError):
            replication.replicated_voice_key({"unrelated": {"key": "voicekey_test"}})
        first, last = wav_bytes(0.01), wav_bytes(0.03)
        content = [{"type": "audio", "data": base64.b64encode(data).decode()} for data in (first, last)]
        self.assertEqual(last, replication.interaction_wav({"steps": [{"type": "model_output", "content": content}]}))
        with self.assertRaises(speech.SpeechClientError):
            replication.interaction_wav({"steps": [{"type": "user_input", "content": content}]})
        with self.assertRaises(speech.SpeechClientError):
            replication.interaction_wav({"steps": [{"type": "model_output", "content": [{"type": "audio", "data": "invalid!"}]}]})

    def test_reference_duration_checked_before_api(self):
        with tempfile.TemporaryDirectory() as directory:
            short = Path(directory) / "short.wav"
            short.write_bytes(wav_bytes())
            with mock.patch.object(replication, "post_json") as post, contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(1, replication.main(["--reference", str(short), "--consent", str(short)]))
            post.assert_not_called()


if __name__ == "__main__":
    unittest.main()
