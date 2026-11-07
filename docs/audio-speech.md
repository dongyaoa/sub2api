# 语音合成 API 接入文档

本站提供 OpenAI 兼容的语音合成接口。输入文本、选择音色，即可生成音频文件。调用时使用**本站用户 API Key**，可用模型和价格以该 Key 所属分组的模型广场为准。

接口参数核对日期：2026-11-07。供应商接口说明入口：[语音模型接口说明](https://ai-proxy.niaotv.art/key)。实际调用请使用本文的本站地址和本站 Key。

## 1. 接口地址与鉴权

| 项目 | 内容 |
|---|---|
| SDK Base URL | `https://api.zicc.cc/v1` |
| 完整请求地址 | `https://api.zicc.cc/v1/audio/speech` |
| 请求方法 | `POST` |
| 鉴权请求头 | `Authorization: Bearer YOUR_SUB2API_KEY` |
| 请求类型 | `Content-Type: application/json` |
| 成功响应 | 二进制音频，不是 JSON |
| 示例模型 | `gemini-3.8-flash-tts` |

建议优先使用 `gemini-3.8-flash-tts` 或 `gemini-3.8-flash-lite-tts`。可接入的语音模型如下，实际开放情况取决于 Key 的分组权限和模型广场当前列表：

| 模型名称 | 说明 |
|---|---|
| `gemini-3.8-flash-tts` | 推荐首先测试的模型。 |
| `gemini-3.8-flash-lite-tts` | 可选的 3.8 系列语音模型。 |
| `gemini-3.1-flash-tts-preview` | 以本站模型广场是否开放为准。 |
| `gemini-2.5-pro-preview-tts` | 以本站模型广场是否开放为准。 |
| `gemini-2.5-flash-preview-tts` | 以本站模型广场是否开放为准。 |

以上模型均使用本文的语音接口；不要把本接口地址替换为聊天接口。

## 2. 最小请求

```json
{
  "model": "gemini-3.8-flash-tts",
  "input": "你好，欢迎收听今天的节目。",
  "voice": "Kore",
  "response_format": "wav"
}
```

增加语气和表达要求时，填写 `instructions`：

```json
{
  "model": "gemini-3.8-flash-tts",
  "input": "你好，欢迎收听今天的节目。今天我们聊聊如何保持专注。",
  "voice": "Zephyr",
  "instructions": "请用自然、温暖的普通话播客语气朗读，语速稍慢，在句号处适当停顿。",
  "response_format": "wav"
}
```

## 3. 参数说明

以下是 **HTTP JSON 请求体**中的字段：

| 字段 | 类型 | 要求 | 说明 |
|---|---|---|---|
| `model` | string | 必填 | 当前 Key 有权限调用的语音模型名称。 |
| `input` | string | 必填，或使用文本别名 | 要朗读的非空文本。推荐统一使用此字段。 |
| `voice` | string | 可选，建议显式填写 | 预设音色名称，例如 `Kore`、`Zephyr`；该模型的上游默认音色为 `Kore`。保留名称大小写。 |
| `instructions` | string | 可选 | 自然语言语气、情绪、节奏、口音或表达方式要求。 |
| `response_format` | string | 建议显式填写 | 使用 `wav` 或 `pcm`；推荐 `wav`，便于直接播放。 |
| `text` | string | 可选别名 | `input` 的文本别名。 |
| `prompt` | string | 可选别名 | 文本输入的另一别名；推荐使用 `input`。 |
| `style_prompt` | string | 可选别名 | 风格指令别名；未提供 `instructions` 时按此字段处理。 |
| `speakers` | array | 可选 | 双人对话的说话人到预设音色的映射，最多 2 项；每项包含 `speaker` 和 `voice`。 |

文本按 `input`、`text`、`prompt` 的顺序选取第一个非空值。请一次只使用一个文本字段，风格指令也优先统一使用 `instructions`，避免歧义。

### 预设音色

下列 30 个名称可作为 `voice` 的选值，请保留大小写。建议先用 `Kore` 或 `Zephyr` 测试；具体音色表现可通过同一段文本试听比较。

| 音色名称 | 音色名称 | 音色名称 |
|---|---|---|
| `Achernar` | `Achird` | `Algenib` |
| `Algieba` | `Alnilam` | `Aoede` |
| `Autonoe` | `Callirrhoe` | `Charon` |
| `Despina` | `Enceladus` | `Erinome` |
| `Fenrir` | `Gacrux` | `Iapetus` |
| `Kore` | `Laomedeia` | `Leda` |
| `Orus` | `Pulcherrima` | `Puck` |
| `Rasalgethi` | `Sadachbia` | `Sadaltager` |
| `Schedar` | `Sulafat` | `Umbriel` |
| `Vindemiatrix` | `Zephyr` | `Zubenelgenubi` |

音色名称参考 [Google Gemini 语音生成文档](https://ai.google.dev/gemini-api/docs/speech-generation#prebuilt-voices)；不同模型的实际可用性以接口响应为准。

### 双人对话

双人合成使用顶层 `speakers` 数组，最多 2 位说话人。每项结构如下：

| 字段 | 类型 | 说明 |
|---|---|---|
| `speaker` | string | 文本中的说话人名称，例如 `Host`、`Guest`；必须与文本标记一致。 |
| `voice` | string | 该说话人使用的预设音色，例如 `Zephyr`、`Puck`。 |

完整 HTTP 请求示例：

```json
{
  "model": "gemini-3.8-flash-tts",
  "input": "Host: 你好，欢迎收听今天的节目。\nGuest: 大家好，很高兴来到这里。",
  "voice": "Kore",
  "instructions": "请用自然的普通话进行轻松访谈，让两位说话人轮流表达，停顿清晰。",
  "speakers": [
    {"speaker": "Host", "voice": "Zephyr"},
    {"speaker": "Guest", "voice": "Puck"}
  ],
  "response_format": "wav"
}
```

通过 OpenAI SDK 调用时，将已经确认格式的数组放入 `extra_body={"speakers": speakers}`。直接发送 HTTP JSON 时，应将 `speakers` 放在请求顶层，不添加 `extra_body` 包装。

### 自然语言风格选项

本地网页提供以下全部风格预设。它们是帮助编写提示语的快捷选项，**不是额外的 HTTP 字段，也不是上游参数的完整枚举**；选项统一组合为一个 `instructions` 字符串。使用 API 时可以直接填写任意自然语言要求，也可以修改网页生成的“最终风格指令”。

| 选项类别 | 网页中的全部可选项 |
|---|---|
| 使用场景 | 默认、自然对话、播客、有声书、新闻播报、客服、故事讲述 |
| 情绪 | 默认、自然、愉快、平静、温暖、严肃、悲伤、兴奋、安慰 |
| 语速 | 默认、慢、适中、快 |
| 节奏 | 默认、均匀、轻快、舒缓、有停顿 |
| 表达方式 | 默认、正常说话、耳语、轻声 |
| 语言 / 口音 | 自动识别、普通话、英语 · 美式口音、英语 · 英式口音、日语、韩语、西班牙语、法语、德语 |
| 音高倾向 | 默认、稍低沉、稍明亮 / 偏高 |
| 音量 / 力度 | 默认、轻柔、正常、有力 |
| 停顿方式 | 默认、句末短停顿、段落间停顿、重点前后停顿 |
| 额外要求 | 自由填写文本，例如“人物对白带一点笑意” |
| 最终风格指令 | 可直接编辑并发送的完整 `instructions` 文本 |

“默认”和“自动识别”不追加对应风格指令。所有选项均为默认且没有额外要求时，可以省略 `instructions`。再次修改选项会重新组合最终指令，提交前可通过参数预览确认实际内容。

例如选择“播客、温暖、慢、普通话”，可以发送：

```text
像专业播客主持人一样朗读，与听众自然交流。语气温暖、亲切。语速放慢，吐字清楚。使用标准普通话朗读中文内容，保留原文含义。
```

音高、音量、语速和停顿均通过自然语言描述，不代表固定赫兹、分贝、速度倍率或毫秒值。语言 / 口音选项描述的是发音偏好，不会自动翻译 `input`。最终效果由所选模型决定，可通过修改朗读文本和 `instructions` 调整。

### SDK 参数与 HTTP 字段的区别

- `extra_body` 是 OpenAI SDK 合并额外 JSON 字段的客户端选项，**不是 HTTP 请求体中的包装层**。例如已确认的额外字段应通过 `extra_body={"style_prompt": "自然语气"}` 传入，SDK 发出的 JSON 中是顶层 `style_prompt`。
- SDK 的 `timeout=120` 是客户端超时设置，**不要把 `timeout` 写入请求 JSON**。
- `with_streaming_response` 是 SDK 读取响应的方式，**不要因此在 JSON 中加入聊天接口的 `stream` 参数**。
- 本文没有将 `speed`、`pitch`、`seed` 等字段列为已支持参数。需要额外控制时，使用 `instructions` 描述期望效果；模型实际表达以生成结果为准。

## 4. Python：OpenAI 官方 SDK

安装依赖：

```shell
python -m pip install -U openai
```

将下面代码保存为 `speech_demo.py`，运行 `python speech_demo.py`。程序会隐藏输入本站 Key，并保存 `speech.wav`。

```python
import getpass
from pathlib import Path
from openai import OpenAI

client = OpenAI(
    base_url="https://api.zicc.cc/v1",
    api_key=getpass.getpass("请输入本站 API Key：").strip(),
    timeout=120.0,
    max_retries=0,
)

output = Path("speech.wav")
with client.audio.speech.with_streaming_response.create(
    model="gemini-3.8-flash-tts",
    input="你好，欢迎收听今天的节目。",
    voice="Zephyr",
    instructions="请用自然、温暖的普通话朗读，语速稍慢。",
    response_format="wav",
) as response:
    response.stream_to_file(output)

print("已保存：", output.resolve())
```

示例关闭 SDK 自动重试，便于在出现超时或连接问题时先核对请求结果。再次成功调用会产生新的一次合成费用。

### Python SDK 双人合成示例

安装依赖后，将下面代码保存为 `dialogue_demo.py`，运行 `python dialogue_demo.py`。顶层 `voice="Kore"` 用于满足 SDK 的必填参数；双人音色通过 `speakers` 中对应的 `voice` 指定。

```python
import getpass
from pathlib import Path
from openai import OpenAI

client = OpenAI(
    base_url="https://api.zicc.cc/v1",
    api_key=getpass.getpass("请输入本站 API Key：").strip(),
    timeout=120.0,
    max_retries=0,
)

output = Path("dialogue.wav")
with client.audio.speech.with_streaming_response.create(
    model="gemini-3.8-flash-tts",
    input="Host: 你好，欢迎收听今天的节目。\nGuest: 大家好，很高兴来到这里。",
    voice="Kore",
    instructions="请用自然的普通话进行轻松访谈，让两位说话人轮流表达，停顿清晰。",
    response_format="wav",
    extra_body={
        "speakers": [
            {"speaker": "Host", "voice": "Zephyr"},
            {"speaker": "Guest", "voice": "Puck"},
        ]
    },
) as response:
    response.stream_to_file(output)

print("已保存：", output.resolve())
```

## 5. Python：无需第三方依赖的测试脚本

获取随接入资料提供的 `scripts/test_audio_speech.py`。使用 Python 3.10 或更新版本，在项目根目录运行：

```shell
python scripts/test_audio_speech.py --base-url "https://api.zicc.cc/v1" --model "gemini-3.8-flash-tts" --voice "Kore" --text "你好，这是一段语音接口测试。" --instructions "请自然朗读，语速稍慢。" --output "speech.wav"
```

脚本会隐藏输入本站 Key，显示状态码和响应类型，校验音频后保存文件。非音频或不完整响应不会覆盖已有输出文件。

| 脚本参数 | 作用 |
|---|---|
| `--base-url` | API 基础地址，默认 `https://api.zicc.cc/v1`。 |
| `--model` | 模型名称，默认 `gemini-3.8-flash-tts`。 |
| `--voice` | 预设音色，默认 `Kore`。 |
| `--text` | 朗读文本。 |
| `--instructions` | 自然语言风格要求。 |
| `--format wav` 或 `--format pcm` | 请求格式；默认 `wav`，PCM 会包装成 WAV 后保存。 |
| `--output` | 输出文件路径，默认 `speech.wav`。 |
| `--timeout` | 客户端超时秒数，默认 `120`。 |
| `--extra-json` | 额外参数的 UTF-8 JSON 文件；字段合并到请求顶层，仅填写当前模型已确认支持的字段。 |

也可通过 `SUB2API_API_KEY` 和 `SUB2API_BASE_URL` 环境变量配置 Key 与地址。`--extra-json` 不允许覆盖 `model`、`input` 和 `response_format`；这三个字段请使用对应命令行参数。

## 6. Linux / macOS：cURL

先将第 2 节的 JSON 保存为 UTF-8 编码的 `request.json`。下面的 Bash 命令隐藏输入本站 Key，并保存音频：

```bash
read -r -s -p "请输入本站 API Key: " SUB2API_API_KEY
printf '\n'
curl --fail --show-error --max-time 120 \
  "https://api.zicc.cc/v1/audio/speech" \
  -H "Authorization: Bearer ${SUB2API_API_KEY}" \
  -H "Content-Type: application/json" \
  --data-binary @request.json \
  --output speech.wav
unset SUB2API_API_KEY
```

`--data-binary @request.json` 会发送文件内容。`speech.wav` 应为可以播放的音频；需要自动校验响应内容时，推荐第 5 节的 Python 脚本。

## 7. Windows：PowerShell

Windows PowerShell 5.1 中，`curl` 可能是 `Invoke-WebRequest` 的别名。不要直接粘贴 Bash 中使用反斜杠 `\` 换行的命令；可以使用下面的 PowerShell 示例。中文请求体显式编码为 UTF-8：

```powershell
$secureKey = Read-Host "请输入本站 API Key" -AsSecureString
$key = [System.Net.NetworkCredential]::new("", $secureKey).Password
$payload = @{
    model = "gemini-3.8-flash-tts"
    input = "你好，欢迎收听今天的节目。"
    voice = "Kore"
    instructions = "请用自然、温暖的普通话朗读。"
    response_format = "wav"
} | ConvertTo-Json

try {
    $result = Invoke-WebRequest `
        -Uri "https://api.zicc.cc/v1/audio/speech" `
        -Method POST `
        -Headers @{ Authorization = "Bearer $key" } `
        -ContentType "application/json; charset=utf-8" `
        -Body ([System.Text.Encoding]::UTF8.GetBytes($payload)) `
        -OutFile ".\speech.wav" `
        -PassThru `
        -UseBasicParsing `
        -TimeoutSec 120 `
        -ErrorAction Stop

    Write-Host "HTTP 状态码：" $result.StatusCode
    Write-Host "响应类型：" $result.Headers["Content-Type"]
    Write-Host "已保存：" (Join-Path $PWD "speech.wav")
} catch {
    Write-Host "请求失败：" $_.Exception.Message
    Write-Host "错误详情：" $_.ErrorDetails.Message
} finally {
    Remove-Variable key, secureKey -ErrorAction SilentlyContinue
}
```

使用 Windows 自带的 cURL 程序时，命令名称应写成 `curl.exe`。Bash 的 `\` 换行、PowerShell 的反引号换行和 CMD 的 `^` 换行不能混用。

## 8. 音频格式与响应方式

| 格式 | 返回内容 | 使用方式 |
|---|---|---|
| `wav` | 带 WAV 文件头的音频 | 保存为 `.wav`，通常可直接播放。 |
| `pcm` | 24 kHz、单声道、16-bit signed little-endian 原始 PCM 字节 | 不带 WAV 文件头；播放器需指定格式，或先包装为 WAV。 |

该模型提供 24 kHz 音频。Python 标准库测试脚本会报告 WAV 文件头中的实际采样率、声道数、位深和时长。

接口为同步请求：服务端在完整接收并检查上游音频后返回文件；响应大小受服务端上限约束。它不是 SSE 事件流，不返回聊天增量，也不保证边生成边播放。SDK 的 `with_streaming_response` 可以用于接收并写文件，但不会改变这一生成方式。

客户端建议至少等待 120 秒，并根据文本长度及服务状态调整客户端超时。增加超时不会更改模型参数，也不保证长请求一定成功。

## 9. 费用与请求结果

- 单次价格和适用倍率以当前 Key 所属分组的模型广场为准。此接口使用**平价按次**计费，每次正常成功合成计 1 次，不按文本字符数量增加次数。
- 上游报错、空音频或未完整返回的音频不产生成功合成账单。
- 如果上游已经完整生成音频，而客户端随后断开或保存失败，该次成功生成仍会计费。
- 一次请求使用开始调用前确定的价格完成结算；价格调整影响后续请求。
- 超时后先查看站内使用记录或向客服提供请求 ID，再决定是否重试。新的成功合成会再次计费。

## 10. 常见问题

| 现象 | 处理方法 |
|---|---|
| `401` 或 Key 无效 | 使用本站签发的用户 Key，检查是否禁用或已过期。 |
| `403` 或模型无权限 | 确认 Key 所属分组、模型权限；浏览器请求还需区分预检跨域错误。 |
| `404` | 完整地址应为 `https://api.zicc.cc/v1/audio/speech`，确认地址没有重复 `/v1`。 |
| `400` 参数错误 | 检查 `model`、非空 `input`、音色大小写以及参数类型。 |
| `audio_speech_pricing_required` | 当前模型未配置本接口所需的平价按次单价，请联系平台处理。 |
| `429`、并发已满 | 降低并发，按返回提示稍后再试。 |
| `503`、没有可用账号 | 当前分组或模型暂时无可用通道，请联系平台。 |
| `502`、返回 HTML/JSON 而不是音频 | 保存错误内容和请求 ID 联系平台；不要把错误页面当作 WAV。 |
| 浏览器显示跨域错误 | 使用平台允许的网页来源，或改用 Python、PowerShell、cURL；这些本地命令不受浏览器 CORS 限制。 |
| PCM 文件无法直接播放 | 按 24 kHz、单声道、16-bit signed little-endian 读取，或使用测试脚本包装为 WAV。 |

提交问题时请提供模型名称、请求时间、HTTP 状态码、错误内容和请求 ID，不要提供完整 API Key。

## 11. 使用本地网页试听与参数预览

网页可选择单人或双人模式、音色及自然语言风格，查看实际请求参数，生成后试听并下载音频。

需要 Python 3.10 或更新版本，无需安装第三方库。请将下面三个文件保存在**同一目录**，文件名保持不变：

```text
audio-playground.html
serve_audio_playground.py
test_audio_speech.py
```

在这个目录打开终端，运行：

```shell
python serve_audio_playground.py --base-url "https://api.zicc.cc/v1"
```

如果使用完整项目，在项目根目录运行：

```shell
python scripts/serve_audio_playground.py --base-url "https://api.zicc.cc/v1"
```

浏览器打开 `http://127.0.0.1:8765/`，填写本站 Key、模型和朗读文本。选择单人音色，或在双人模式中填写两个说话人名称和音色；文本中的 `Host:`、`Guest:` 等标记应与名称一致。

风格选项会组合成 `instructions`，也可以直接编辑“最终风格指令”。点击“查看实际发送参数”检查完整 JSON，或点击“复制请求 JSON”用于自己的程序；预览和复制内容不包含 API Key。点击“生成语音”后，可在页面播放并下载音频。

此启动方式通过本机服务发送请求，不需要修改目标站点的浏览器跨域配置。三个文件必须一起保留：本机服务会读取 HTML，并导入同目录的 `test_audio_speech.py`。端口被占用时加 `--port 8766`，然后打开终端显示的新地址。关闭服务可在终端按 `Ctrl+C`。

也可以直接打开 `audio-playground.html`，但此时由浏览器直连本站，需本站允许当前网页来源。若出现 CORS 或预检错误，推荐使用上述 Python 启动方式。

## 附录：平台管理员配置说明

部署支持 `/v1/audio/speech` 的版本，并将上游 API Key 账号绑定到用户分组。Gemini 类型账号需要配置实际提供 OpenAI 兼容语音接口的自定义上游地址；也可使用 OpenAI API Key 类型账号。模型列表或映射应覆盖用户请求的模型。

在对应渠道或分组的模型定价中配置 `per_request` 和明确的默认单价；合法的 `0` 表示免费。此接口不接受有效的按次阶梯或仅有 token 价格的配置。按次价卡使用与模型广场相同的配置来源，不使用固定的模型费用；同时保留渠道计费模型来源、模型映射和用户倍率设置。

发布更新后，可运行无外部请求的客户端回归检查：

```shell
python -m unittest discover -s scripts/tests -p "test_audio_speech_clients.py" -v
```
