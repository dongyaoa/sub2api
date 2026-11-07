# Muse Video API 用户调用文档

本文介绍通过本站 Sub2API 调用 `muse-video` 的完整方式：查询模型和价格、文生视频、首帧图生视频、查询进度、下载视频、读取及清空历史。

参数核对日期：**2026-11-07**。本文以本站实际适配代码和 muse2api 上游源码为准。`muse-video` 是 muse2api 暴露的模型标识；兼容 OpenAI 风格的路径和鉴权，并不代表支持其他视频模型的全部字段。

## 目录

1. [地址、鉴权与能力概览](#1-地址鉴权与能力概览)
2. [完整接口清单](#2-完整接口清单)
3. [查询可用模型和价格](#3-查询可用模型和价格)
4. [创建视频：全部参数](#4-创建视频全部参数)
5. [文生视频与图生视频示例](#5-文生视频与图生视频示例)
6. [任务状态、响应字段与轮询](#6-任务状态响应字段与轮询)
7. [下载、播放与保存](#7-下载播放与保存)
8. [历史记录与清空](#8-历史记录与清空)
9. [计费、并发、超时与重试](#9-计费并发超时与重试)
10. [错误码与排查](#10-错误码与排查)
11. [完整 Python 示例](#11-完整-python-示例)
12. [完整 Node.js 示例](#12-完整-nodejs-示例)
13. [未支持的功能与参数](#13-未支持的功能与参数)
14. [本站、上游及官方资料对照](#14-本站上游及官方资料对照)

## 1. 地址、鉴权与能力概览

本文沿用本站现有用户文档中的域名；私有部署请把地址替换为自己的 Sub2API 域名。

| 项目 | 值 |
| --- | --- |
| API Base URL | `https://api.zicc.cc/v1` |
| 推荐创建地址 | `POST https://api.zicc.cc/v1/videos/generations` |
| 兼容创建地址 | `POST https://api.zicc.cc/v1/videos` |
| 模型 | `muse-video` |
| 鉴权 | `Authorization: Bearer YOUR_SUB2API_KEY` |
| 创建请求格式 | `Content-Type: application/json` |
| 普通响应格式 | JSON |
| 下载响应格式 | 视频二进制，正常为 `video/mp4` |

使用**本站用户 API Key**，不要使用网页登录 Token、管理员 Key 或 muse2api 上游 Key。Key 所属分组需要开放该模型和媒体生成功能。创建、查询、下载必须使用提交任务时的同一个 Key；同一用户的另一个 Key 也不能读取该任务。

鉴权层也兼容 `x-api-key: YOUR_SUB2API_KEY` 和 `x-goog-api-key: YOUR_SUB2API_KEY`；新客户端统一使用 Bearer 即可，不要同时发送互相冲突的凭据。查询参数 `?key=`、`?api_key=` 不支持，会被拒绝。查询和下载仍会检查 Key 是否有效、用户/分组状态及 IP 权限，不能认为“只要保存了任务 ID 就可以匿名取回视频”。

| 功能 | 当前支持情况 |
| --- | --- |
| 文生视频 | 支持；填写 `prompt`，不提交参考图 |
| 首帧图生视频 | 支持；一张图片，同时填写 `prompt` |
| 时长 | 期望时长为整数 **1–60 秒**；本站 API 省略时使用 **5 秒** |
| 画幅比例 | 支持比例提示；常用 `16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`3:2`、`2:3` |
| 分辨率 | 自动决定；不提供固定 480p、720p、1080p、2K、4K 控制 |
| 每次数量 | 一个视频；`n` 只能为 `1` |
| 异步执行 | 支持；创建后返回任务 ID，再查询状态和下载 |
| 价格 | 由本站模型广场对应价卡决定，支持按次或按请求秒数计费 |
| 多任务 | 可以分别创建多个任务；并发数量受站点及分组限制 |

**时长和画幅是生成意图，不是输出保证。** 当前上游浏览器驱动把它们加入提示词。60 秒是参数上限，不能理解成所有任务都能生成恰好 60 秒的视频。没有官方依据证明本接口能固定输出 4K；把“4K”写进提示词或传入 `size="3840x2160"` 都不能建立 4K 输出保证。

## 2. 完整接口清单

以下路径均相对于 Base URL，统一使用 `/v1` 前缀即可。

| 方法 | 路径 | 用途 | 正常 HTTP 状态 |
| --- | --- | --- | --- |
| GET | `/models?video_studio=1` | 当前 Key 可用的视频模型列表 | `200` |
| GET | `/videos/pricing?model=muse-video` | 当前 Key 的模型报价 | `200` |
| POST | `/videos/generations` | 创建文生视频或首帧图生视频任务 | `200` |
| POST | `/videos` | 创建任务的兼容入口；与上一行二选一 | `200` |
| GET | `/videos/{id}` | 查询一个任务 | `200` |
| GET | `/videos/{id}/content` | 下载或播放一个任务的视频 | `200`，已存储文件的范围请求可能为 `206` |
| GET | `/videos/tasks?limit=10` | 查询当前 Key 的视频历史 | `200` |
| DELETE | `/videos/tasks` | 清空/隐藏当前 Key 的全部视频历史 | `204`，无响应体 |

本站还兼容 `/videos/generations/{id}` 查询及 `/videos/generations/{id}/content` 下载，以及无 `/v1` 前缀的同名视频路径。建议新客户端只使用上表，避免重复拼接 `/v1/v1`。

`{id}` 是创建响应中的 `request_id` 或 `id`，视为不透明字符串并进行 URL 编码。`/videos/tasks` 和 `/videos/pricing` 是固定路径，不是任务 ID。

## 3. 查询可用模型和价格

### 3.1 模型列表

```bash
export SUB2API_BASE_URL="https://api.zicc.cc/v1"
export SUB2API_API_KEY="YOUR_SUB2API_KEY"

curl --fail-with-body "$SUB2API_BASE_URL/models?video_studio=1" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

响应包含 `object: "list"` 和 `data` 数组，每个模型至少有 `id`、`object`；其他元数据以实际响应为准。检查是否存在 `id == "muse-video"`。列表中没有该模型时，请检查分组权限或联系站点管理员。

普通 `/models` 也可列模型，但可能受站点模型清单配置影响；视频客户端推荐使用 `?video_studio=1`。

### 3.2 价格查询

```bash
curl --fail-with-body "$SUB2API_BASE_URL/videos/pricing?model=muse-video" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

按次价格的**示例响应，金额仅用于说明，不代表本站实际售价**：

```json
{
  "model": "muse-video",
  "billing_model": "muse-video",
  "billing_mode": "per_request",
  "pricing_source": "group",
  "rate_multiplier": 0.5,
  "tiers": {
    "auto": {
      "base_price": 0.4,
      "unit_price": 0.2,
      "configured": true
    }
  }
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 查询的公开模型 ID |
| `billing_model` | string | 实际选用的计费模型；存在渠道映射时可能不同，不要据此替换请求中的 `model` |
| `billing_mode` | string | Muse 支持 `per_request`（每次）或 `video`（每秒） |
| `pricing_source` | string | 价格来源，例如 `group` 或 `channel` |
| `rate_multiplier` | number | 该报价采用的有效倍率 |
| `tiers.auto.base_price` | number | 倍率前的单位价格，美元 |
| `tiers.auto.unit_price` | number | 已应用倍率的单位价格，美元；客户端直接用这个数估价 |
| `tiers.auto.configured` | boolean | 是否来自已配置价格；明确配置为零也属于有效报价 |

计算方式：

- `per_request`：一条生成任务的预估费用 = `unit_price`，不乘时长。
- `video`：预估费用 = `unit_price × duration`；省略时长时按本站默认 5 秒估算。
- `unit_price` 已包含倍率，客户端不要再次乘 `rate_multiplier`。
- `auto` 是自动输出这一档的名字，不是 480p 的别名。
- 价格未配置时查询返回错误，不能把“查询失败”当作免费。

## 4. 创建视频：全部参数

### 4.1 JSON 字段总表

文生视频与图生视频使用同一个 POST 接口。除下表外，没有其他已经接通的 Muse 视频生成参数。

| 字段 | 类型 | 必填 | 省略时行为 | 可用值、限制及作用 |
| --- | --- | --- | --- | --- |
| `model` | string | 是 | 不允许省略 | 使用 `muse-video`。本站识别时去除首尾空白、不区分大小写，建议固定小写 |
| `prompt` | string | 是 | 不允许省略 | 非空提示词；去除首尾空白后仍须有内容。文生视频、图生视频都必填。无单独公开的字符上限，仍受请求体大小和上游限制 |
| `duration` | integer | 否 | 与 `seconds` 均未提供时为 `5` | **1–60**，单位秒，表示期望时长；必须传 JSON 数字，不支持 `"10"`、`2.5`、`0`、`61` |
| `seconds` | integer | 否 | 使用 `duration` 或默认 5 秒 | `duration` 的兼容别名，范围同为 1–60；同时提供时 `duration` 优先 |
| `aspect_ratio` | string | 否 | 不发送画幅提示，由上游决定 | 本站统一字段；推荐七种常用比例。最终采用的画幅字符串最多 16 个 Unicode 字符 |
| `size` | string | 否 | 回退 `aspect_ratio` | 上游原生画幅提示字段，优先于 `aspect_ratio`；建议填比例。不是可靠的输出分辨率开关 |
| `resolution` | string | 否 | 自动决定 | 仅支持 `"auto"`；可省略。固定分辨率如 `"720p"`、`"1080p"`、`"4k"` 会被拒绝 |
| `image` | string 或 object | 图生视频时提供一个图片字段 | 无图片则文生视频 | 单张首帧；支持图片 data URL、公开 HTTP(S) URL、纯 base64，或下文的兼容对象格式；不支持数组 |
| `input_reference` | string 或 object | 图生视频时提供一个图片字段 | 回退 `image`，都没有则文生视频 | 首帧图片别名；与 `image` 使用同样格式。实际优先采用 `image` |
| `n` | number | 否 | `1` | 只能为 `1`，一次创建一条任务；批量需求应分别 POST |

可选字段建议直接省略，不依赖 `null`。当前解析器将 `duration`、`seconds`、`size`、`aspect_ratio`、`resolution`、`image`、`input_reference` 的 `null` 视作未提供；`n: null` 不合法。`resolution: ""` 也会作为自动输出处理，建议始终用 `"auto"` 或省略。

### 4.2 字段优先级和冲突规则

| 目的 | 选择顺序 | 注意事项 |
| --- | --- | --- |
| 时长 | 非 null 的 `duration` → 非 null 的 `seconds` → `5` | 两个字段都出现时，二者都必须合法；不能用合法 `duration` 掩盖非法 `seconds` |
| 画幅 | 去空白后非空 `size` → 去空白后非空 `aspect_ratio` → 不指定 | 两字段都应是字符串或 null；采用的值最多 16 字符 |
| 首帧 | 非空 `image` → 非空 `input_reference` → 不指定 | 两者都出现时仍会验证各自格式；建议只提供一种 |

例如 `duration: 10, seconds: 8` 使用 10 秒；`size: "9:16", aspect_ratio: "16:9"` 使用竖屏 `9:16`。API 省略画幅时不强制 `16:9`；工作台的界面初始值与 API 默认值是两回事。

### 4.3 画幅和分辨率

| 推荐画幅 | 常见用途 |
| --- | --- |
| `16:9` | 横屏视频 |
| `9:16` | 竖屏短视频 |
| `1:1` | 方形展示 |
| `4:3` | 传统横屏画幅 |
| `3:4` | 传统竖屏画幅 |
| `3:2` | 横向摄影画幅 |
| `2:3` | 纵向摄影画幅 |

上述比例是工作台提供的常用值，不是上游声明的强制枚举。API 原生 `size` 可以接受其他不超过 16 字符的提示，例如 `21:9`；“参数通过校验”不等于上游保证该比例。

`size: "1080x1920"` 也能作为字符串被接受，但当前上游把它加入 `aspect ratio …` 提示语，并不据此锁定像素数。不要把这个写法宣传成 1080p，更不能据此宣传 4K。真实宽高、时长应从下载后的视频文件读取。

### 4.4 首帧图片的全部兼容形式

推荐两种字符串形式：

```json
{
  "image": "https://your-public-cdn.example/first-frame.png"
}
```

```json
{
  "image": "data:image/png;base64,iVBORw0KGgo..."
}
```

省略号仅为说明，实际请求必须是完整且有效的图片编码。

本站还兼容以下对象形式；后端会提取 URL 并转成上游需要的字符串：

| 写法 | 是否支持 |
| --- | --- |
| `"image": {"url": "https://…"}` | 支持，工作台采用的格式 |
| `"image": {"image_url": "https://…"}` | 支持 |
| `"image": {"image_url": {"url": "https://…"}}` | 支持 |
| `"input_reference": "data:image/jpeg;base64,…"` | 支持 |
| `"input_reference": {"url": "https://…"}` | 支持；该对象兼容由本站提供 |
| `"image": "完整纯base64"` | 上游支持，默认按 PNG 解码；更推荐带正确 MIME 的 data URL |
| 顶层 `"image_url": "https://…"` | 不支持，不会被当成首帧 |
| `"image": ["https://…"]` | 不支持，单张也不能用数组 |
| `"images": […]` / `"reference_images": […]` | 不支持，会被拒绝，空数组也不例外 |

图片要求及边界：

- 推荐 PNG、JPEG、WebP；工作台允许这三种格式，每张不超过 **20 MiB**。
- 上游通过 HTTP(S) 下载参考图时限制 **20 MiB**，最多处理 5 次重定向，图片读取默认超时 20 秒。
- 图片 URL 必须公开可访问，不需要额外登录或 Authorization。上游会拒绝解析到内网、环回等非公开地址的 URL。
- 本地文件路径、`file://`、浏览器 `blob:` 地址不能作为图片 URL。请把本地文件转成 data URL。
- data URL/纯 base64 还受本站整体请求体大小限制。base64 会使数据体积约增加三分之一；不能把“原图 20 MiB”理解为所有部署都能接收编码后的 JSON。
- 本接口没有提供可信的最小宽高、强制像素尺寸或图片与输出画幅必须一致的官方约束表。建议使用清晰、比例接近期望视频的首帧；这是输入建议，不是硬性参数规则。

## 5. 文生视频与图生视频示例

### 5.1 最小文生视频

```json
{
  "model": "muse-video",
  "prompt": "枫叶在微风中缓缓飘落，镜头缓慢向前推进。"
}
```

该请求使用本站默认期望时长 5 秒，不指定画幅或固定分辨率。

### 5.2 显式指定参数

```bash
curl --fail-with-body "$SUB2API_BASE_URL/videos/generations" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "muse-video",
    "prompt": "海边日落，浪花轻拍礁石，镜头平稳缓慢横移。",
    "duration": 10,
    "aspect_ratio": "16:9",
    "resolution": "auto",
    "n": 1
  }'
```

### 5.3 单张首帧图生视频

```bash
curl --fail-with-body "$SUB2API_BASE_URL/videos/generations" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "muse-video",
    "prompt": "保持主体外观与构图，让头发和衣角随风轻轻摆动，镜头缓慢推近。",
    "duration": 8,
    "aspect_ratio": "9:16",
    "image": {"url": "https://your-public-cdn.example/first-frame.png"}
  }'
```

将示例图片地址替换成自己的公开图片。图生视频没有单独的 `mode` 开关：存在有效的 `image` / `input_reference` 即使用首帧图生视频。

### 5.4 使用上游原生字段

```json
{
  "model": "muse-video",
  "prompt": "让画面中的小船缓缓前进，水面出现自然涟漪。",
  "seconds": 15,
  "size": "16:9",
  "input_reference": "https://your-public-cdn.example/boat.png"
}
```

上述 JSON 可提交到本站 `/videos` 或 `/videos/generations`。两者是兼容入口，不应为同一创作同时调用两次。

## 6. 任务状态、响应字段与轮询

### 6.1 创建成功

创建成功返回任务信息，不直接返回视频二进制：

```json
{
  "id": "task_0123456789abcdef",
  "request_id": "task_0123456789abcdef",
  "object": "video.generation.task",
  "model": "muse-video",
  "status": "pending",
  "progress": 0,
  "created_at": 1794000000
}
```

示例 ID、时间戳均为占位值。保存 `request_id`，若缺失可读取 `id`。任务可能很快完成，不能假定创建响应一定是 `pending`。HTTP `200` 代表请求已被处理，仍须读取 `status` 判断任务结果。

### 6.2 查询任务

```bash
export TASK_ID="task_0123456789abcdef"
curl --fail-with-body "$SUB2API_BASE_URL/videos/$TASK_ID" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

上游任务刚完成时的示例：

```json
{
  "id": "task_0123456789abcdef",
  "request_id": "task_0123456789abcdef",
  "object": "video.generation.task",
  "model": "muse-video",
  "status": "done",
  "progress": 100,
  "created_at": 1794000000,
  "video": {
    "url": "/v1/videos/task_0123456789abcdef/content"
  }
}
```

视频已保存到本站后，查询可能来自本地记录：

```json
{
  "id": "task_0123456789abcdef",
  "request_id": "task_0123456789abcdef",
  "object": "video.generation.task",
  "status": "completed",
  "created_at": 1794000000,
  "completed_at": 1794000080,
  "video": {
    "url": "/v1/videos/task_0123456789abcdef/content"
  }
}
```

### 6.3 状态兼容表

| 客户端应识别的状态 | 含义 | 后续操作 |
| --- | --- | --- |
| `pending` | 上游排队或运行中，经本站统一转换 | 等待后继续查询 |
| `processing` | 本站历史记录中的处理中状态 | 继续通过单任务接口查询 |
| `done` | 上游已经生成，本站提供下载入口 | 请求 `/content` 下载 |
| `completed` | 本站记录中的完成状态 | 请求 `/content`；仅完成状态不保证文件已保存 |
| `failed` | 生成失败或被取消 | 读取 `error`，不再轮询该失败任务 |

为了兼容上游原始响应或其他部署版本，通用客户端还可将 `queued/running` 归为处理中，`succeeded/success` 归为成功，`cancelled/canceled/expired/error` 归为失败。本站当前 Muse 适配主要返回上表五个值。

### 6.4 单任务响应字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` / `request_id` | string | 同一任务的标识；本站 Muse 响应会带这两个字段 |
| `object` | string | `video.generation.task` |
| `model` | string，可选 | 可能为 `muse-video`；本地保存后的查询可能省略 |
| `status` | string | 见状态表 |
| `progress` | number，可选 | 上游进度参考，通常为 0–100；不代表剩余时间，本地响应可能省略 |
| `created_at` | integer | Unix 秒级时间戳；上游响应与本地记录的创建时间可能略有差异 |
| `completed_at` | integer，可选 | 本站记录完成的时间，不是视频播放长度 |
| `video.url` | string，可选 | 本站受保护的下载路径；不是可匿名访问的上游直链 |
| `error` | object 或 string，可选 | 失败原因；对象常带 `message` 和 `code` |

单任务接口不承诺返回实际视频秒数、宽高、码率、文件大小或用量账单。提示词、请求秒数、自动分辨率等提交参数可在历史列表的 `metadata` 中读取。

轮询建议：开始每 5 秒一次，超过 5 分钟改为每 15 秒一次；这与工作台行为一致。轮询超时表示客户端停止等待，不会取消服务端任务。请保留 ID 后继续查询。

## 7. 下载、播放与保存

```bash
curl --fail "$SUB2API_BASE_URL/videos/$TASK_ID/content" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Accept: video/mp4,video/*;q=0.9,*/*;q=0.8" \
  --output video.mp4.part && mv video.mp4.part video.mp4
```

- `/content` 返回二进制，不能当 JSON 解析。HTTP 错误时可能返回 JSON 错误对象，应先检查 HTTP 状态，避免把错误文本保存成视频。
- 未完成时可能返回 `409 video_not_ready`；稍后查询同一个任务，不要重新创建。
- 首次下载包含读取上游文件、转成浏览器可播放的 H.264 MP4、保存等步骤，可能比后续下载慢。客户端和反向代理需要留出下载等待时间。
- 当前本站视频存储/转换限制为 **512 MiB**；这不是首帧图片大小限制，也不是上游生成能力的承诺。
- 已保存的视频可以处理 `Range: bytes=…` 范围请求。首次读取 Muse 文件可能忽略 Range 并返回完整 `200`；客户端必须兼容 `200` 和 `206`。
- `video.url` 通常是以 `/v1/…` 开头的相对本站路径。最稳妥的方式是自己用已知 Base URL 和任务 ID 构造 `/videos/{id}/content`，不要把 Key 发往任意响应 URL。
- 浏览器 `<video src="…">` 无法自行附加 Bearer Header；应由自己的后端代理，或通过带鉴权的 `fetch` 获取 Blob 后播放。不要把 Key 放进 URL 查询串。
- 下载成功后请保存文件。`persistent_history: true` 表示持久化历史能力，不保证上游媒体或对象存储永久可下载。

## 8. 历史记录与清空

### 8.1 查询历史

```bash
curl --fail-with-body "$SUB2API_BASE_URL/videos/tasks?limit=10" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

| 查询参数 | 类型 | 默认 | 范围 |
| --- | --- | --- | --- |
| `limit` | integer | `10` | 正常使用 1–50；大于 50 按 50，非法值或小于等于 0 回退 10 |

列表按创建时间从新到旧返回。没有分页游标、`page`、`offset`、模型筛选或状态筛选参数。它包含当前 Key 的视频任务；一个 Key 允许多个视频模型时，可在客户端依据 `metadata.model` 筛选 Muse。

```json
{
  "object": "list",
  "retention_days": 7,
  "persistent_history": true,
  "data": [
    {
      "id": "task_0123456789abcdef",
      "request_id": "task_0123456789abcdef",
      "object": "video.generation.task",
      "status": "completed",
      "created_at": 1794000000,
      "completed_at": 1794000080,
      "expires_at": 1794604800,
      "metadata": {
        "operation": "image",
        "model": "muse-video",
        "prompt": "让水面产生自然涟漪。",
        "resolution": "auto",
        "aspect_ratio": "16:9",
        "duration": 10
      },
      "content_available": true,
      "content_type": "video/mp4",
      "byte_size": 1048576,
      "browser_playable": true,
      "billing_status": "charged"
    }
  ]
}
```

示例中的保留天数、文件大小、时间、价格状态均不代表固定服务配置。

| 字段 | 说明 |
| --- | --- |
| `retention_days` | 当前保留策略提示；代码默认 7 天，可由站点配置调整 |
| `persistent_history` | 是否启用持久化历史；不等于视频文件永久保留 |
| `data[].status` | 本地状态：`processing`、`completed`、`failed` |
| `http_status` | 可选，最近记录的任务查询/交付 HTTP 状态，不是独立的账单状态 |
| `error` | 可选，任务失败原因 |
| `expires_at` | 记录上的过期时间提示，Unix 秒；持久化历史的可见性还取决于站点策略 |
| `metadata.operation` | `text` 或 `image` |
| `metadata.model/prompt` | 提交时的模型和提示词 |
| `metadata.duration` | 提交的期望秒数，不是媒体实测秒数 |
| `metadata.aspect_ratio` | 提交的画幅提示；未指定时可能省略 |
| `metadata.resolution` | Muse 为 `auto` |
| `content_available` | 本站是否已经保存可访问的视频文件；为 false 不一定意味着生成失败 |
| `content_type/byte_size` | 可选，已保存文件的媒体类型和字节数 |
| `browser_playable` | 本站记录的播放兼容状态；应同时检查 `content_available` |
| `billing_status` | `pending`、`charged`、`not_charged` 或 `billing_failed` |

历史列表不会主动把每个任务刷新成最新上游状态。处理中任务应继续调用 `/videos/{id}`。列表不返回首帧原始图片数据，复用图生视频参数时请自行保留并重新提交图片。

### 8.2 清空历史

```bash
curl --fail-with-body -X DELETE "$SUB2API_BASE_URL/videos/tasks" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

成功为 `204 No Content`，不要调用 JSON 解析器读取响应体。

该操作清空/隐藏**当前 Key 的全部视频历史**，不受上一次 `limit` 影响，也不支持传任务 ID 做单条删除。后端没有“自动只删已完成任务”的过滤；请等处理中或下载中的任务结束再调用。

清空历史不会取消上游生成、删除上游视频、撤销账单或退款。已知 ID 是否仍可查询取决于存储和保留策略；不要把清空历史当作取消任务或永久擦除接口。

## 9. 计费、并发、超时与重试

### 9.1 计费规则

模型广场对应价卡是价格来源；调用前可通过 `/videos/pricing` 获取同一套计费逻辑的报价。

| 配置 | 一次成功生成如何计算 |
| --- | --- |
| `per_request` | 按次单价 × 1；1 秒和 60 秒的同价卡任务费用相同 |
| `video` | 按秒单价 × 请求的期望秒数；上游没有返回可信实际秒数供本接口结算 |

按次价使用用户/分组有效倍率；按秒价可能使用站点配置的视频独立倍率。客户端应直接使用报价中的 `unit_price`，不要自己拼接管理员配置猜测价格。

正常持久化任务在首次获取视频内容、完成保存和播放格式处理后触发结算；创建受理、单纯等待不代表已经扣费。失败/取消且未产出视频的 Muse 任务不按成功生成扣费。同一个任务的重复轮询和重复下载使用同一结算标识，不应重复收取生成费用。

`billing_failed` 表示计费登记异常，不能当作“免费生成”的承诺；请联系站点处理。计费状态可能晚于生成状态，最终以本站用量记录为准。报价接口不会锁定价格；提交时保存的是所选计费模型，不应理解为锁定所有价格和倍率。

### 9.2 并发与任务生命周期

每次 POST 是独立任务；`n > 1` 不可用于批量生成。本站用户、分组和上游都有容量限制，没有统一公开的固定并发数。遇到 `429`、`503` 时降低并发，并依据错误原因及 `Retry-After`（如有）处理。

上游默认视频任务超时配置为 600 秒，可由运营方调整；这不是用户可以提交的 `timeout` 参数，也不是生成时长保证。首帧读取、排队、转码和下载可能还有各自的等待时间。

客户端断开连接或停止轮询不会取消已经创建的任务。已确认的上游实现没有面向普通调用者的视频取消接口；上游服务重启也可能使未完成任务失败。

### 9.3 重试与幂等

- **创建 POST 没有公开的幂等键保证。** 重复提交相同 JSON 会创建不同任务，可能分别计费。
- 创建超时或网络中断时，上游可能已经受理。优先检查近期历史并联系站点排查，不要立即自动重发创建请求。
- `X-Request-ID`、`X-Client-Request-ID` 等响应头用于排查，不等于业务任务 ID，也不是防重复提交的幂等键。
- 查询/下载遇到临时网络错误、`408/409/425/429/5xx` 可以对**同一任务 ID**退避重试；具体 `5xx` 是否可恢复仍取决于错误原因。
- 任务响应中的 `status: "failed"` 是业务终态，即使 HTTP 为 200 也不应无限轮询。
- 客户端等待截止后保留任务 ID，下次从查询恢复。不要为了“继续等待”再次 POST。

## 10. 错误码与排查

常见 JSON 错误格式：

```json
{
  "error": {
    "type": "invalid_request_error",
    "message": "duration must be an integer from 1 to 60"
  }
}
```

鉴权中间件也可能返回顶层错误结构：

```json
{
  "code": "INVALID_API_KEY",
  "message": "Invalid API key"
}
```

客户端应兼容 `error.message` 和顶层 `message`，以及 `error.code`、`error.type` 和顶层 `code`。上游错误体可能直接透传；不要只靠一个字段识别全部错误。

| HTTP / 状态 | 常见类型或原因 | 建议 |
| --- | --- | --- |
| `400` | `invalid_request_error`；无 prompt、时长错误、固定分辨率、多图、多视频等 | 按参数表修正请求 |
| `401` | `authentication_error` / `invalid_api_key` | 检查本站 Key、Bearer 格式和 Key 状态；上游凭据问题需管理员排查 |
| `403` | 分组未开放媒体生成、Key 无权限、余额不足或过期等 | 检查分组和账户状态；不要通过修改任务 ID 尝试访问其他 Key 的任务 |
| `404` | 任务不存在、Key 不匹配、模型不允许或记录已无法恢复 | 使用原 Key 与原任务 ID；核对模型列表 |
| `409` | `video_not_ready` | 继续查询原任务，完成后再下载 |
| `413` | 请求体超过站点限制 | 压缩首帧、减少 base64 体积，或使用可公开访问的图片 URL |
| `429` | 频率/并发/额度限制，或上游额度耗尽 | 降低并发；若为额度耗尽需联系站点；遵循 Retry-After（如有） |
| `500` | 任务存储、服务内部错误等 | 保留任务 ID 和响应中的追踪 ID，联系站点 |
| `501` 或任务失败 | 上游所选驱动尚未实现视频能力 | 管理员检查上游驱动；用户不能用参数开启能力 |
| `502` | `upstream_error` / `video_conversion_error`；下载、返回格式或转码失败 | 已有任务先重试查询/下载；不要直接重复创建 |
| `503 pricing_not_configured` | 报价接口没有可用价卡 | 联系管理员配置模型价格 |
| `503 video_pricing_unavailable` | 创建前无法获得有效价格 | 先检查报价接口，勿当作免费请求 |
| `503 muse_video_no_eligible_account` | 没有可调度的 Muse 上游账号 | 等待或联系管理员检查账号、模型映射和分组绑定 |
| `503 no_account_available` | 上游账号池无可用容量 | 降低并发或联系管理员 |
| `504` / 代理 `524` | 上游或反向代理超时 | 已获任务 ID 则继续查询；创建阶段未获 ID 则先核查历史 |
| HTTP `200` 且 `status=failed` | 异步任务失败，`error` 提供具体原因 | 结束轮询，展示错误；重建任务是一项新的生成请求 |

上游异步错误中还可能出现 `upstream_timeout`、`upstream_auth_failed`、`upstream_quota_exhausted`、`upstream_refused`、`interrupted`、`internal_error` 等 `error.code`。它们可能位于 HTTP 200 的任务响应内部，不一定表现为相同的 HTTP 错误状态。

## 11. 完整 Python 示例

使用 Python 3.10+，安装 `requests`。首次运行将创建一条生成任务；设置环境变量 `MUSE_TASK_ID` 后只恢复该任务，不再次创建。

```bash
python -m pip install requests
export SUB2API_BASE_URL="https://api.zicc.cc/v1"
export SUB2API_API_KEY="YOUR_SUB2API_KEY"
# 可选：本地首帧；不设置则文生视频
export MUSE_IMAGE_PATH="first-frame.png"
# 可选：恢复已有任务时设置，优先于创建
# export MUSE_TASK_ID="task_0123456789abcdef"
python muse_video_example.py
```

保存下面代码为 `muse_video_example.py`：

```python
import base64
import mimetypes
import os
import time
from datetime import datetime, timezone
from email.utils import parsedate_to_datetime
from pathlib import Path
from urllib.parse import quote

import requests

BASE = os.getenv("SUB2API_BASE_URL", "https://api.zicc.cc/v1").rstrip("/")
KEY = os.environ["SUB2API_API_KEY"]
SUCCESS = {"done", "completed", "succeeded", "success"}
FAILED = {"failed", "cancelled", "canceled", "expired", "error"}
TRANSIENT = {408, 409, 425, 429}
session = requests.Session()
session.headers.update({"Authorization": f"Bearer {KEY}"})


def check(response):
    if not response.ok:
        raise RuntimeError(f"HTTP {response.status_code}: {response.text[:1000]}")
    return response


def retry_delay(response, fallback):
    value = response.headers.get("Retry-After", "").strip()
    if not value:
        return fallback
    try:
        seconds = float(value)
    except ValueError:
        try:
            seconds = (parsedate_to_datetime(value) - datetime.now(timezone.utc)).total_seconds()
        except (ValueError, TypeError, OverflowError):
            return fallback
    return max(fallback, seconds)


def main():
    task_id = os.getenv("MUSE_TASK_ID", "").strip()
    if not task_id:
        payload = {
            "model": "muse-video",
            "prompt": "海边日落，浪花轻拍礁石，镜头平稳缓慢横移。",
            "duration": 10,
            "aspect_ratio": "16:9",
            "resolution": "auto",
        }
        image_path = os.getenv("MUSE_IMAGE_PATH", "").strip()
        if image_path:
            path = Path(image_path)
            mime = mimetypes.guess_type(path.name)[0]
            if mime not in {"image/png", "image/jpeg", "image/webp"}:
                raise ValueError("首帧请使用 PNG、JPEG 或 WebP")
            if path.stat().st_size > 20 * 1024 * 1024:
                raise ValueError("示例限制首帧不超过 20 MiB")
            encoded = base64.b64encode(path.read_bytes()).decode("ascii")
            payload["image"] = f"data:{mime};base64,{encoded}"

        price = check(session.get(
            f"{BASE}/videos/pricing", params={"model": "muse-video"}, timeout=30
        )).json()
        mode = price["billing_mode"]
        if mode not in {"per_request", "video"}:
            raise RuntimeError(f"未支持的计费方式: {mode}")
        units = 1 if mode == "per_request" else payload["duration"]
        print("预估费用 USD:", price["tiers"]["auto"]["unit_price"] * units, flush=True)

        # 不对创建 POST 自动重试；网络中断时先核查历史。
        task = check(session.post(
            f"{BASE}/videos/generations", json=payload, timeout=(15, 120)
        )).json()
        task_id = task.get("request_id") or task.get("id")
        if not task_id:
            raise RuntimeError(f"创建响应缺少任务 ID: {task}")
        print("请保存任务 ID:", task_id, flush=True)

    path_id = quote(task_id, safe="")
    started = time.monotonic()
    deadline = started + 30 * 60  # 客户端等待上限，可自行调整。
    while time.monotonic() < deadline:
        delay = 15 if time.monotonic() - started >= 300 else 5
        try:
            response = session.get(f"{BASE}/videos/{path_id}", timeout=(15, 60))
        except requests.RequestException as exc:
            print("查询暂时失败，将重试:", type(exc).__name__)
            time.sleep(delay)
            continue
        if response.status_code in TRANSIENT or response.status_code >= 500:
            print("查询暂时不可用:", response.status_code)
            wait = retry_delay(response, delay)
            response.close()
            time.sleep(min(wait, max(0, deadline - time.monotonic())))
            continue
        task = check(response).json()
        status = str(task.get("status", "")).lower()
        print("任务状态:", status, "进度:", task.get("progress", "未知"))
        if status in FAILED:
            raise RuntimeError(f"任务失败: {task.get('error', task)}")
        if status in SUCCESS:
            break
        time.sleep(delay)
    else:
        raise TimeoutError(f"停止等待；任务未取消。设置 MUSE_TASK_ID={task_id} 可继续查询")

    # 构造本站下载地址，避免向外部 URL 发送本站 Key。
    output = Path("muse-video.mp4")
    partial = output.with_suffix(".mp4.part")
    with session.get(f"{BASE}/videos/{path_id}/content", stream=True,
                     timeout=(15, 600)) as response:
        check(response)
        with partial.open("wb") as target:
            for chunk in response.iter_content(1024 * 1024):
                if chunk:
                    target.write(chunk)
    partial.replace(output)
    print("已保存:", output.resolve())


if __name__ == "__main__":
    main()
```

本例只对查询做有限时间内的重试。下载中断时可以用 `MUSE_TASK_ID` 重新运行；会查询和下载原任务，不会创建第二条视频。

## 12. 完整 Node.js 示例

使用 Node.js 20+，不需要 OpenAI SDK。环境变量同上；保存为 `muse-video-example.mjs`，运行 `node muse-video-example.mjs`。本例为文生视频；图生视频在 `payload` 中按第 4 节添加 `image` 即可。

```javascript
import { createWriteStream } from 'node:fs';
import { rename } from 'node:fs/promises';
import { Readable } from 'node:stream';
import { pipeline } from 'node:stream/promises';

const base = (process.env.SUB2API_BASE_URL || 'https://api.zicc.cc/v1').replace(/\/$/, '');
const key = process.env.SUB2API_API_KEY;
if (!key) throw new Error('请设置 SUB2API_API_KEY');
const headers = { Authorization: `Bearer ${key}` };
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const success = new Set(['done', 'completed', 'succeeded', 'success']);
const failed = new Set(['failed', 'cancelled', 'canceled', 'expired', 'error']);

async function readJSON(response) {
  if (!response.ok) throw new Error(`HTTP ${response.status}: ${(await response.text()).slice(0, 1000)}`);
  return response.json();
}

function retryDelay(response, fallback) {
  const value = response.headers.get('Retry-After');
  if (!value) return fallback;
  const seconds = Number(value);
  const millis = Number.isFinite(seconds) ? seconds * 1000 : Date.parse(value) - Date.now();
  return Number.isFinite(millis) ? Math.max(fallback, millis) : fallback;
}

let taskId = (process.env.MUSE_TASK_ID || '').trim();
if (!taskId) {
  const payload = {
    model: 'muse-video', prompt: '海边日落，镜头缓慢平移。',
    duration: 10, aspect_ratio: '16:9', resolution: 'auto',
  };
  const price = await readJSON(await fetch(`${base}/videos/pricing?model=muse-video`, {
    headers, signal: AbortSignal.timeout(30000),
  }));
  if (!['per_request', 'video'].includes(price.billing_mode)) throw new Error('未知计费方式');
  console.log('预估费用 USD:', price.tiers.auto.unit_price * (price.billing_mode === 'video' ? payload.duration : 1));

  // 创建请求不自动重试。
  const task = await readJSON(await fetch(`${base}/videos/generations`, {
    method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' },
    body: JSON.stringify(payload), signal: AbortSignal.timeout(120000),
  }));
  taskId = task.request_id || task.id;
  if (!taskId) throw new Error('创建响应没有任务 ID');
  console.log('请保存任务 ID:', taskId);
}

const pathId = encodeURIComponent(taskId);
const started = Date.now();
let completed = false;
while (Date.now() - started < 30 * 60 * 1000) {
  const delay = Date.now() - started >= 300000 ? 15000 : 5000;
  let response;
  try {
    response = await fetch(`${base}/videos/${pathId}`, { headers, signal: AbortSignal.timeout(60000) });
  } catch {
    console.log('查询网络暂时不可用，稍后重试');
    await sleep(delay);
    continue;
  }
  if ([408, 409, 425, 429].includes(response.status) || response.status >= 500) {
    const wait = retryDelay(response, delay);
    await response.body?.cancel();
    await sleep(Math.min(wait, Math.max(0, 30 * 60 * 1000 - (Date.now() - started))));
    continue;
  }
  const task = await readJSON(response);
  const status = String(task.status || '').toLowerCase();
  console.log('状态:', status, '进度:', task.progress ?? '未知');
  if (failed.has(status)) throw new Error(`任务失败: ${JSON.stringify(task.error)}`);
  if (success.has(status)) { completed = true; break; }
  await sleep(delay);
}
if (!completed) throw new Error(`停止等待，任务未取消。设置 MUSE_TASK_ID=${taskId} 可恢复`);

const response = await fetch(`${base}/videos/${pathId}/content`, {
  headers, signal: AbortSignal.timeout(600000),
});
if (!response.ok) throw new Error(`下载失败 HTTP ${response.status}: ${(await response.text()).slice(0, 1000)}`);
if (!response.body) throw new Error('下载响应没有内容');
await pipeline(Readable.fromWeb(response.body), createWriteStream('muse-video.mp4.part'));
await rename('muse-video.mp4.part', 'muse-video.mp4');
console.log('已保存 muse-video.mp4');
```

Windows PowerShell 设置环境变量时使用 `$env:SUB2API_API_KEY="…"`、`$env:SUB2API_BASE_URL="…"`；Bash 示例中的 `export` 不能直接照搬到 PowerShell。使用 curl 时可明确调用 `curl.exe`。

## 13. 未支持的功能与参数

以下能力没有被当前 Muse 适配接通。某些未知 JSON 字段可能被忽略，并不一定返回错误；“请求成功”不能证明这些字段生效。

| 功能 / 字段 | 当前行为或替代方式 |
| --- | --- |
| 固定分辨率：`resolution=480p/720p/1080p/2k/4k` | 拒绝；只能 `auto` 或省略 |
| `width`、`height`、`image_size` | 不作为 Muse 输出尺寸控制传给上游 |
| `fps`、`frame_rate`、`bitrate`、编码/质量档位 | 未接通，不能控制输出帧率、码率或编码质量 |
| `seed`、`negative_prompt`、`guidance_scale`、`steps` | 未接通；`seed` 不提供可重复生成保证。额外描述可写入 prompt，但不等价于原生控制 |
| `first_frame`、`last_frame`、首尾帧模式 | 这些字段未接通；首帧必须用 `image` 或 `input_reference`；尾帧不支持 |
| `image_references`、`video_references`、多参考图 | 未接通；`images` / `reference_images` 明确拒绝 |
| `video`、`video_url`、视频编辑/视频续写 | 当前 Muse 模型不支持；不要调用 `/videos/edits` 或 `/videos/extensions` |
| `audio`、`audio_url`、`generate_audio`、`audio_enabled`、配音、口型控制 | 没有独立控制参数；不承诺生成结果必然有声或无声 |
| `watermark` | 没有水印开关，不承诺无水印 |
| `response_format`、`b64_json`、缩略图、封面/分镜提取 | 视频通过受保护的 `/content` 获取，不提供这些返回形式 |
| `stream` / SSE、`async=false` | 不支持切换；本接口始终采用异步任务流程 |
| `callback_url` / webhook | 没有任务回调，客户端轮询 |
| `timeout` / `priority` | 不是可用的 Muse 请求参数；客户端超时和服务端配置分别管理 |
| `user`、`metadata` | 不作为 Muse 视频生成或历史自定义字段透传 |
| multipart 文件上传 | 当前视频适配仅支持 JSON；将图片编码为 data URL，或提供公开图片 URL |
| `Idempotency-Key` | 没有公开的创建幂等保证 |
| 取消、单任务 DELETE、重新生成/混剪端点 | 未提供；重新 POST 是新的生成任务；清空历史不等于取消 |

Grok、Gemini Omni、Sora 等模型的文档中即使存在上述参数，也不能直接套用到 `muse-video`。使用 OpenAI SDK 时，某些 `videos` 封装会强制 multipart、限定模型/seconds/size 或假定不同响应结构；建议优先使用本文展示的 HTTP JSON 调用，不能承诺所有 SDK 的 `videos.create()` 可直接兼容。

## 14. 本站、上游及官方资料对照

### 14.1 两层 API 的差异

| 项目 | 本站 Sub2API | muse2api 原生接口 |
| --- | --- | --- |
| Key | 本站用户 Key | muse2api 服务 Key |
| 创建路径 | `/v1/videos/generations` 或 `/v1/videos` | `/v1/videos` |
| `model` | 必须 `muse-video`，由管理员配置映射 | 可省略；默认 `muse-video`，源码也含 `sora` 别名及未知名称回落逻辑；本站不因此开放这些别名 |
| `prompt` | 非空字符串，首尾去空白 | schema 为长度至少 1 的字符串 |
| `duration` / `seconds` | 整数 1–60；均省略时本站明确发送 5 秒 | schema 均默认 null；都不传时驱动不追加秒数提示，没有固定 5 秒的原生默认保证 |
| `size` | 字符串，采用值最多 16 字符 | 字符串，schema 未声明枚举或长度上限 |
| `aspect_ratio` | 本站提供的兼容字段，转为 `size` | 没有原生字段；直接传给上游不会自动转为 `size` |
| `resolution` | 只支持 auto，固定分辨率提前报错 | schema 没有该控制字段 |
| `image` / `input_reference` | 单字符串或兼容 URL 对象 | schema 为单个字符串；底层加载支持 data URL、公开 URL 和 base64 |
| `n` | 只接受 1 | 视频 schema 没有此字段，每次创建一个任务 |
| 状态 | 适配为 pending/done/failed，本地历史还有 processing/completed | queued/running/succeeded/failed/cancelled |
| 成功结果 | `video.url` 指向本站鉴权下载接口 | `result.url`、`result.mime`、`result.width`、`result.height`，宽高可能为 null |
| 视频下载 | `/v1/videos/{id}/content`，同一个本站 Key | `/v1/media/{name}`，上游路由为公开媒体访问 |
| 历史/报价 | 本站提供用户历史和模型价卡报价 | 不提供本站这两套用户接口 |

上游视频 schema 的全部业务字段是 `prompt`、`model`、`size`、`duration`、`seconds`、`image`、`input_reference`。其宽松 schema 允许额外字段出现，但视频路由只消费这些字段。本站适配也只构造 `model/prompt/duration/size/image` 发给上游，所以未知字段不会自动获得透传支持。

### 14.2 资料来源与能力依据

本次上游源码基线：`www222fff/muse2api` 提交 `3a20b5e7aa4056c99c0eb8c732d73ce3804d5dbf`。

- [muse2api 中文 README](https://github.com/www222fff/muse2api/blob/3a20b5e7aa4056c99c0eb8c732d73ce3804d5dbf/README.zh-CN.md)：视频创建、查询和部署说明。
- [上游请求 schema](https://github.com/www222fff/muse2api/blob/3a20b5e7aa4056c99c0eb8c732d73ce3804d5dbf/src/muse2api/api/schemas.py)：全部原生视频请求字段、1–60 秒约束。
- [上游视频路由](https://github.com/www222fff/muse2api/blob/3a20b5e7aa4056c99c0eb8c732d73ce3804d5dbf/src/muse2api/api/routes/videos.py)：字段优先级、异步返回形态。
- [上游浏览器驱动](https://github.com/www222fff/muse2api/blob/3a20b5e7aa4056c99c0eb8c732d73ce3804d5dbf/src/muse2api/drivers/browser/driver.py)：`_media_prompt` 把 size/duration 写入自然语言提示，`generate_video` 上传单张首帧。
- [上游图片读取与媒体存储](https://github.com/www222fff/muse2api/blob/3a20b5e7aa4056c99c0eb8c732d73ce3804d5dbf/src/muse2api/core/media.py)：图片引用类型、网络图片大小/重定向限制。
- [上游任务状态](https://github.com/www222fff/muse2api/blob/3a20b5e7aa4056c99c0eb8c732d73ce3804d5dbf/src/muse2api/services/tasks.py)：异步状态与重启中断行为。
- [Muse 官方网站](https://muse.ai/)、[官方产品介绍](https://introducing.muse.ai/)、[官方商业介绍](https://muse.ai/business)、[官方支持页](https://muse.ai/support)：本次已核对的官方公开资料。这些页面没有提供可核对的 `muse-video` 公开 API 参数规格，也没有给出本接口的视频时长、4K、画幅、音轨或多图参考输出保证。

**官方依据的边界：** 公开介绍未提供参数规格，不等于官方产品绝不具备某项能力；只是不能把未确认的网页功能写成此 API 的支持承诺。因此本文全部可调用参数以本站实现与 muse2api 源码交叉核对，60 秒和画幅仅作为请求提示，也没有把其他同名产品或其他视频模型的参数表混入本站 Muse 接口。
