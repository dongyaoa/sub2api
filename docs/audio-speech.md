# 语音合成接口与本地测试

本版本提供 `POST /v1/audio/speech`。请求使用本站用户 API Key，按 Key 所属分组调度上游账号；OpenAI API Key 账号，以及配置了自定义上游地址的 Gemini API Key 账号均可接入。预设音色请求会调用上游的 `/v1/audio/speech`，不会转换为 `generateContent`。

本版本还提供 `POST /v1/audio/voices` 创建本站托管的录音克隆音色。创建后，用户仍用本站 `/v1/audio/speech` 合成，只需把 `voice` 设置为返回的 `s2voice_...` 凭据。托管音色会固定到创建时的上游账号，合成时转换为该上游的 Google 格式 `/v1beta/interactions` 请求；预设音色仍走上游 `/v1/audio/speech`。

## 部署与价格配置

1. 构建并部署包含此次改动的后端版本。仅保存源码不会让已经运行的旧服务器新增接口。
2. 账号填写上游地址和上游 Key，并在账号模型列表或映射中包含要提供的模型。当前供应商可使用 `gemini-3.8-flash-tts`；其他模型仍需上游实际支持。
3. 在对应分组或渠道的模型定价中配置该模型的**按次 + 默认单价**，并确认模型广场展示正确。本接口只接受平价按次卡，不接受上下文、音色或图片尺寸阶梯价格。明确配置为 `0` 表示免费；没有默认价格、包含按次阶梯，或只有 token 价格时，接口会在请求上游前拒绝，避免错扣或漏扣费用。
4. 用户用绑定该分组的本站 Key 调用语音接口。价格来自现有模型定价和倍率配置，不在语音转发代码中固定金额。每次成功合成计一次，字符长度不改变次数；请求执行期间使用同一份价格快照完成预留和结算。

录音克隆沿用所选模型的上述价格：**创建音色成功计 1 次，随后合成语音成功再计 1 次**。点击“创建并合成”会执行这两个独立计费操作。创建失败不会自动切换为预设音色。

当前接口支持同步音频返回：上游完整返回并通过检查后，向用户发送二进制音频。可以使用 OpenAI 兼容 SDK 的 `audio.speech.with_streaming_response.create` 保存文件，但服务端目前会先完整接收上游音频；这不是聊天 SSE，也不承诺逐音频块低延迟转发。

计费以收到上游完整、非空的音频为准。上游错误、空响应或截断响应不计成功合成；如果上游已成功生成，而用户中途断开连接，仍会记录并结算这次成功生成的费用。

## 推荐：使用本地 HTML 测试页

需要 Python 3.10 或更新版本，无需安装第三方库。在项目根目录运行：

```powershell
python scripts/serve_audio_playground.py --base-url "https://api.zicc.cc/v1"
```

浏览器打开 **http://127.0.0.1:8765/**，输入本站用户 Key。页面包含两个页签：

- **语音合成**：填写模型、文本、预设或自定义音色、风格指令；生成后可播放和下载 WAV。请求 PCM 时，页面会按 24 kHz 单声道 16-bit 格式包装为 WAV。
- **录音克隆**：上传参考录音和本人授权录音，本地试听；勾选授权确认后选择“仅创建音色”或“创建并合成”。创建成功后还可复制音色凭据，或继续使用该音色合成新文本。

参考录音需为同一位成年说话人的 10—30 秒清晰录音。授权录音最长 60 秒，必须由同一人朗读：

> 我是此声音的拥有者并授权谷歌使用此声音创建语音合成模型

页面可在本地把浏览器支持的 WAV、MP3 等录音解码并转换为 24 kHz 单声道 16-bit WAV。每份转换后的 WAV 最多 10 MiB；原始文件最多 25 MiB。录音选好后会显示实际时长和大小，并可在上传前试听。

本地 Python 服务仅监听 `127.0.0.1`，目标站点由启动参数固定。页面通过同源代理发送请求，无需修改服务器 CORS；代理只开放语音创建和合成两个路径，并校验 Host、Origin 和随机会话凭据，不记录 Key 或录音。更改目标站点时，停止本地服务并用新的 `--base-url` 重新启动。端口被占用可加 `--port 8766`。

也可直接双击 `scripts/audio-playground.html` 使用浏览器直连模式；目标站点需要允许该请求跨域。遇到浏览器跨域错误时，使用上面的 Python 启动方式。

页面不会把 Key、录音或音色凭据写入浏览器持久存储。刷新页面会清除这些数据；更换连接、Key、模型或录音也会清除已有音色凭据。创建的 `s2voice_...` 凭据需要配合创建时的本站 Key 使用，并受服务端有效期和上游账号绑定限制。

## 本站录音克隆的上游要求

你的供应商必须同时提供 Google 格式的 **`/v1beta/voices` 和 `/v1beta/interactions`**。本站使用所选账号自己的上游地址和凭据访问这两个入口，不会把第三方供应商 Key 改发到 Google 官方服务。

如果供应商只支持截图中的 `/v1/audio/speech`，它仍可提供预设音色合成；录音克隆需要供应商补齐另外两个入口。上游不支持克隆相关入口时，本站会返回 `501` 和具体说明。

创建本站音色的请求结构：

```json
{
  "model": "gemini-3.8-flash-tts",
  "display_name": "我的播客声音",
  "source_audio": {"mime_type": "audio/wav", "data": "参考录音的Base64"},
  "consent_audio": {"mime_type": "audio/wav", "data": "本人授权录音的Base64"}
}
```

请求地址为本站 `POST /v1/audio/voices`，鉴权使用 `Authorization: Bearer 本站用户Key`。成功响应中的 `id` 为 `s2voice_...`。随后向本站 `POST /v1/audio/speech` 发送：

```json
{
  "model": "gemini-3.8-flash-tts",
  "input": "你好，这是我的专属声音。",
  "voice": "s2voice_创建接口返回的完整凭据",
  "response_format": "wav"
}
```

不要把 Google 官方项目单独创建的 `voicekey_...` 当作本站 `s2voice_...` 使用。本站凭据用于维持用户、Key、模型及上游账号之间的绑定。

本站使用 `store: false` 创建音色，不把原始录音持久化。加密的本站音色凭据最长有效 7 天；供应商返回更短有效期时，以较短时间为准。凭据只能配合创建时的同一个本站 Key 和分组使用。更换上游账号的 Key、地址或服务器 `JWT_SECRET` 后，需要重新创建音色；正常部署更新应保持 `JWT_SECRET` 一致。已绑定账号不可用时会明确报错，不会把音色请求换到另一个账号。

## 使用本站接口合成语音

需要 Python 3.10 或更新版本，无需安装第三方库。在项目根目录运行：

```powershell
python scripts/test_audio_speech.py
```

脚本默认调用 `https://api.zicc.cc/v1/audio/speech`，并隐藏输入本站 Key。成功后检查响应类型和 WAV 完整性，保存为当前目录的 `speech.wav`。错误响应不会覆盖已有音频文件。

自定义文本、音色、风格和输出位置：

```powershell
python scripts/test_audio_speech.py --base-url "https://api.zicc.cc/v1" --model "gemini-3.8-flash-tts" --voice "Zephyr" --text "你好，欢迎收听今天的节目。" --instructions "请使用温暖、自然的中文播客语气，语速稍慢。" --output "speech.wav"
```

测试本机运行的后端可把 `--base-url` 换成实际监听地址，例如 `http://localhost:8080/v1`。测试另一模型只需修改 `--model`。选择 `--format pcm` 时，脚本会按此上游文档的 24 kHz、单声道、16-bit PCM 包装成可播放的 WAV。

可选环境变量：`SUB2API_API_KEY`、`SUB2API_BASE_URL`。API Key 不需要写进源码或命令行参数。

`--extra-json 参数.json` 会透传供应商支持的额外 JSON 字段，例如 `speakers`；具体字段结构以该供应商文档为准。可用它传递已获供应商确认的语音扩展参数，不能用未公开参数假定上游具备录音克隆能力。

## 使用 Google 官方接口测试录音克隆

选择 `Kore`、`Zephyr` 是使用预设音色。录音克隆需要先上传参考录音和本人授权录音，创建新的克隆音色。

以下脚本按 [Google AI Studio Voice Replication 文档](https://aistudio.google.com/docs/voice-replication) 的官方流程编写：

1. `POST https://generativelanguage.googleapis.com/v1beta/voices`，使用 `x-goog-api-key`，上传 `source_audio`、`consent_audio`，并设置 `store: false`。
2. 从响应 Voice 对象的 `key` 获取 `voicekey_...`。官方 SDK 示例把这个对象命名为 `replicated_voice`，因此示例写作 `replicated_voice.key`；这个变量名不代表 REST JSON 必须有同名包装字段。
3. 使用**同一 Google 项目的 Key** 调用 `/v1beta/interactions`，把该音色填入 `generation_config.speech_config`，获取并保存生成的 WAV。

准备两份 WAV 文件：

- `reference.wav`：同一位成年说话人的 10—30 秒真实参考录音。
- `consent.wav`：该说话人本人清晰朗读下面这句授权文本的录音。

> 我是此声音的拥有者并授权谷歌使用此声音创建语音合成模型

两份录音推荐导出为 **24 kHz、单声道、16-bit PCM WAV**。脚本校验 WAV 和参考录音长度，不会伪造授权录音。

把两份文件放在项目根目录，然后运行：

```powershell
python scripts/test_google_voice_replication.py --reference "reference.wav" --consent "consent.wav" --text "你好，这是使用我的录音创建的声音。欢迎收听今天的节目。" --output "cloned-voice.wav"
```

脚本隐藏输入 **Google 项目的 Gemini API Key**，也可读取环境变量 `GEMINI_API_KEY`。它会先创建音色，再生成音频；两个步骤都会访问 Google 官方服务，需要该项目具备对应模型、Voice Replication 和 Interactions 的使用权限及可用额度。`store: false` 的音色 Key 按官方文档绑定项目并有 7 天有效期；本脚本每次创建新音色，不把 Key 写入磁盘。

脚本已做离线模拟验证，尚未使用真实 Google 项目调用并生成克隆音频。真实结果取决于该项目的接口权限、模型可用性及录音审核结果。

**这份独立克隆脚本直连 Google，不经过本站，也不使用本站余额。** 它用于验证你自己的 Google 项目权限和官方协议。测试本站克隆和计费应使用上面的 HTML 测试页。Google 项目的音色 Key 不能假定可以跨项目或交给第三方供应商使用。

## 常见结果

| 结果 | 检查方向 |
|---|---|
| 本站 `/v1/audio/speech` 返回 404 | 确认已部署新增接口的版本，以及反向代理路径正确 |
| 401 / 403 | 区分本站用户 Key、上游 Key 与 Google 项目 Key，检查各自权限 |
| 价格未配置或不支持当前计费模式 | 为对应模型配置明确的按次价格，检查模型映射后的计费模型 |
| 没有可用账号 | 检查分组绑定、API Key 账号类型、模型白名单、自定义上游地址和并发限制 |
| 返回 HTML / JSON 而不是音频 | 检查实际接口地址和上游错误，脚本不会将其保存为 WAV |
| Google 录音克隆报模型或权限错误 | 确认该 Google 项目已获得官方文档中接口及模型的使用权限 |
| 本站录音克隆返回 501 | 供应商需要支持 `/v1beta/voices` 与 `/v1beta/interactions`，仅有预设 TTS 接口不够 |
| 本地页面提示代理会话无效 | 代理重启后刷新页面，重新获取本地会话凭据 |

离线脚本回归测试不调用任何上游：

```powershell
python -m unittest discover -s scripts/tests -p "test_audio_*.py" -v
```

后续更新建议在自己的分支合并官方代码，再构建和部署定制版本；直接替换为尚未包含这些改动的官方程序或镜像，会失去本次新增接口。
