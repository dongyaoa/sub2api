# Gemini Omni 视频接入

本适配依据提供的上游接口截图。模型为 `gemini-omni-1.2-flash`，同时识别截图列出的 `gemini-omni-1.2`、`omni-1.2`、`omni-featured` 别名。截图中的教程正文写了 3–10 秒，但参数表给出 3–15 秒；本实现按用户要求与参数表采用 **3–15 秒**，默认 5 秒。

## 配置与自动识别

1. 上游账户保持 **Gemini → API Key** 协议，填写现有上游地址与密钥。视频适配使用该账户的凭据，以 Bearer 认证调用同一上游的 `/v1/videos`；地址末尾的 `/v1` 或 `/v1beta` 会正确转换。
2. 账户模型映射或已同步模型目录中包含上述模型，并关联工作台 API Key 所属分组。若分组启用模型白名单，需包含要使用的模型。现有 Key 无需重新创建。
3. 分组开启媒体生成权限。工作台通过 `GET /v1/models?video_studio=1` 自动列出当前 Key 可使用的视频模型；不默认添加未配置的 Omni 模型。
4. 使用模型广场共用的**分组模型价卡或渠道模型定价**。按次选择 `per_request`；按秒选择 `video`。本实现不使用截图价格或 Grok 默认价格作为 Omni 的兜底价。

## 参数

| 参数 | 支持范围 / 规则 |
| --- | --- |
| `prompt` | 必填文字提示词 |
| `duration` / `seconds` | 3–15 的整数，默认 5；`seconds` 兼容 SDK 的数字字符串；同时传入时须相同 |
| `resolution` | `360P`、`720P`、`1080P`、`4K`，不区分大小写，默认 `720P` |
| `aspect_ratio` | `16:9`、`9:16`、`auto`，默认 `16:9` |
| `size` / `image_size` | 固定像素尺寸；`image_size` 另兼容档位名称、`[width,height]` 和 `{width,height}`；与显式分辨率、画幅冲突时拒绝 |
| `first_frame` / `last_frame` | 首帧、尾帧图片 URL 或 data URI；尾帧必须同时提供首帧 |
| `image_references` / `image_urls` | 最多 10 张图片参考 |
| `video_references` / `video_urls` / `video_url` | 最多 3 段视频参考，支持单 URL 别名 |
| `seed` | 可选整数 |

首尾帧模式与图片/视频参考模式互斥；参考模式可以同时使用图片和视频。工作台支持本地上传和 URL 输入，每个上传文件上限 20 MB；API 对内嵌 data URI 同样校验，远程 URL 的文件大小由上游读取时校验。

横屏尺寸依次为 `640x360`、`1280x720`、`1920x1080`、`3840x2160`，竖屏交换宽高。自动画幅时工作台不提交固定像素尺寸，让上游决定画幅。

## 创建、轮询与下载

```http
POST /v1/videos
Authorization: Bearer <Sub2API Key>
Content-Type: application/json

{
  "model": "gemini-omni-1.2-flash",
  "prompt": "镜头缓缓掠过雨后的森林，树叶上的水珠反射晨光",
  "duration": 15,
  "resolution": "1080P",
  "aspect_ratio": "16:9",
  "image_size": "1920x1080",
  "seed": 42
}
```

图生视频加入 `first_frame`，可选加入 `last_frame`；多参考模式改用 `image_references` 和/或 `video_references`。

- 创建：工作台和上游均使用 `POST /v1/videos`。
- 查询：`GET /v1/videos/{id}`，兼容上游 `queued`、`in_progress`、`completed`、`failed`，归一为现有工作台任务状态。
- 下载：`GET /v1/videos/{id}/content`。后端使用提交任务的原 Gemini 账户访问配置上游的同名端点，保存并验证可播放视频，供后续预览、下载和历史记录使用。
- 报价：`GET /v1/videos/pricing?model=gemini-omni-1.2-flash`，使用与实际计费一致的价卡。

任务按用户与 API Key 隔离，轮询和下载不换上游账户。正常持久化任务在视频成功保存、可播放后收费；失败任务不收生成费，重复轮询与下载复用去重计费。创建请求遇到不明确的上游错误时不会自动换账户重新创建。

按次价格不乘时长，使用普通分组/用户倍率；按秒价格乘提交秒数，并应用现有视频倍率规则。明确配置的零价有效，未配置价格的分辨率不能生成。预估金额来自后端报价，不受公共模型广场页面是否开启影响。

任务历史保存时长、分辨率、画幅和可选随机种子（包括 `0`），刷新后可复用；参考素材不写入任务参数历史，图生复用时需重新添加素材。部署时正常运行数据库迁移，为持久化视频任务添加可空种子字段。

## 验证范围

自动测试使用模拟上游，覆盖参数边界、参考素材组合、路由、Gemini 账户调度、模型发现、报价与扣费、任务状态及内容下载。未使用截图中的凭据，也未发起真实收费生成；真实上游的可用性、实际输出效果仍需部署后验证。
