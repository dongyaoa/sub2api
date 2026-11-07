# Muse 视频接入创作工作台

面向用户的完整调用说明见 [Muse Video API 用户调用文档](muse-video-api.md)，包含全部参数、响应字段、价格、curl/Python/Node.js 示例和功能边界。本文保留为部署及适配说明。

核对来源：[`www222fff/muse2api`](https://github.com/www222fff/muse2api)，提交 `3a20b5e7aa4056c99c0eb8c732d73ce3804d5dbf`（2026-11-07 核对）。同时检查 README、`api/schemas.py`、`api/routes/videos.py`、`drivers/browser/driver.py`、`services/tasks.py` 和 `core/media.py`，以实际接口实现为准。

## 上游能力与限制

| 项目 | muse2api 当前实现 | 工作台适配 |
| --- | --- | --- |
| 模型 | `muse-video` | OpenAI API Key 账户，独立模型能力 |
| 文生视频 | `prompt`，必填 | 支持 |
| 图生视频 | `image` 或 `input_reference`，一张首帧；支持 data URL、公开 HTTP(S) URL，底层也接受 base64 | 工作台上传单张首帧，以 data URL 提交；兼容统一接口的 `{ "url": "…" }` |
| 时长 | `duration` / `seconds`，整数 1–60，二者同时存在时 `duration` 优先 | 支持 1–60 秒；不再沿用 Grok 的 15 秒上限 |
| 画幅 | `size` 为自由字符串，示例为 `16:9` | 常用七种比例；API 也可传原生 `size`；统一 `aspect_ratio` 转为 `size` |
| 分辨率 | 没有独立分辨率字段、固定像素档位或 4K 保证 | 显示“由模型决定”，不展示虚构的 480p/720p/1080p/4K 选项 |
| 数量 | 每次一个异步视频任务 | 一次提交一条视频 |
| 其他生成参数 | 没有 seed、negative_prompt、音轨、尾帧、多图参考、视频编辑/延长等参数 | 不展示这些控制项 |

**1–60 秒是 API 可接受的期望时长，不是已证实的实际视频长度范围。** 浏览器驱动的 `_media_prompt` 把 `size` 和 `duration` 分别拼成 `aspect ratio …`、`… seconds long` 提示词。实际时长、画幅和清晰度取决于 Muse 的生成结果，接口没有保证值，也没有返回实际时长。`size=3840x2160` 会被原样当作画幅提示词，不能据此宣称支持 4K。

生产生成需使用 muse2api 的 `browser` 驱动；仓库的 `http` 驱动尚未实现视频生成，`mock` 只供测试。

## 配置

1. 在账号管理新增/使用 **OpenAI → API Key** 账户，Base URL 填 muse2api 服务地址，可带 `/v1`；填写 muse2api 的 API Key。
2. 同步该账户模型或配置 `muse-video → muse-video` 映射，绑定到工作台所用分组。专用视频目录读取映射、已同步模型目录和分组白名单，不受 Codex 专用模型清单限制。
3. 分组开启创作所需的图像/媒体生成权限；若开启模型白名单，加入 `muse-video`。
4. 在模型广场所用的分组模型价卡/渠道模型定价中，为 `muse-video` 配置价格。按次收费选择 `per_request`，填写每次价格；按秒收费选择 `video`，填写每秒单价。不要用 Grok 的分辨率单价配置替代 Muse 的模型价卡。
5. 创作工作台选择该分组 Key 和 `muse-video`，可切换文生视频/首帧图生视频。报价不可用时禁止生成，避免用 Grok 默认价格误收费。

升级时需执行项目数据库迁移，包括 `275_video_task_billing_model.sql`：新增可空的任务计费模型字段，以便渠道映射变化或缓存过期后仍按提交时选定的计费模型结算；旧任务保持兼容。

Muse 不提供实际秒数，若选择按秒收费，本适配按提交的期望秒数计价；推荐按次收费。设置为按次后，侧栏显示“/次”，预估费用不乘视频时长；明确配置的零价仍按零价处理。

## 接口对应

| Sub2API 工作台接口 | muse2api 上游接口/行为 |
| --- | --- |
| `GET /v1/models?video_studio=1` | 读取本地账户模型配置，列出可用视频模型 |
| `GET /v1/videos/pricing?model=muse-video` | 从与实际视频计费共用的定价逻辑获取报价和计费单位 |
| `POST /v1/videos/generations`（兼容 `POST /v1/videos`） | `POST /v1/videos` |
| `GET /v1/videos/{id}` | `GET /v1/videos/{id}`，转换 `queued/running/succeeded/failed/cancelled` |
| `GET /v1/videos/{id}/content` | 查询任务的 `result.url`，从配置上游的 `/v1/media/{filename}` 下载 |
| `GET /v1/videos/tasks` | Sub2API 既有任务历史，按用户与 API Key 隔离 |

本地任务记录的画幅字段上限为 16 个字符，超长 `size` 在提交前拒绝，避免上游已生成但历史记录保存失败。统一请求示例：

```json
{
  "model": "muse-video",
  "prompt": "枫叶在微风中缓缓飘落",
  "duration": 30,
  "aspect_ratio": "16:9",
  "resolution": "auto",
  "image": { "url": "data:image/png;base64,..." }
}
```

文生视频省略 `image`。后端转为 muse2api 的 `model`、`prompt`、`duration`、`size`、字符串 `image`；同时兼容原生 `seconds` 和 `input_reference`。非法时长、固定分辨率、多图或多视频数量在调用上游前报错。

任务与原上游账户绑定，查询不会切到其他账户。`succeeded + result.url` 转换为工作台的完成状态；失败/取消任务不产生生成扣费。复用既有视频保存、浏览器播放转换、用量去重流程；正常持久化任务在视频保存并可播放后扣费，重复轮询/下载不会重复收费。上游视频文件地址只提取合法媒体文件名，在管理员配置的 Base URL 下下载；不跟随任意返回地址或重定向。

## 验证范围

自动测试覆盖原生/统一参数、1–60 秒边界、单首帧、创建/状态/内容转换、任务归属平台、受控媒体下载、模型目录、报价/实际收费一致、免费价卡、缺价拦截、按次不乘秒数和任务去重。前端测试覆盖 OpenAI Key、参数切换、历史恢复、价格单位与过期报价防护。

这些验证使用模拟上游，不会发起真实收费生成。实际输出时长、画幅和清晰度需要部署后用自己的 muse2api 服务确认。
