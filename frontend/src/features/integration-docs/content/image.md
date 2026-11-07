# 图片生成接入文档

通过统一的图片异步接口接入 OpenAI、Gemini 和 Grok 图片模型，支持文生图、图片编辑及多参考图。模型权限、可用档位、价格和并发额度以当前 API Key 所属分组及实际接口响应为准。

## 快速开始

### 地址与鉴权

| 项目 | 内容 |
| --- | --- |
| 站点根地址 | `https://api.zicc.cc`，不带 `/v1` |
| 文生图接口 | `POST /v1/images/generations/async` |
| 图片编辑接口 | `POST /v1/images/edits/async` |
| 查询任务 | `GET /v1/images/tasks/{task_id}` |
| 鉴权请求头 | `Authorization: Bearer YOUR_API_KEY` |
| JSON 请求类型 | `Content-Type: application/json` |

使用本站生成的用户 API Key，并确认其分组已开放所需模型和图片生成功能。创建任务、查询任务和读取历史都必须使用**提交任务时的同一个 Key**，同一用户的另一个 Key 不能代查。

接入流程：**提交图片任务 → 保存 `task_id` → 轮询至完成 → 读取 `result.data[]` → 保存图片**。

### 第一个请求

以下 cURL 示例使用 Bash / Git Bash 语法。将占位值替换为自己的 Key；生产应用建议从自己的后端调用接口。

```bash
export BASE_URL="https://api.zicc.cc"
export API_KEY="YOUR_API_KEY"

curl "$BASE_URL/v1/images/generations/async" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "白色陶瓷杯的电商产品摄影，浅灰背景，柔和自然光，无水印",
    "n": 1,
    "size": "1024x1024"
  }'
```

成功提交返回 HTTP `202` 和任务 ID；`202` 只表示已接受，图片仍在生成。将返回的 ID 用于后续查询：

```bash
curl "$BASE_URL/v1/images/tasks/imgtask_example" \
  -H "Authorization: Bearer $API_KEY"
```

Windows PowerShell 可使用「异步任务与结果」中的 Python 示例，或用 `curl.exe` 并调整续行语法。SDK 如果要求含版本的 Base URL，填写 `https://api.zicc.cc/v1`，避免重复 `/v1/v1`。

### 接口速查

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/v1/models?image_studio=1` | 查询当前 Key 可用的图片模型 |
| GET | `/v1/images/pricing?model={model}` | 查询当前模型报价 |
| POST | `/v1/images/generations/async` | 异步文生图，JSON 请求 |
| POST | `/v1/images/edits/async` | 异步图片编辑，推荐 multipart 请求 |
| GET | `/v1/images/tasks/{task_id}` | 查询任务与结果 |
| GET | `/v1/images/tasks?limit=10` | 当前 Key 的图片历史 |
| DELETE | `/v1/images/tasks/{task_id}` | 删除单条记录，不能取消生成 |
| DELETE | `/v1/images/tasks` | 清空当前 Key 的记录 |
| POST | `/v1/images/generations` | OpenAI / Grok 同步文生图 |
| POST | `/v1/images/edits` | OpenAI / Grok 同步编辑 |

生产接入优先使用异步接口，减少长请求受代理超时的影响。Gemini 使用异步 Images 兼容入口；其原生 `/v1beta/models/{model}:generateContent` 请求与响应不同，不能直接混用。异步接口需要站点对象存储配置可用。

## 模型与参数

### 查询当前 Key 的模型

```bash
curl "https://api.zicc.cc/v1/models?image_studio=1" \
  -H "Authorization: Bearer YOUR_API_KEY"
```

客户端应使用返回的完整 `id`，保留 `-preview` 等后缀。列表受分组白名单、账户模型配置和上游快照影响；模型广场展示某个模型，不代表当前 Key 一定有权限调用。

### 图片模型

下表列出模型的适配档位，实际输出仍受当前渠道和上游支持限制。

| 平台 | 模型 ID | 可请求档位 | 说明 |
| --- | --- | --- | --- |
| OpenAI | `gpt-image-2` | 1K / 2K / 4K | 使用具体 `size` |
| OpenAI | `gpt-image-2.5` | 1K / 2K / 4K | 按本站模型映射接入，不推定为固定变体 |
| OpenAI | `gpt-image-2.5-flare` | 1K / 2K / 4K | 文生图与图生图，具体能力依上游 |
| OpenAI | `gpt-image-2.5-sunburst` | 1K / 2K / 4K | 文生图与图生图，具体能力依上游 |
| OpenAI | `gpt-image-2.6` | 1K / 2K / 4K | 支持文生图与图生图，传质量参数 |
| OpenAI | `gpt-image-2.6-flare` | 1K / 2K / 4K | 支持文生图与图生图，传质量参数 |
| OpenAI | `gpt-image-2.6-sunburst` | 1K / 2K / 4K | 支持文生图与图生图，传质量参数 |
| Gemini | `gemini-3-pro-image` | 1K / 2K / 4K 参数 | 价格需查询当前报价 |
| Gemini | `gemini-3-pro-image-preview` | 1K / 2K / 4K 参数 | 保留完整模型 ID |
| Gemini | `gemini-3.1-flash-image` | 1K / 2K / 4K | 使用异步兼容接口 |
| Gemini | `gemini-3.1-flash-image-preview` | 1K / 2K / 4K | 保留 `-preview` 后缀 |
| Gemini | `gemini-nano-banana-2.1` | 1K / 2K / 4K | 本站模型 ID，不推定对应其他官方型号 |
| Grok | `grok-imagine-image` | 1K / 2K | 本版未确认 4K |
| Grok | `grok-imagine-image-2.0` | 1K / 2K | 文生图与图生图 |
| Grok | `grok-imagine-image-quality` | 1K / 2K | 实际效果与耗时以生成结果为准 |

### 请求字段

| 字段 | 类型 | 要求 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 必填 | 当前 Key 可调用的完整模型 ID |
| `prompt` | string | 必填 | 画面内容、风格、文字、构图等描述 |
| `n` | integer | 建议显式填写 | OpenAI / Grok 为 1–4；Gemini 只能为 1；生产建议 1 |
| `size` | string | OpenAI 建议填写 | 具体像素，如 `1024x1024`、`3840x2160` |
| `quality` | string | OpenAI 可选 | 工作台使用 `auto`、`low`、`medium`、`high` |
| `resolution` | string | Gemini / Grok 建议填写 | Gemini：`1K` / `2K` / `4K`；Grok：`1k` / `2k` |
| `aspect_ratio` | string | Gemini / Grok 建议填写 | 如 `16:9`，范围见下表 |

异步接口不接受 `stream=true`。Gemini / Grok 不能直接照搬 OpenAI 的 `quality`、`mask` 等字段并假设有效；透明背景等高级选项需按具体渠道和模型确认。

### 数量、参考图与比例

| 平台 | 输出数量 `n` | 参考图数量 | 推荐比例 |
| --- | --- | --- | --- |
| OpenAI | 1–4 | API 接入建议不超过 10 张 | 1:1、3:2、2:3、16:9、9:16、4:3、3:4、21:9 |
| Gemini | 仅 1 | 最多 14 张 | 1:1、16:9、9:16、4:3、3:4、3:2、2:3 |
| Grok | 1–4 | 最多 10 张 | 1:1、16:9、9:16、4:3、3:4、3:2、2:3 |

参考图推荐 PNG、JPEG/JPG 或 WebP，单张控制在 20 MiB 以内；总请求大小还受站点和反向代理限制。OpenAI 的 10 张是推荐接入上限，不是所有上游 API 的统一硬上限。`n` 是输出数量，与上传参考图数量无对应关系。

### OpenAI 推荐尺寸

1K 普通方形使用 `1024x1024`。2K、4K 是本站档位名，各比例的实际像素组合不同。

| 比例 | 2K 档工作台尺寸 | 4K 档工作台尺寸 |
| --- | --- | --- |
| 1:1 | 工作台会切换为 3:2；普通方形使用 `1024x1024` | `2880x2880` |
| 3:2 | `1536x1024` | `3456x2304` |
| 2:3 | `1024x1536` | `2304x3456` |
| 16:9 | `2048x1152` | `3840x2160` |
| 9:16 | `1152x2048` | `2160x3840` |
| 4:3 | `1536x1152` | `3072x2304` |
| 3:4 | `1152x1536` | `2304x3072` |
| 21:9 | `1792x768` | `3584x1536` |

4K 横屏 16:9 使用 `3840x2160`，不能统一写为 `4096x4096`。4K 取决于本站档位配置与上游支持；报价中的 `openai4k_allowed` 只是配置标志。Gemini / Grok 使用档位和比例组合，档位不保证固定输出像素。

## 文生图示例

所有异步文生图均请求 `POST /v1/images/generations/async`。下面示例假设已按「快速开始」设置 `BASE_URL` 和 `API_KEY`；切换模型平台时，也需切换为有对应分组权限的 Key。

### OpenAI：普通方形图片

```bash
curl "$BASE_URL/v1/images/generations/async" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "白色陶瓷杯的电商产品摄影，浅灰背景，柔和自然光，无水印",
    "n": 1,
    "size": "1024x1024",
    "quality": "high"
  }'
```

需要 4K 横屏时，在确认权限和上游支持后将 `size` 改为 `3840x2160`。其他 OpenAI 图片模型可替换 `model`，须先确认其模型映射和可用性。

使用 `gpt-image-2.6`、`gpt-image-2.6-flare` 或 `gpt-image-2.6-sunburst` 时，将示例中的 `model` 替换为对应完整名称即可，接口路径不变。三个模型均支持 1K / 2K / 4K、文生图与图生图；请求时传入质量参数 `quality`。图片编辑示例也使用相同的模型名称，价格以该模型的当前报价为准。

### Gemini：2K 横屏图片

```bash
curl "$BASE_URL/v1/images/generations/async" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.1-flash-image",
    "prompt": "白色陶瓷杯产品海报，画面简洁，中文标题为春日好时光",
    "n": 1,
    "resolution": "2K",
    "aspect_ratio": "16:9"
  }'
```

`resolution` 接受大小写归一。接口兼容 `size: "2K"`，建议统一使用 `resolution`，避免传入相互矛盾的档位。Gemini 适配器不接受 `21:9`。

### Grok：2K 图片

```bash
curl "$BASE_URL/v1/images/generations/async" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-imagine-image-2.0",
    "prompt": "白色陶瓷杯与一本打开的书，木质桌面，晨间阳光，产品摄影",
    "n": 1,
    "resolution": "2k",
    "aspect_ratio": "4:3"
  }'
```

### 同步调用补充

OpenAI / Grok 可通过 `POST /v1/images/generations` 同步等待结果，长请求可能遇到代理超时。成功响应示例：

```json
{
  "created": 1793000000,
  "data": [
    { "url": "https://assets.example.com/generated.png" }
  ]
}
```

部分渠道返回 `data[].b64_json`，需要 Base64 解码保存。同步数量和高级字段受上游限制，不能将异步接口 `n≤4` 当作所有同步供应商的统一规格。Gemini 不支持本文的同步 Images 兼容入口。

## 图片编辑与多参考图

使用 `POST /v1/images/edits/async` 和 `multipart/form-data`，每张参考图重复提交一个名为 **`image`** 的文件字段。Gemini 适配器只读取 `image`，不要写成 `image[]`。

让 cURL 或 HTTP 库自动生成 multipart 的 `Content-Type` 和 boundary，不要手写缺少 boundary 的请求头。示例需要自行准备 `reference-1.png` 和 `reference-2.jpg`。

### OpenAI：两张参考图

```bash
curl "$BASE_URL/v1/images/edits/async" \
  -H "Authorization: Bearer $API_KEY" \
  -F "model=gpt-image-2" \
  -F "prompt=保留第一张杯子的外形，参考第二张图的配色，将背景改为浅绿色" \
  -F "n=1" \
  -F "size=1536x1024" \
  -F "quality=high" \
  -F "image=@reference-1.png;type=image/png" \
  -F "image=@reference-2.jpg;type=image/jpeg"
```

OpenAI 编辑入口也支持 JSON 形式 `images: [{"image_url": "HTTPS_URL_OR_DATA_URL"}]`。这是 OpenAI 特定输入方式，不能套用于 Gemini；该路径不支持 `images[].file_id`。

### Gemini：多参考图

```bash
curl "$BASE_URL/v1/images/edits/async" \
  -H "Authorization: Bearer $API_KEY" \
  -F "model=gemini-nano-banana-2.1" \
  -F "prompt=将两张参考图中的产品组合到同一张简洁海报中，保持各产品外形" \
  -F "n=1" \
  -F "resolution=2K" \
  -F "aspect_ratio=16:9" \
  -F "image=@reference-1.png;type=image/png" \
  -F "image=@reference-2.jpg;type=image/jpeg"
```

最多 14 个 `image` 文件字段，输出数量仍为 `n=1`。该适配接口不使用 JSON URL 数组替代 multipart。

### Grok：多参考图

```bash
curl "$BASE_URL/v1/images/edits/async" \
  -H "Authorization: Bearer $API_KEY" \
  -F "model=grok-imagine-image-quality" \
  -F "prompt=保持第一张产品的外观，结合第二张的环境，生成统一光照的产品照片" \
  -F "n=1" \
  -F "resolution=2k" \
  -F "aspect_ratio=4:3" \
  -F "image=@reference-1.png;type=image/png" \
  -F "image=@reference-2.jpg;type=image/jpeg"
```

最多 10 张参考图。编辑任务的查询方式与文生图相同，任务对象可为 `image.edit.task`，`metadata.operation` 可为 `edit`。

## 异步任务与结果

### 保存任务 ID

异步提交成功返回 HTTP `202 Accepted`，响应示例：

```json
{
  "id": "imgtask_example",
  "task_id": "imgtask_example",
  "object": "image.generation.task",
  "status": "processing",
  "created_at": 1793000000,
  "expires_at": 1793604800,
  "poll_url": "/v1/images/tasks/imgtask_example"
}
```

响应头包含 `Location` 查询路径与 `Retry-After: 3`。立即保存 `task_id`、所用 Key 的内部标识和提交参数，之后用原 Key 查询。日志和前端任务记录无需保存明文 Key。

### 轮询状态

```bash
curl "https://api.zicc.cc/v1/images/tasks/imgtask_example" \
  -H "Authorization: Bearer YOUR_API_KEY"
```

按 `Retry-After` 轮询；没有该头时，每 3 秒或更长查询一次。查询 HTTP `200` 不等于生成成功，要检查 `status`。

| `status` | 处理方式 |
| --- | --- |
| `processing` | 继续等待与轮询 |
| `completed` | 读取 `result.data[]` 并保存结果 |
| `failed` | 读取 `error`、`http_status`，停止成功等待流程 |

### 完成响应

```json
{
  "id": "imgtask_example",
  "task_id": "imgtask_example",
  "object": "image.generation.task",
  "status": "completed",
  "http_status": 200,
  "image_url": "https://assets.example.com/generated.png",
  "result": {
    "created": 1793000060,
    "data": [
      { "url": "https://assets.example.com/generated.png" }
    ]
  },
  "created_at": 1793000000,
  "completed_at": 1793000060,
  "expires_at": 1793604860,
  "metadata": {
    "operation": "generate",
    "model": "gpt-image-2",
    "quantity": 1,
    "size": "1024x1024"
  }
}
```

- 多图结果需要遍历 `result.data[]`；顶层 `image_url` 只是第一张的快捷字段。
- 异步结果通常返回对象存储 URL。及时下载，不依赖链接永久有效。
- 下载图片时使用结果资源地址，**不要把本站 `Authorization` 请求头转发到第三方资源域名**。预签名 URL 按原地址下载，应用应校验允许的资源来源。
- `metadata` 可能附带分组信息等可选字段，不应假设全部存在。
- 时间戳单位为 Unix 秒。存在 `completed_at` 时，耗时可按 `completed_at - created_at` 计算。

### 失败响应与多任务

任务失败时，查询请求本身仍可返回 HTTP `200`：

```json
{
  "task_id": "imgtask_example",
  "status": "failed",
  "http_status": 502,
  "error": {
    "type": "api_error",
    "message": "Upstream request failed"
  }
}
```

`n>1` 会拆成多个单图请求，任一子请求失败都可能使整体任务标记为 `failed`，且不返回已成功的部分图片。子请求仍按正常规则计费，因此整体失败不代表一定没有费用。生产接入建议每次 `n=1`，保存多个独立任务 ID，并设置有界并发。

收到任务 ID 后即可提交下一个任务，各任务独立轮询。本站异步任务不等同于离线 Batch API，不应使用 `/v1/images/batches` 替代，也不据此假设存在取消、Webhook 或服务重启自动续跑能力。默认任务执行等待上限为 30 分钟，不是完成时效承诺。

### Python：提交与轮询

安装 `requests`，将下面代码保存为 `image_client.py`。代码只定义函数，不会自动发起生成；在自己的程序中调用 `submit_image`，持久保存其返回的 ID 后调用 `wait_image`。本示例固定绑定一个 Key，跨 Key 并发应使用独立客户端。

```bash
python -m pip install requests
```

```python
import os
import time
from urllib.parse import quote

import requests

BASE_URL = os.environ.get("MEDIA_BASE_URL", "https://api.zicc.cc").rstrip("/")
API_KEY = os.environ["MEDIA_API_KEY"]
HEADERS = {"Authorization": f"Bearer {API_KEY}"}
RETRYABLE = {408, 409, 425, 429, 500, 502, 503, 504}


def submit_image(payload):
    # 不自动重试 POST：超时不代表服务端一定没有创建任务。
    response = requests.post(
        BASE_URL + "/v1/images/generations/async",
        headers=HEADERS,
        json=payload,
        timeout=(15, 180),
    )
    response.raise_for_status()
    body = response.json()
    task_id = body.get("task_id") or body.get("id")
    if not task_id:
        raise RuntimeError("未收到任务 ID，请保存响应并联系平台")
    return str(task_id)


def wait_image(task_id, wait_seconds=1800):
    url = BASE_URL + "/v1/images/tasks/" + quote(task_id, safe="")
    started = time.monotonic()
    while time.monotonic() - started < wait_seconds:
        try:
            response = requests.get(url, headers=HEADERS, timeout=(15, 60))
        except (requests.Timeout, requests.ConnectionError):
            time.sleep(3)
            continue
        retry_after = response.headers.get("Retry-After", "")
        delay = max(3, int(retry_after)) if retry_after.isdigit() else 3
        if response.status_code in RETRYABLE:
            time.sleep(delay)
            continue
        response.raise_for_status()
        body = response.json()
        if body.get("status") == "completed":
            return body
        if body.get("status") == "failed":
            raise RuntimeError(f"任务 {task_id} 失败：{body.get('error')}")
        time.sleep(delay)
    raise TimeoutError(f"停止等待，任务未取消；保留此 ID 继续查询：{task_id}")
```

运行自己的程序前设置环境变量 `MEDIA_API_KEY`；`MEDIA_BASE_URL` 可选，填写不带 `/v1` 的根地址。将「文生图示例」的 JSON 对象作为 `payload`。客户端超时后仍可用已保存的 ID 继续查询，不要自动重新提交生成。

## 报价与历史记录

### 查询当前报价

接入时以当前 Key 的模型报价和使用记录为准：

```bash
curl "https://api.zicc.cc/v1/images/pricing?model=gpt-image-2" \
  -H "Authorization: Bearer YOUR_API_KEY"
```

响应结构示例，数值仅用于展示格式：

```json
{
  "model": "gpt-image-2",
  "billing_model": "gpt-image-2",
  "billing_mode": "image",
  "pricing_source": "channel",
  "rate_multiplier": 1,
  "tiers": {
    "1K": { "base_price": 0.04, "unit_price": 0.04 },
    "2K": { "base_price": 0.04, "unit_price": 0.04 },
    "4K": { "base_price": 0.08, "unit_price": 0.08 }
  },
  "openai4k_allowed": true
}
```

| 字段 | 含义 |
| --- | --- |
| `model` | 请求的模型 ID |
| `billing_model` | 映射后用于计费的模型 ID |
| `billing_mode` | `image` 按图片模式；`token` 不适合按固定张价估算 |
| `pricing_source` | 报价来源，如 `channel` |
| `base_price` | 倍率前基础价格 |
| `unit_price` | 已含当前 Key 有效倍率的单价，不要再次乘倍率 |
| `tiers` | 分辨率阶梯；Token 模式下可能为 `null` |
| `openai4k_allowed` | OpenAI 4K 配置标志，不是上游能力实测 |

图片模式可用 `unit_price × 张数` 估算，实际结算还参考可识别的输出尺寸等信息。报价按模型解析，不要退回整个分组的旧统一图片单价。

### 读取历史

```bash
curl "https://api.zicc.cc/v1/images/tasks?limit=20" \
  -H "Authorization: Bearer YOUR_API_KEY"
```

默认 `limit=10`，最大 50，只返回当前 Key 的记录。响应示例：

```json
{
  "data": [],
  "retention_days": 7
}
```

`data` 中每项与任务查询结构相同；7 仅为示例，应读取实际 `retention_days` 和任务 `expires_at`。

### 记录与文件保留

任务记录保留期、图片对象生命周期、下载签名链接有效期分别管理。记录存在不代表文件或当前 URL 永久有效，重要结果应及时保存到自己的存储。参考图原始文件也应自行保存，不能依赖历史记录取回。

仅在任务已结束且不再需要结果时使用 `DELETE /v1/images/tasks/{task_id}` 或 `DELETE /v1/images/tasks`。删除记录不等于取消上游生成，也不保证删除对象存储文件；删除进行中的任务可能丢失后续查询入口。

## 常见问题

### HTTP 错误处理

| 状态码 | 常见原因 | 建议 |
| --- | --- | --- |
| `400` | 参数类型、格式、数量或比例不正确 | 检查模型对应参数后再提交 |
| `401` | Key 缺失、无效、停用或用户状态不允许 | 检查认证和账户状态 |
| `403` | 分组权限、余额/订阅、IP 限制、Key 过期 | 按错误信息处理，避免连续重试 |
| `404` | 模型不可用、入口不支持、异步未启用、任务不存在或 Key 不一致 | 区分模型、功能与任务错误 |
| `413` | 参考图或请求体超过限制 | 压缩图片、减少数量并确认总大小限制 |
| `429` | 并发、速率或 Key 额度限制 | 遵守 `Retry-After`；额度耗尽需处理额度 |
| `5xx` | 上游、存储或网关暂时异常 | 查询可退避重试；生成提交不盲目重发 |

错误响应通常包含 `error.message`，认证或中间件错误也可能使用顶层 `message`。程序应同时检查 HTTP 状态和错误正文。

```json
{
  "error": {
    "type": "invalid_request_error",
    "message": "错误说明"
  }
}
```

反馈问题时提供模型 ID、请求时间、HTTP 状态、脱敏错误以及响应头 `X-Request-Id`，不要提供完整 API Key。

### Gemini 同步接口为什么返回 404？

本文的同步 Images 兼容路由不支持 Gemini。文生图使用 `/v1/images/generations/async`，图生图使用 `/v1/images/edits/async`。如果异步请求提示 `async image tasks are not enabled`，请联系平台确认功能配置。

### 已提交但生成很慢，是否再次提交？

收到任务 ID 后继续查询原任务。网络超时不一定表示任务未创建，重复 POST 可能重复生成并再次计费。客户端停止等待也不等于服务端任务被取消。

### 为什么不建议一次请求多张？

当前 `n>1` 的子请求任一失败都可能导致整体失败，且已成功子请求仍按正常规则计费。需要逐张可靠处理时，使用多次 `n=1` 独立提交。Gemini 只能 `n=1`。

### 4K 能否统一填 4096×4096？

不能。OpenAI 使用具体像素，16:9 的 4K 示例为 `3840x2160`；其他比例参考尺寸表。Gemini 使用 `resolution` 档位和 `aspect_ratio`，Grok 本版未确认 4K 支持。

### 浏览器遇到 CORS 怎么处理？

可由自己的服务端调用本站，再向网页提供业务接口。直接浏览器接入需站点允许对应网页来源，且不要把生产 Key 暴露在公开网页或代码仓库中。
