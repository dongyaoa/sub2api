# 视频生成 API 用户调用文档

更新时间：2026-11-07（Asia/Shanghai）

适用范围：本平台创作工作台使用的 Muse、Gemini Omni、Grok 视频 API，以及共用的报价、任务查询、下载与历史接口。

本文面向 API 调用者。请求应发送到**本平台域名**，使用**本平台生成的 API Key**。账户背后采用 OpenAI、Gemini 或 Grok 协议，不改变本文的 HTTP 调用方式。

本文核对了本平台当前实现、提供的 Gemini Omni 上游参数表、Muse 上游实现，以及 xAI 官方视频文档。不同模型的参数不能混用。特别是 `gemini-omni-1.2-flash` 是本次对接上游提供的模型名称；不要据此套用 Google Veo 或 Gemini 原生 `generateContent` API。

## 目录

1. [开始调用与鉴权](#1-开始调用与鉴权)
2. [模型与功能一览](#2-模型与功能一览)
3. [接口清单](#3-接口清单)
4. [查询可用模型与价格](#4-查询可用模型与价格)
5. [Gemini Omni 完整参数](#5-gemini-omni-完整参数)
6. [Muse 完整参数](#6-muse-完整参数)
7. [Grok 生成参数与官方扩展](#7-grok-生成参数与官方扩展)
8. [Grok 视频编辑与延长](#8-grok-视频编辑与延长)
9. [任务状态、下载与历史](#9-任务状态下载与历史)
10. [计费规则](#10-计费规则)
11. [可运行客户端与 SDK](#11-可运行客户端与-sdk)
12. [错误码与排查](#12-错误码与排查)
13. [兼容性与未支持能力](#13-兼容性与未支持能力)
14. [参考来源](#14-参考来源)

## 1. 开始调用与鉴权

### 1.1 基础地址

下面统一使用示例域名：

```text
站点根地址：https://api.example.com
视频创建地址：https://api.example.com/v1/videos
```

请将 `api.example.com` 替换为实际平台域名。`/api/v1` 是网站管理接口前缀，**不是本文的视频生成接口前缀**。如果客户端已经设置了 `base_url=https://api.example.com/v1`，后续路径不要再重复拼接 `/v1`。

### 1.2 请求头

```http
Authorization: Bearer YOUR_SUB2API_KEY
Content-Type: application/json
Accept: application/json
```

| 请求头 | 必需 | 说明 |
| --- | --- | --- |
| `Authorization` | 是，推荐方式 | `Bearer ` 后填写本平台 Key |
| `Content-Type` | 提交 JSON 时是 | `application/json` |
| `Accept` | 否 | 查询使用 `application/json`；下载可使用 `video/mp4` |
| `x-api-key` / `x-goog-api-key` | 可替代鉴权 | 网关兼容这两种请求头；建议统一用 Bearer，不同时发送多套凭据 |

不要把 Key 写到 URL 的 `key` 或 `api_key` 查询参数；当前网关会拒绝此方式。浏览器 `<video src="…">` 无法直接携带 Bearer 请求头，前端应先用 `fetch` 下载 Blob，再创建对象 URL 播放。

### 1.3 开始前确认

- Key 已启用并绑定到正确分组，分组允许媒体生成。
- 先查询视频模型目录，确认 Key 有权使用目标模型。
- 先获取报价，确认所选模型及分辨率有价格。
- 素材 URL 必须能被上游访问；本机文件路径、`localhost`、登录后才能打开的图片链接不能直接作为上游素材。
- 查询、下载、历史请求使用**创建任务时的同一个 Key**。同一用户的其他 Key 也不能直接读取该任务。

### 1.4 最小调用示例

以下命令适用于 Bash / zsh；Windows PowerShell 建议使用后面的 Python / Node.js 客户端。

```bash
export SUB2API_BASE_URL='https://api.example.com'
export SUB2API_API_KEY='YOUR_SUB2API_KEY'

curl --fail-with-body -sS "$SUB2API_BASE_URL/v1/videos" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H 'Content-Type: application/json' \
  --data '{
    "model": "gemini-omni-1.2-flash",
    "prompt": "镜头缓缓掠过雨后的森林，晨光照亮树叶上的水珠",
    "duration": 5,
    "resolution": "720p",
    "aspect_ratio": "16:9"
  }'
```

创建返回的是任务信息，**不是视频二进制文件**。保存返回的 `id` 或 `request_id`，之后轮询、下载。

## 2. 模型与功能一览

“已有适配”表示本平台明确处理该模型参数和任务生命周期；“依赖上游”表示网关可以保留相关 JSON 字段，但实际能力取决于所选上游。

| 项目 | `gemini-omni-1.2-flash` | `muse-video` | `grok-imagine-video` | `grok-imagine-video-1.5` |
| --- | --- | --- | --- | --- |
| 本平台账户协议 | Gemini API Key | OpenAI API Key | Grok | Grok |
| 文生视频 | 已有适配 | 已有适配 | 支持 | 最新官方支持；现工作台文生选项与上游版本可能有差异，API 见第 7 节 |
| 单图生视频 | 首帧 | 单张首帧 | `image` | `image` |
| 首尾帧 | 已有适配 | 不支持 | 官方不支持 | 官方支持，依赖上游透传 |
| 参考图片 | 最多 10 张 | 不支持多参考 | 官方最多 7 张 | 官方最多 14 张 |
| 参考视频 | 最多 3 段，用于动作/镜头参考 | 不支持 | 使用独立编辑/延长接口 | 不等同于 Omni 的 `video_references` |
| 声音参考 | 无已适配字段 | 无已适配字段 | 官方不支持 | 官方最多 3 个声音参考，权限限制见第 7 节 |
| 中间关键帧 | 无已适配字段 | 不支持 | 官方不支持 | 官方最多 4 个，依赖上游透传 |
| 新生成时长 | 3–15 秒，默认 5 | 1–60 秒，默认 5；是期望时长 | 1–15 秒，默认 8；参考图模式最多 10 秒 | 1–15 秒，默认 8 |
| 分辨率 | 360p / 720p / 1080p / 4K | 模型决定，`auto` | 480p / 720p | 文/单图最高原生 1080p；参考模式最高 720p |
| 视频编辑/延长 | 不支持独立端点 | 不支持 | 官方支持，本平台有路由 | 官方不支持 |
| 随机种子 | 已有适配 | 不支持 | 当前官方生成参数未列出 | 当前官方生成参数未列出 |
| 价格 | 当前分组/渠道价卡，按次或按秒 | 当前分组/渠道价卡，按次或按秒 | 原分组视频价及明确模型价卡 | 原分组视频价及明确模型价卡 |

别名说明：

- Omni 已识别 `gemini-omni-1.2`、`omni-1.2`、`omni-featured`；只使用你的模型目录实际返回的名称。
- `svggrok-imagine-video-1.5` 是站点兼容的已配置名称，不是 xAI 官方标准模型名。需由管理员配置上游映射；本平台按 1.5 家族提供工作台选项与默认计费规则。
- xAI 官方还列出 `grok-imagine-video-1.5-lite`。本指南不把它列为当前工作台默认支持模型；仅在管理员另行配置并验证后使用，不能从官方存在该名称推断当前 Key 可用。

## 3. 接口清单

| 方法 | 路径 | 用途 | 模型范围 |
| --- | --- | --- | --- |
| GET | `/v1/models?video_studio=1` | 当前 Key 的视频模型目录 | 本平台已识别的工作台视频模型 |
| GET | `/v1/videos/pricing?model={model}` | 当前模型报价和计价单位 | 本指南模型 |
| POST | `/v1/videos` | 创建生成任务，推荐统一入口 | Omni / Muse / Grok |
| POST | `/v1/videos/generations` | 创建生成任务的兼容入口 | 同上；Grok 原生风格 |
| POST | `/v1/videos/edits` | 编辑已有视频 | Grok 标准模型，需上游支持 |
| POST | `/v1/videos/extensions` | 延长已有视频 | Grok 标准模型，需上游支持 |
| GET | `/v1/videos/{id}` | 查询任务状态 | 创建时的原 Key |
| GET | `/v1/videos/{id}/content` | 下载视频二进制 | 创建时的原 Key |
| GET | `/v1/videos/tasks?limit=10` | 查询该 Key 的任务历史 | 当前 Key |
| DELETE | `/v1/videos/tasks` | 清空该 Key 的历史列表 | 不等于取消上游任务 |

兼容路径还包括不带 `/v1` 的 `/videos…`，以及 `/v1/videos/generations/{id}`、`/v1/videos/edits/{id}`、`/v1/videos/extensions/{id}` 及其 `/content` 后缀。新客户端统一使用上表路径。

`pricing` 和 `tasks` 是保留路径，不要作为自定义任务 ID。任务 ID 由服务端返回，作为路径参数时应进行 URL 编码。

## 4. 查询可用模型与价格

### 4.1 视频模型目录

```bash
curl --fail-with-body -sS "$SUB2API_BASE_URL/v1/models?video_studio=1" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

示例响应（不同 Key 返回的列表不同）：

```json
{
  "object": "list",
  "data": [
    {"id": "gemini-omni-1.2-flash", "object": "model"}
  ]
}
```

模型列表为空表示当前分组没有可用的、已识别的视频模型。模型出现在目录中不代表已经配置价格，两项需分别检查。

### 4.2 模型报价

```bash
curl --fail-with-body -sS \
  "$SUB2API_BASE_URL/v1/videos/pricing?model=gemini-omni-1.2-flash" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

按次报价示例，以下数值仅用于说明：

```json
{
  "model": "gemini-omni-1.2-flash",
  "billing_model": "gemini-omni-1.2-flash",
  "billing_mode": "per_request",
  "pricing_source": "group",
  "rate_multiplier": 0.1,
  "tiers": {
    "360p": {"base_price": 0.5, "unit_price": 0.05, "configured": true},
    "720p": {"base_price": 0.5, "unit_price": 0.05, "configured": true},
    "1080p": {"base_price": 0.5, "unit_price": 0.05, "configured": true},
    "4k": {"base_price": 0.5, "unit_price": 0.05, "configured": true}
  }
}
```

| 字段 | 含义 |
| --- | --- |
| `billing_mode=per_request` | `unit_price` 是单次生成价；5 秒、15 秒不会额外乘秒数 |
| `billing_mode=video` | `unit_price` 是每秒价；生成预估费用为单价 × 计费秒数 |
| `billing_mode=image` | 部分旧 Grok 价卡兼容模式；在视频报价中按次解释 |
| `billing_mode=token` | 没有可用的固定视频报价，不应在客户端自行推算成按次价格 |
| `base_price` | 价卡基础单位价格，美元 |
| `unit_price` | 已应用本次报价倍率的单位价格，美元；不要再乘一次 `rate_multiplier` |
| `pricing_source` | 可能为 `group`、`channel` 或 Grok 的 `default` |
| `configured` | 是否来自明确配置；Grok 默认价格可能为 `false`，不等于价格无效 |
| `billing_model` | 用于计费的模型标识，渠道映射后可能与请求名称不同 |

Muse 的分辨率档位键为 `auto`。Omni 只配置部分分辨率时，未配置档位会从 `tiers` 省略；不要把缺失档位解释为免费。明确返回 `unit_price: 0` 才是零价。

价格读取 Key 所属分组及关联渠道的实际价卡。模型广场中其他分组的价格不会自动套用到当前 Key。管理员保存价格后，可重新请求报价；无须重新创建 Key。

## 5. Gemini Omni 完整参数

### 5.1 创建字段

使用 `POST /v1/videos`，请求体为 JSON 对象。

| 参数 | 类型 | 默认值 | 可用值与限制 |
| --- | --- | --- | --- |
| `model` | string，必填 | 无 | `gemini-omni-1.2-flash` 或模型目录返回的已支持别名 |
| `prompt` | string，必填 | 无 | 非空；描述画面、动作、镜头、对白、环境声 |
| `duration` | integer | `5` | 3–15；推荐字段 |
| `seconds` | integer 或数字字符串 | 不单独设置 | `duration` 的兼容别名，如 `15` 或 `"15"`；同时传入时必须一致 |
| `resolution` | string | `720P` | `360P`、`720P`、`1080P`、`4K`，大小写均可 |
| `aspect_ratio` | string | `16:9` | `16:9`、`9:16`、`auto`；不支持 `1:1` 等其他比例 |
| `image_size` | string / `[width,height]` / object | 由分辨率、画幅推导 | 档位字符串或精确尺寸；详见下一节 |
| `size` | 同 `image_size` | 不单独设置 | 兼容别名；同时提供时不能冲突 |
| `first_frame` | string，兼容 URL 对象 | 无 | 首帧图片 URL 或 base64 data URI |
| `last_frame` | string，兼容 URL 对象 | 无 | 尾帧；必须同时提供首帧 |
| `image` | string / URL object | 无 | 单首帧别名 |
| `input_reference` | string / URL object | 无 | 单首帧兼容别名；不是 multipart 文件参数 |
| `image_references` | array | 无 | 最多 10 张参考图，元素推荐使用 URL/data URI 字符串 |
| `image_urls` | array | 无 | `image_references` 的别名 |
| `reference_images` / `images` | array | 无 | 平台兼容的参考图别名，计入同一个 10 张上限 |
| `video_references` | array | 无 | 最多 3 段视频参考，用于动作/镜头；元素推荐 URL/data URI 字符串 |
| `video_urls` | array | 无 | 视频参考数组别名，计入同一个上限 |
| `video_url` | string / URL object | 无 | 单条视频参考别名，计入 3 段上限 |
| `seed` | integer | 不传即由上游决定 | −9223372036854775808 至 9223372036854775807，`0` 有效；JavaScript 建议限定在 ±9007199254740991 内 |
| `n` | integer | `1` | 仅允许 `1`；多条视频请创建多个任务 |

`seed` 是随机种子：相同提示词、参数、种子有助于得到相近结果，但不保证完全一致。通常省略即可。`null` 或省略 seed 表示未指定；不要用 `0` 表示“随机”。

整数请使用普通 JSON 整数字面量，例如 `5`、`42`。Omni 不接受用 `5.0` 或科学计数文本表示 `duration` / `seed`；`seed` 也不能是字符串。

### 5.2 分辨率与精确尺寸

| 分辨率 | `16:9` 横屏 | `9:16` 竖屏 |
| --- | --- | --- |
| 360P | `640x360` | `360x640` |
| 720P | `1280x720` | `720x1280` |
| 1080P | `1920x1080` | `1080x1920` |
| 4K | `3840x2160` | `2160x3840` |

同一精确尺寸的三种写法：

```json
{"image_size": "1920x1080"}
```

```json
{"image_size": [1920, 1080]}
```

```json
{"image_size": {"width": 1920, "height": 1080}}
```

- 也可写 `"image_size": "1080P"`，只指定分辨率档位。
- 精确尺寸会推导未显式提供的分辨率与画幅。若显式参数冲突，例如 `resolution=720p` 配 `1920x1080`，返回 400。
- `aspect_ratio=auto` 时建议省略 `size` 和 `image_size`；如果又指定精确尺寸，该尺寸会确定画幅。
- 只支持表中的固定尺寸，不支持任意宽高或正方形。

### 5.3 素材限制和互斥

1. **首尾帧模式**：`first_frame` 可单独使用；`last_frame` 需要 `first_frame`。
2. **参考模式**：可只传参考图、只传参考视频，或同时传最多 10 张参考图与 3 段参考视频。
3. 首尾帧模式与参考模式互斥，不能同时提交。
4. `first_frame`、`image`、`input_reference` 同时出现时必须指向同一素材；推荐只传其中一个。
5. 各数组别名合并并按相同字符串去重；合并后仍不得超限，单个数组也不得超限。
6. 单个内嵌素材解码后最大 **20 MiB（20 × 1024 × 1024 字节）**。远程 URL 的真实文件大小由上游拉取时检查。
7. 图片 data URI 使用 `data:image/...;base64,...`；视频使用 `data:video/...;base64,...`。不能只传一段没有 MIME 前缀的 base64。
8. URL 使用公开 HTTP(S)，不能含用户名/密码；API 兼容 `{"url":"https://…"}`，但统一使用字符串更容易区分 Omni 与 Grok 的请求结构。

### 5.4 完整请求示例

文生视频，15 秒、1080p、横屏：

```json
{
  "model": "gemini-omni-1.2-flash",
  "prompt": "雨后城市夜景，镜头平稳跟随一辆红色电车，地面反射霓虹灯，伴随轻微雨声",
  "duration": 15,
  "resolution": "1080P",
  "aspect_ratio": "16:9",
  "image_size": "1920x1080",
  "seed": 42
}
```

首尾帧图生视频：

```json
{
  "model": "gemini-omni-1.2-flash",
  "prompt": "镜头缓缓推进，人物自然转身，从首帧过渡到尾帧",
  "duration": 10,
  "resolution": "720p",
  "aspect_ratio": "9:16",
  "first_frame": "https://assets.example.com/start.jpg",
  "last_frame": "https://assets.example.com/end.jpg"
}
```

多参考图与视频：

```json
{
  "model": "gemini-omni-1.2-flash",
  "prompt": "保持参考图中的人物和服装，参考视频的运镜方式，在海边缓缓行走",
  "duration": 15,
  "resolution": "4K",
  "aspect_ratio": "auto",
  "image_references": [
    "https://assets.example.com/person.jpg",
    "https://assets.example.com/clothes.png"
  ],
  "video_references": ["https://assets.example.com/camera.mp4"]
}
```

以上素材域名是占位符，需要替换成真实可访问的文件。

## 6. Muse 完整参数

### 6.1 参数表

推荐 `POST /v1/videos`，同样使用 JSON。

| 参数 | 类型 | 默认值 | 可用值与限制 |
| --- | --- | --- | --- |
| `model` | string，必填 | 无 | `muse-video` |
| `prompt` | string，必填 | 无 | 非空提示词 |
| `duration` | integer | `5` | 1–60，表示请求的期望时长 |
| `seconds` | integer | 不单独设置 | `duration` 的别名；**必须是 JSON 数字**，不接受 `"5"` |
| `size` | string | 省略 | 画幅提示，如 `16:9`；最长 16 个字符 |
| `aspect_ratio` | string | 省略 | 平台兼容字段，会转为 `size`；最长 16 字符 |
| `resolution` | string | `auto` | 仅 `auto`，也可省略；不能指定 720p/1080p/4K |
| `image` | string / URL object | 无 | 单张首帧图片，推荐公开 URL 或完整 data URI |
| `input_reference` | string / URL object | 无 | 单张首帧的兼容别名，不是 multipart 文件上传 |
| `n` | integer | `1` | 仅允许 `1` |

同时出现的处理顺序：`duration` 优先于 `seconds`；非空 `size` 优先于 `aspect_ratio`；有效 `image` 优先于 `input_reference`。每个已提供的时长字段仍须各自合法。建议使用一套名称，避免冲突。

常用画幅提示为 `16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`3:2`、`2:3`，并非上游强制的固定枚举。Muse API 当前没有单独的 20 MiB 素材校验，不要把 Omni 的限制直接套用；仍建议控制素材大小，并遵守站点请求体和上游上传限制。

Muse 没有独立的尾帧、多参考图、参考视频、seed、负面提示词、分辨率、视频编辑或延长控制。不要因为请求没有立刻报错就认为未知字段生效；适配器会重建上游请求，未支持字段可能被忽略。`images`、`reference_images` 会明确被拒绝。

**Muse 的 1–60 秒是接口接受的期望时长，不是输出长度保证。** 上游将时长和 `size` 作为生成提示；实际时长、画幅、分辨率由模型结果决定。`size="3840x2160"` 也不能保证输出 4K。

### 6.2 示例

文生视频：

```json
{
  "model": "muse-video",
  "prompt": "一片金色枫叶在微风中旋转落下，近景，柔和逆光",
  "duration": 15,
  "size": "16:9"
}
```

单首帧图生视频：

```json
{
  "model": "muse-video",
  "prompt": "让画面中的人物自然微笑并轻轻挥手，保持人物外观",
  "duration": 10,
  "aspect_ratio": "9:16",
  "resolution": "auto",
  "image": {"url": "https://assets.example.com/portrait.jpg"}
}
```

Muse 按次价格通过对应分组的模型广场价卡读取。未配置 Muse 价卡时，平台不会自动套用 Omni 或 Grok 的价格。

## 7. Grok 生成参数与官方扩展

本节依据 2026-11-07 查询的 xAI 官方文档。使用 `/v1/videos` 或 `/v1/videos/generations`。网关通常保留视频 JSON 字段并转发，但自定义代理可能只实现官方功能的子集；下述高级参数尤其需要上游支持。

### 7.1 基础参数

| 参数 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `model` | string，本平台必填 | 无 | 目录返回的 Grok 视频模型 |
| `prompt` | string | 文生必填 | 官方单图/固定帧模式可省略；参考模式的指南与 REST schema 描述有差异，为兼容各上游，建议总是提供非空描述 |
| `duration` | integer / 数字字符串 | 官方 `8` | 1–15 秒；标准模型参考图模式最多 10 秒；推荐 JSON 整数 |
| `seconds` | integer / 数字字符串 | 兼容字段 | 官方接受 `duration` 别名；本网关计费解析同时提供时优先 `seconds`，建议只传 `duration` |
| `resolution` | string | `480p` | 标准模型 `480p` / `720p`；1.5 文/单图额外支持 `1080p`；参考模式最高 `720p` |
| `aspect_ratio` | string | 文生通常 `16:9` | `1:1`、`16:9`、`9:16`、`4:3`、`3:4`、`3:2`、`2:3`、`21:9`、`5:2`；官方单图模式按输入图比例，忽略此字段 |
| `image` | object | 无 | `{"url":"https://…"}` 或 `{"url":"data:image/png;base64,…"}`；图片作为首帧 |
| `reference_images` | array of objects | 无 | 每项与 `image` 同形；作为人物/风格/内容参考，不固定为首帧 |
| `input_reference` | object | 无 | 官方 `image` 兼容名；本平台没有为它单独提取素材元数据，推荐使用 `image` |
| `generate_audio` | boolean | 官方默认 `true` | `false` 请求静音；官方生成指南有此字段，所核对 OpenAPI 版本尚未列出；需上游支持 |
| `user` | string | 无 | 官方支持的终端用户标识，建议使用不含直接个人信息的内部编号 |

Grok 官方素材图片格式为 JPEG、PNG、WebP。推荐使用 `image.url`；网关兼容部分 `image_url` 写法，但不要把 Omni 的字符串数组直接用于 Grok 的 `reference_images`。

Grok 本平台已识别的计费时长为 1–15 秒。即使某个代理接受更大的值，也不要用超限值调用本接口。标准模型没有正式的 1080p 生成能力；价卡里出现 1080p 价格不代表模型本身支持该分辨率。

### 7.2 参考图片

| 模型 | 图片上限 | 时长上限 | 分辨率上限 |
| --- | --- | --- | --- |
| `grok-imagine-video` | 7 张 | 10 秒 | 720p |
| `grok-imagine-video-1.5` | 14 张 | 15 秒 | 720p |

标准模型不要将 `image` 和 `reference_images` 混合；1.5 官方允许首帧、参考图、声音、尾帧与关键帧按其规则组合。不要套用 Omni 的“首尾帧与参考素材互斥”规则。

```json
{
  "model": "grok-imagine-video",
  "prompt": "保持第一张图中的人物和第二张图中的场景，拍摄自然行走的中景镜头",
  "reference_images": [
    {"url": "https://assets.example.com/character.jpg"},
    {"url": "https://assets.example.com/location.jpg"}
  ],
  "duration": 10,
  "resolution": "720p",
  "aspect_ratio": "16:9"
}
```

### 7.3 1.5 声音参考、首尾帧与关键帧

这些字段属于**官方能力、本网关 JSON 透传**。工作台目前未为每个字段提供控件；自定义 Grok 上游若尚未升级，可能拒绝请求。

| 参数 | JSON 形状 | 限制 |
| --- | --- | --- |
| `reference_audios` | `[{"voice_id":"eve"}]` | 最多 3 项；使用官方预设声音 ID；可不传参考图片而仅使用声音参考 |
| `reference_audios[].url` | `{"url":"https://…"}` 或音频 data URI | 与 `voice_id` 互斥；自定义音频最长 15 秒，官方目前需要特定合作权限，不能默认认为可用 |
| `image` | `{"url":"…"}` | 固定首帧；与尾帧/关键帧/参考素材组合时以官方上游规则为准 |
| `last_frame` | `{"url":"…"}` | 固定尾帧；**Grok 官方允许只传尾帧**，与 Omni 必须有首帧的规则不同 |
| `keyframes` | `[{"image":{"url":"…"},"timestamp_s":2.0}]` | 最多 4 个中间关键帧；`0 < timestamp_s < duration` |
| `keyframes[].timestamp_s` | number | 官方按 1/3 秒网格对齐，不能落在同一网格槽；建议按时间升序、间隔至少 1/3 秒 |

参考提示词可用 `<IMAGE_0>`、`<IMAGE_1>` 与 `<AUDIO_0>`、`<AUDIO_1>` 关联对应素材；同时提供固定首帧 `image` 时，它占用 `<IMAGE_0>`，参考图从 `<IMAGE_1>` 起。这里的 3 是同时使用的声音参考数量，不是预设声音总数。

官方视频指南引用的 TTS 预设声音名单，在核对日期包括以下 ID（大小写不敏感）：

```text
carina, zagan, helix, orion, luna, iris, altair, zenith,
perseus, helios, lux, kepler, rigel, cosmo, celeste, ursa,
sirius, lumen, castor, naksh, atlas, aurora, liora,
ara, eve, leo, rex, sal
```

实际可用于视频的集合受模型、上游版本与权限控制；官方遇到无效 ID 时会返回 400 并列出可用声音。未传 `reference_audios` 表示由模型生成音频，不代表默认指定了某个预设声音。

首尾帧与中间关键帧示例：

```json
{
  "model": "grok-imagine-video-1.5",
  "prompt": "镜头在三个构图之间平滑过渡，保持人物与环境一致",
  "image": {"url": "https://assets.example.com/start.jpg"},
  "keyframes": [
    {"image": {"url": "https://assets.example.com/middle.jpg"}, "timestamp_s": 4.0}
  ],
  "last_frame": {"url": "https://assets.example.com/end.jpg"},
  "duration": 8,
  "resolution": "720p",
  "aspect_ratio": "16:9"
}
```

参考图与预设声音示例：

```json
{
  "model": "grok-imagine-video-1.5",
  "prompt": "<IMAGE_0>中的人物面对镜头，用<AUDIO_0>的声音说：欢迎来到我们的工作室。",
  "reference_images": [{"url": "https://assets.example.com/person.jpg"}],
  "reference_audios": [{"voice_id": "eve"}],
  "duration": 8,
  "resolution": "720p",
  "aspect_ratio": "9:16"
}
```

Grok 默认可依据提示词生成声音和对白；需要静音可在支持该能力的上游使用 `"generate_audio": false`。没有核实到通用 `audio`、`fps`、`seed`、`negative_prompt`、`watermark` 控制项；不要假定其他视频服务的同名参数适用。

### 7.4 官方文件与输出存储参数：仅作进阶参考

| 字段 | 官方含义 | 本平台使用限制 |
| --- | --- | --- |
| `image.file_id` / 参考图 `file_id` | 使用 xAI Files API 的已上传图片；与 `url` 互斥 | 当前视频调用文档不提供稳定的跨账户 Files 工作流。文件必须属于实际被调度的上游账户；推荐使用 URL/data URI |
| `video.file_id` | 编辑/延长使用 xAI Files API 的已上传视频；与 `video.url` 互斥 | 与图片 `file_id` 有相同的账户归属限制，常规示例使用 `video.url` |
| `output.upload_url` | 上游将结果 HTTP PUT 到签名 URL | 可能改变上游返回与存储方式；本平台下载/保存路径依赖可取回的视频结果，未经站点验证不要用于常规任务 |
| `storage_options.filename` | 官方 Files 存储文件名，提供该对象时必填 | 同上，不等于本平台下载文件名 |
| `storage_options.expires_after` | 官方文件自动过期秒数，最大 2592000（30 天）；省略不自动过期 | 不控制本平台任务历史保留期 |
| `storage_options.public_url` | `true` 生成公开 URL，`false`/省略不生成；也可为 `{"expires_after":3600}` | 对象有效期 3600–2592000 秒；省略有效期时继承官方文件 TTL 或永久。不是本平台公开下载开关 |

这些字段可被视频 JSON 转发路径保留，但当前平台没有把它们作为经过端到端验证的存储功能提供。无需使用它们即可通过 `/content` 完成下载。

## 8. Grok 视频编辑与延长

这两个接口使用 `grok-imagine-video`，需要 Grok 分组或实际路由到 Grok 的配置以及上游支持。Muse/Omni 的参考视频不等于这两个端点。

### 8.1 编辑：`POST /v1/videos/edits`

| 参数 | 类型 | 要求 |
| --- | --- | --- |
| `model` | string | `grok-imagine-video` |
| `prompt` | string | 编辑要求；说明需要改变和保留的内容 |
| `video` | object | 推荐 `{"url":"https://…/source.mp4"}`；官方也支持视频 data URI |
| `user` | string | 可选用户标识 |
| `output` / `storage_options` | object | 官方输出存储扩展，嵌套参数与第 7.4 节相同；本平台常规调用不建议使用 |

官方输入最长 **8.7 秒**，MP4，支持 H.264/H.265/AV1 等编码。输出继承输入时长、画幅与分辨率，最高 720p。官方编辑接口**不支持**指定 `duration`、`aspect_ratio`、`resolution`；不要套用生成参数。

```bash
curl --fail-with-body -sS "$SUB2API_BASE_URL/v1/videos/edits" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H 'Content-Type: application/json' \
  --data '{
    "model": "grok-imagine-video",
    "prompt": "把背景改成雪山，保留人物、动作和镜头运动",
    "video": {"url": "https://assets.example.com/source.mp4"}
  }'
```

### 8.2 延长：`POST /v1/videos/extensions`

| 参数 | 类型 | 要求 |
| --- | --- | --- |
| `model` | string | `grok-imagine-video` |
| `prompt` | string | 描述接下来发生什么 |
| `video` | object | 推荐 `{"url":"https://…/source.mp4"}`；输入长度 2–15 秒 |
| `duration` | integer | 延长部分 2–10 秒，官方默认 6；建议明确传入 |
| `output` / `storage_options` | object | 官方输出存储扩展，见第 7.4 节 |

输出保留原视频并拼接延长部分；例如输入 10 秒、`duration=5`，总输出为 15 秒。画幅与分辨率继承输入，最高 720p，不能指定 `aspect_ratio` 或 `resolution`。所核对的官方延长 schema 未列 `user`，不要直接复用编辑请求的全部字段。

```json
{
  "model": "grok-imagine-video",
  "prompt": "人物走出画面，镜头继续向上移动，展示远处的山峰",
  "video": {"url": "https://assets.example.com/source.mp4"},
  "duration": 5
}
```

**当前计费限制**：编辑和延长虽有透传路由，但共用的报价接口只接受模型名，没有“编辑/延长”、源视频长度或源分辨率参数。持久化任务计费主要保留提交时的生成参数，不能据此保证报价与官方编辑/延长的输入时长、总输出时长口径一致。按秒使用这两个操作前，应由站点确认计费规则；不要自行补官方不支持的参数来调整账单。常规文生/图生按第 10 节计价。

## 9. 任务状态、下载与历史

### 9.1 完整调用顺序

```mermaid
sequenceDiagram
    participant C as 用户客户端
    participant S as 本平台
    C->>S: GET /v1/models?video_studio=1
    C->>S: GET /v1/videos/pricing?model=...
    C->>S: POST /v1/videos
    S-->>C: id / request_id
    loop 每约5秒查询
        C->>S: GET /v1/videos/{id}
        S-->>C: pending / done / failed 等
    end
    C->>S: GET /v1/videos/{id}/content
    S-->>C: 视频二进制
```

创建响应可能仅包含 ID，也可能包含状态：

```json
{"id":"task_example","request_id":"task_example","object":"video.generation.task","status":"pending"}
```

Grok 原生创建常见形状：

```json
{"request_id":"task_example"}
```

### 9.2 查询状态

```bash
export VIDEO_ID='这里填创建返回的任务ID'
curl --fail-with-body -sS "$SUB2API_BASE_URL/v1/videos/$VIDEO_ID" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

建议约 5 秒查询一次，并设整体等待上限，例如 10–20 分钟。达到客户端等待上限不代表任务已失败或取消，可稍后用同一 ID 恢复查询。

| 类别 | 客户端应兼容的状态 | 下一步 |
| --- | --- | --- |
| 处理中 | `pending`、`queued`、`processing`、`in_progress`、`running` | 继续查询 |
| 生成完成 | `done`、`completed`、`succeeded`、`success` | 调用 `/content` 下载 |
| 终止失败 | `failed`、`expired`、`cancelled`、`canceled`、`error` | 读取 `error`，停止轮询 |

Muse 与 Omni 的适配响应通常归一为 `pending/done/failed`；本地持久化结果和历史使用 `processing/completed/failed`。Grok 原生可能返回 `pending/done/expired/failed`。不要只判断单一的 `completed`。

完成示例：

```json
{
  "id": "task_example",
  "request_id": "task_example",
  "status": "done",
  "video": {"url": "/v1/videos/task_example/content"}
}
```

`progress`、`created_at`、`completed_at`、`model` 等字段取决于上游，不保证每次都有。时间戳为 Unix 秒。任务 ID 与响应头中的 `X-Request-Id` 不同：前者用于视频查询，后者用于单次 HTTP 请求排错。

| 查询响应字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` / `request_id` | string | 同一任务的标识；Grok 原生响应可能只返回 `request_id` |
| `status` | string | 按上述状态表处理 |
| `progress` | number，可选 | 上游提供的进度，不保证持续更新 |
| `video.url` | string，可选 | 完成后的地址，可能是本平台相对路径，也可能是上游地址；统一下载方式仍是 `/content` |
| `video.duration` | integer，可选 | Grok 上游可能提供的输出时长；Muse/Omni 不保证返回 |
| `video.respect_moderation` | boolean，可选 | Grok 上游内容审核结果字段；不是可由客户端关闭审核的请求参数 |
| `error` | object 或 string，可选 | 失败原因；对象常含 `code`、`message` |
| `created_at` / `completed_at` | integer，可选 | Unix 秒，字段是否出现取决于响应路径 |

不得把没有 `video.url` 的未知成功结构当成已下载；应调用内容接口并核对成功状态及视频内容。

### 9.3 下载视频

```bash
curl --fail-with-body -sS \
  "$SUB2API_BASE_URL/v1/videos/$VIDEO_ID/content" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H 'Accept: video/mp4' \
  --output result.mp4
```

请确认 HTTP 成功状态及响应 `Content-Type`，不要把错误 JSON 当成 MP4。示例客户端已处理这一点。

- 统一调用固定的本网关 `/content` 路径，不要把本平台 Key 发送到上游返回的其他域名。
- 首次下载可能同时拉取、保存或转换视频，因此会比普通文件下载耗时更长。一般按 MP4 接收，最终格式以 `Content-Type` 为准。
- `Range` 断点读取依赖已保存内容；首次下载可能返回完整 200，后续可能返回 206，不保证第一次就按范围返回。
- 当前保存流程限制视频结果为 512 MiB；管理员还可能配置代理、请求体及超时限制。
- `done` 表示生成完成；`content_available` 表示已保存可供访问的内容。两者不必同时出现，生成完成后仍应调用 `/content`。

### 9.4 历史记录

```bash
curl --fail-with-body -sS "$SUB2API_BASE_URL/v1/videos/tasks?limit=10" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

`limit` 默认 10，最大 50；小于等于 0 时按 10。当前没有基于游标的翻页参数。

```json
{
  "object": "list",
  "data": [{
    "id": "task_example",
    "request_id": "task_example",
    "object": "video.generation.task",
    "status": "completed",
    "created_at": 1794000000,
    "completed_at": 1794000060,
    "expires_at": 1794604800,
    "metadata": {
      "operation": "text",
      "model": "gemini-omni-1.2-flash",
      "prompt": "雨后的森林",
      "resolution": "720p",
      "aspect_ratio": "16:9",
      "duration": 5,
      "seed": 0
    },
    "content_available": true,
    "content_type": "video/mp4",
    "byte_size": 1234567,
    "browser_playable": true,
    "billing_status": "charged"
  }],
  "retention_days": 7,
  "persistent_history": true
}
```

以上日期和文件大小仅为示例。保留期读取实际 `retention_days`，默认配置为 7 天；`persistent_history=true` 表示持久化历史可用，不代表上游 URL 或视频文件永久有效。历史只保留基本生成参数及 Omni seed，不保存可复用的全部参考素材。

`billing_status` 可能为 `pending`、`charged`、`not_charged`、`billing_failed`。列表读取本地记录，不等于主动向上游轮询所有任务；处理中任务应继续请求其状态端点。

### 9.5 清空历史

```http
DELETE /v1/videos/tasks
Authorization: Bearer YOUR_SUB2API_KEY
```

成功返回 `204 No Content`。这会清空当前 Key 的历史列表；持久化模式下主要是隐藏记录，非持久化模式会清理缓存记录。**它不是取消生成、退款或删除上游视频的接口。** 当前统一视频 API 未提供 `DELETE /v1/videos/{id}` 取消任务协议。

## 10. 计费规则

1. 以当前 Key、模型和分辨率的 `/v1/videos/pricing` 响应为准，不写死上游官网单价或截图价格。
2. 按次模式：预估费用 = `unit_price × 任务数量`，不乘视频时长。
3. 按秒模式：常规生成预估费用 = `unit_price × 提交秒数 × 任务数量`。Muse 的秒数是期望时长，建议以站点配置的按次价卡为准。
4. `unit_price` 已含报价倍率。视频独立倍率、用户专属倍率、渠道价卡等由服务端处理，客户端不要重复计算。
5. 普通新建任务接收成功不代表已扣生成费；当前持久化任务通常在视频保存、可播放后结算。旧任务或非持久化路径可能在首次确认完成时结算。
6. 同一任务重复查询、下载使用已有去重逻辑，不应产生新的生成费用。重新 POST 创建新任务则属于新的一次生成。
7. 普通生成失败通常不收生成费；**Grok 明确的内容政策拒绝有单独规则**：当前代码对相应上游拒绝收取固定 `$0.10`，即使没有视频输出也可能产生费用，最终以使用记录为准。不要把所有失败一概理解成免费。
8. 报价用于提交前预估，不是锁价凭证。服务端任务保留计费模型等元数据，实际费用查看用量记录；编辑/延长的特殊口径见第 8 节。

## 11. 可运行客户端与 SDK

附带两个完整示例：

- [Python 3.10+ 标准库客户端](video-api-examples/video_client.py)
- [Node.js 20+ 客户端](video-api-examples/video_client.mjs)
- [Omni 请求示例 JSON](video-api-examples/payload.json)

客户端包含创建、轮询、失败处理、视频下载和断线后继续查询。不会自动重试 POST 创建；会打印任务 ID，之后可恢复轮询。默认每 5 秒轮询，GET 暂时故障最多尝试 3 次，`--timeout` 默认 900 秒，覆盖这次创建/查询/下载的整体等待预算。单次普通请求上限 120 秒，首次内容下载可等待最多 600 秒，均受整体剩余预算限制。

脚本以 `/v1/videos` 创建常规生成任务，不提供编辑/延长的创建选项；这两个操作可先用对应 HTTP 端点创建，再使用 `--task-id` 继续查询。下载先写临时文件，确认 MP4/WebM 内容和完整长度后再替换输出文件。

### 11.1 Python

```bash
export SUB2API_BASE_URL='https://api.example.com'
export SUB2API_API_KEY='YOUR_SUB2API_KEY'
python video_client.py --payload payload.json --output result.mp4
```

恢复已有任务，不重新创建：

```bash
python video_client.py --task-id 'task_example' --output result.mp4
```

Windows PowerShell：

```powershell
$env:SUB2API_BASE_URL = 'https://api.example.com'
$env:SUB2API_API_KEY = 'YOUR_SUB2API_KEY'
python .\video_client.py --payload .\payload.json --output .\result.mp4
```

### 11.2 Node.js

```bash
export SUB2API_BASE_URL='https://api.example.com'
export SUB2API_API_KEY='YOUR_SUB2API_KEY'
node video_client.mjs --payload payload.json --output result.mp4
```

```bash
node video_client.mjs --task-id 'task_example' --output result.mp4
```

### 11.3 本地素材转 data URI

Python：

```python
import base64
from pathlib import Path

image_data_uri = "data:image/png;base64," + base64.b64encode(
    Path("first.png").read_bytes()
).decode("ascii")
# Omni: payload["first_frame"] = image_data_uri
# Muse: payload["image"] = image_data_uri
# Grok: payload["image"] = {"url": image_data_uri}
```

MIME 类型须与实际文件一致。Base64 会使请求体比原文件增大约三分之一，多素材请求还受站点整体请求体大小限制。

### 11.4 OpenAI / xAI SDK 兼容注意

上游示例里的 `client.videos.create()`、`retrieve()`、`download_content()` 不一定可以原样用于三个模型：SDK 可能限制 `seconds` 枚举、采用 multipart、过滤扩展字段或只认识 `completed` 状态。

- 本文推荐 JSON HTTP 客户端，避免 SDK 模型类型限制。
- Muse 的 `seconds` 必须是数字；Omni 接受数字字符串；不要共用未经转换的 SDK 默认请求体。
- xAI SDK 的 `image_url`、`last_frame_url`、`video_url` 或 Vercel SDK 的 `aspectRatio`、`frameImages`、`providerOptions` 属于 SDK 封装参数，**不是本文 HTTP JSON 的字段名称**。
- 使用 SDK 时须确认其 `base_url` 指向本平台、请求实际发往 `/v1/videos` 或对应路径、字段完整保留，并兼容本文任务状态。
- `pollTimeoutMs`、`poll_interval` 等通常属于客户端轮询设置，不应当作模型请求参数发送。

## 12. 错误码与排查

不同中间件或上游可能采用不同错误外层结构，客户端应至少兼容：

```json
{"error":{"type":"pricing_not_configured","message":"Configure a per-request or per-second price for this video model before generating"}}
```

```json
{"code":"API_KEY_REQUIRED","message":"API key is required"}
```

| HTTP / 类型 | 常见原因 | 处理 |
| --- | --- | --- |
| 400 `invalid_request_error` | 时长越界、缺 prompt、尺寸冲突、首尾帧与参考模式混用 | 按对应模型参数表修正 |
| 400 `api_key_in_query_deprecated` | 把 Key 放在查询参数中 | 改为 Bearer 请求头 |
| 401 | Key 缺失、无效或被停用 | 检查请求头和 Key 状态 |
| 403 `permission_error` | 分组无媒体权限、模型不在允许列表 | 检查 Key 所属分组及模型权限 |
| 403 `API_KEY_EXPIRED` 等 | Key 已过期或额度限制 | 检查 Key 有效期、额度和账户状态 |
| 404 `not_found_error` | 任务不存在、不属于当前 Key、路由/平台不支持 | 检查任务 ID、原 Key 及后台版本 |
| 404，报价路径返回 `Video request not found` / `Videos API is not supported…` | 前端已更新、后端仍为不包含报价接口的旧版本 | 更新后端；不是未配置价格 |
| 409 `video_not_ready` | 内容尚不能下载 | 继续查询原任务 |
| 413 | JSON/base64 总请求体超过站点限制 | 减少素材体积或改用公开 URL |
| 429 | 并发/速率或上游限流 | 遵循 `Retry-After`，降低并发；GET 可稍后重试 |
| 503 `pricing_not_configured` / `video_pricing_unavailable` | 当前分组的模型或选定档位未配置价格 | 在对应模型广场价卡配置后重新报价 |
| 503 `*_no_eligible_account` | 没有可调度的相应上游账户 | 联系站点检查账户、绑定、余额、模型和能力配置 |
| 502 / 5xx | 上游异常、内容拉取、存储或转码失败 | 保留任务 ID、HTTP `X-Request-Id` 和错误信息；先查已有任务 |

Grok 已受理任务的异步 `error.code` 还可能为 `invalid_argument`、`permission_denied`、`failed_precondition`、`service_unavailable`、`internal_error`。这些是任务结果中的错误，不一定对应同名 HTTP 状态；应同时记录 HTTP 状态和 JSON 错误。

请求创建超时不等于上游没有创建任务。不要直接无限重发 POST；优先检查任务历史、已有 ID 或联系站点核实。状态查询临时失败也不等于任务最终失败。

## 13. 兼容性与未支持能力

| 能力 | 当前支持情况 |
| --- | --- |
| 异步创建、状态轮询、二进制下载 | 已支持 |
| 文生、图生、Omni 首尾帧/多素材 | 按模型参数表支持 |
| 多个并行任务 | 可多次创建，受用户/账户并发与余额限制；每条单独计费 |
| Omni 上游 `include_result=1` | 当前适配未接入，使用显式轮询 |
| Omni 上游 `/stream` / SSE | 未提供统一视频流式状态接口 |
| `X-Fal-Webhook` / 回调 URL | Omni 当前适配不透传此回调协议 |
| `Idempotency-Key` 防重复创建 | 当前 Omni/Muse 未提供上游幂等保证，不应依赖它自动重试收费 POST |
| 一次请求 `n>1` | Omni/Muse 拒绝；Grok 官方生成 schema 未列批量 n，统一使用多任务 |
| 通用 `stream`、`fps`、`negative_prompt`、`watermark` | 不属于本平台三类模型的统一支持参数 |
| `POST /chat/completions` 输出视频 | 本指南不提供此工作流，视频任务走 `/v1/videos` |
| 取消单个统一视频任务 | 未提供；清空历史不是取消 |
| 原样传递所有未知字段 | Omni/Muse 会重建请求，未知字段可能被丢弃；Grok 一般透传但不保证上游接受 |
| 任意模型名 | 不支持；使用当前 Key 的目录和管理员已配置的映射 |

部署在特定第三方 Grok 代理上时，平台可能转换 `duration → seconds`、`image → image_url`，并缩减该代理不支持的比例/分辨率。用户优先发送本文标准字段；上游能力不完整时，以站点确认的能力为准。

本指南没有把另一套 Seedance/Ark 的 `/contents/generations/tasks` 协议混入 `/v1/videos`。其 `content` 数组、任务删除和计费语义不同，需要独立接入说明。

## 14. 参考来源

### 上游与官方资料

核对日期：2026-11-07。官网可能继续变化；本文的平台接口行为以当前适配实现为准。

1. Gemini Omni：上游提供的两张参数表和 OpenAI 风格 `/v1/videos` 示例。参数表 `duration` 为 3–15，教程正文另写 3–10；本平台按明确要求和参数表采用 **3–15 秒**。该材料不是 Google 官方 Veo 规格。
2. Muse：[muse2api 上游仓库](https://github.com/www222fff/muse2api)，接入核对版本 [`3a20b5e7aa4056c99c0eb8c732d73ce3804d5dbf`](https://github.com/www222fff/muse2api/tree/3a20b5e7aa4056c99c0eb8c732d73ce3804d5dbf)，包含 schema、视频路由、任务与媒体实现。
3. xAI：[Video Overview](https://docs.x.ai/developers/model-capabilities/video/overview)。
4. xAI：[Video Generation](https://docs.x.ai/developers/model-capabilities/video/generation)。
5. xAI：[Image-to-Video](https://docs.x.ai/developers/model-capabilities/video/image-to-video)。
6. xAI：[Reference-to-Video、参考声音、首尾帧与关键帧](https://docs.x.ai/developers/model-capabilities/video/reference-to-video)。
7. xAI：[Video Editing](https://docs.x.ai/developers/model-capabilities/video/editing)。
8. xAI：[Video Extension](https://docs.x.ai/developers/model-capabilities/video/extension)。
9. xAI：[Videos REST API](https://docs.x.ai/developers/rest-api-reference/inference/videos) 与 [OpenAPI Schema](https://docs.x.ai/openapi.json)。
10. xAI：[预设声音列表](https://docs.x.ai/developers/model-capabilities/audio/text-to-speech#voices)。

xAI 最新能力页已经说明 1.5 支持文生和图生；不能继续把旧工作台“1.5 只在图生选项中优先选择”的行为当作官方永久能力限制。官方 REST schema 与能力页也可能有更新差异，例如固定尾帧在能力页明确介绍；使用高级能力时应同时核对所选上游版本。

### 平台实现核对入口（供维护者）

- [视频路由](../backend/internal/server/routes/gateway.go)
- [Omni 参数与上游适配](../backend/internal/service/gemini_omni_video.go)
- [Muse 参数与上游适配](../backend/internal/service/muse_video.go)
- [Grok 转发与兼容处理](../backend/internal/service/grok_media.go)
- [报价与价卡解析](../backend/internal/service/video_studio_pricing.go)
- [历史与下载接口](../backend/internal/handler/video_task.go)
- [任务持久化与状态](../backend/internal/service/video_task.go)

本文件及示例不包含真实 Key。示例验证使用本地模拟服务，不会提交真实收费视频任务。
