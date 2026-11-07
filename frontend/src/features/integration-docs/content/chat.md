# 对话模型接入文档

使用本站 API Key 接入对话模型，既可以在 Codex、Claude Desktop 中使用，也可以通过 API 集成到自己的应用。模型名称、接口权限和价格以当前 Key 所属分组的模型广场及实际可用列表为准。

## 快速开始

### 先准备一个 API Key

1. 登录 [Z-API 控制台](https://api.zicc.cc)，进入「API 密钥」。
2. 创建或选择与使用场景匹配的 Key：Codex 使用 Codex 分组，Claude Desktop 使用 CC / Claude 分组。
3. 记录该 Key 对应的 Base URL、可用模型和分组权限。
4. 在下表选择接入方式，按对应章节完成配置。

| 使用方式 | 阅读章节 | 需要准备 |
| --- | --- | --- |
| Codex 客户端 | CC-Switch 接入 Codex | Codex、CC-Switch、Codex 分组 Key |
| Claude Desktop | Claude Desktop 配置 | Claude 桌面客户端、CC / Claude 分组 Key |
| 自己编写程序 | 接口与鉴权、API 调用示例 | API Key、完整模型 ID、对应接口协议 |

> Base URL 按客户端分别填写。CC-Switch 使用一键导入的完整地址；Claude Desktop 的 Gateway 使用站点根地址；本文 OpenAI SDK 示例使用带 `/v1` 的地址。不要给所有软件统一追加 `/v1`。

### 查询当前 Key 可用的模型

下面的示例适用于 Bash / zsh / Git Bash。将占位值替换为自己的 Key，模型名称从返回的 `data[].id` 中选择：

```bash
curl --fail-with-body -sS "https://api.zicc.cc/v1/models" \
  -H "Authorization: Bearer YOUR_API_KEY"
```

后文的 `YOUR_MODEL_ID` 是模型名称占位符，必须替换为当前 Key 实际可用、适合对应接口的完整模型 ID。查询模型列表不会发起对话生成。

## 接口与鉴权

### 地址速查

| 场景 | 地址或路径 | 说明 |
| --- | --- | --- |
| 站点根地址 | `https://api.zicc.cc` | Claude Desktop Gateway 使用此地址 |
| OpenAI SDK Base URL | `https://api.zicc.cc/v1` | SDK 会继续拼接对应接口路径 |
| 模型列表 | `GET /v1/models` | 获取当前 Key 可用模型 |
| Responses | `POST /v1/responses` | 使用 `input`，Codex 使用 Responses 协议 |
| Chat Completions | `POST /v1/chat/completions` | 使用 `messages`，用于兼容该协议的应用 |
| Anthropic Messages | `POST /v1/messages` | 使用 `messages`、`max_tokens`，用于兼容 Claude 协议的应用 |

本站地址以控制台实际显示为准。若平台地址发生变化，请替换示例域名。接口是否可用取决于 Key 所属分组、模型和通道；不能仅替换模型名就假定所有协议、工具或多模态能力都受支持。

### 请求头

Responses 与 Chat Completions 使用：

```http
Authorization: Bearer YOUR_API_KEY
Content-Type: application/json
```

Anthropic Messages 示例使用：

```http
x-api-key: YOUR_API_KEY
anthropic-version: 2023-06-01
Content-Type: application/json
```

API Key 前后不要带空格或换行。生产应用应通过自己的服务端保管 Key，不要写进公开前端代码、仓库或 URL。

### 分组与模型

- 使用当前 Key 能访问的完整模型 ID；保留模型名称中的版本和后缀。
- Codex 与 Claude Desktop 使用各自对应分组的 Key。修改客户端模型前，先确认该分组已开放该模型。
- 单价、分组倍率及实际消耗以「模型广场」和「使用记录」为准。

## CC-Switch 接入 Codex

CC-Switch 是管理和切换本地供应商配置的工具；一键导入来自本站控制台。

> 完成切换后必须彻底退出并重新启动客户端。

### 第一步：准备 Codex 与密钥

1. 从 Codex 官方发布渠道下载并安装适合当前系统的客户端。
2. 安装后先启动一次，让客户端创建本地配置目录。
3. 在本站「API 密钥」页面选择 **Codex 分组**的 Key，记录 Base URL 和可用模型。
4. 使用客户端的 **API Key** 登录方式；如当前登录了个人账号，先退出该登录，再使用本站 Key 配置。

| 系统 | 用户级 Codex 配置目录 |
| --- | --- |
| Windows | `%USERPROFILE%\.codex`，即 `C:\Users\你的用户名\.codex` |
| macOS | `~/.codex` |

已有手动配置时，先备份该目录中的配置文件，避免切换时覆盖自己的设置。

### 第二步：安装 CC-Switch

打开 [CC-Switch Releases](https://github.com/farion1231/cc-switch/releases/latest)，选择适合系统的稳定版。

| 系统 | 安装方式 |
| --- | --- |
| Windows | 下载普通 `.msi` 安装包，按提示安装 |
| macOS | 按下方命令通过 Homebrew 安装，或选择 Releases 中适配当前系统的安装包 |

```bash
brew tap farion1231/ccswitch
brew install --cask cc-switch
```

启动 CC-Switch。若提示导入已有 Codex 配置，先选择备份，再继续。

### 第三步：一键导入并启用

1. **完全退出 Codex**，包括仍在后台运行的进程。
2. 在本站「API 密钥」页面找到对应 Key 的 **CC-Switch 一键导入 / 一键配置**入口。
3. 浏览器询问是否打开 CC-Switch 时，允许打开。
4. 在 CC-Switch 中核对供应商、API Key、Base URL 和默认模型，确认导入。
5. 返回 **Codex** 供应商列表，找到刚导入的配置，点击 **启用**。
6. 确认状态显示为 **使用中**，再启动 Codex。

### 第四步：核对配置

| 配置项 | 应如何设置 |
| --- | --- |
| API Key | 当前 Codex 分组的本站 Key；与客户端使用的 Key 一致 |
| Base URL / `base_url` | 保留用户中心显示、一键导入的完整地址，不自行追加 `/v1` |
| 默认模型 / `model` | 选择该 Key 实际可用的模型 |
| `wire_api` | `responses` |
| `model_provider` | 必须与 `model_providers` 中对应配置段的名称完全一致 |

例如 `model_provider = "custom"` 应对应 `[model_providers.custom]`；导入结果为 `OpenAI` 时，应对应 `[model_providers.OpenAI]`。名称区分大小写，不要只修改其中一处。

若仍使用旧地址，在 Codex 设置中进入 **Configuration**，打开用户级 `config.toml` 检查。Windows 可尝试 `Ctrl + ,` 打开设置。供应商配置应放在用户级配置中，不能只写入某个项目下的 `.codex/config.toml`。

API Key 的保存方式由 CC-Switch、客户端版本及认证配置决定；不要为了排错随意添加一个 `api_key` 字段或同时更改多套凭证。采用 `requires_openai_auth = true` 时，Codex 使用登录认证，此时 `env_key` 不生效。

### 第五步：确认生效

新建会话，发送一个简单任务，确认正常返回；随后在本站「使用记录」查看对应调用。验证请求会产生实际用量。

切换或修改后仍未生效时，先完全退出客户端，再在 CC-Switch 中重新启用供应商并重启。避免同时在 CC-Switch 与 `config.toml` 中修改同一项，以免后保存的一方覆盖另一方。

## Claude Desktop 配置

通过桌面端 **Configure third-party inference** 配置第三方网关；菜单名称以所安装版本为准。

### 第一步：安装桌面客户端

从 [Claude Desktop 下载页](https://claude.com/download)选择适合系统的安装包，安装并启动。若当前版本要求先登录才能进入设置，按客户端提示进入可操作的设置界面。

### 第二步：启用开发者模式

依次进入以下设置：

1. 左上角菜单 → **Settings**。
2. **Help** → **Troubleshooting**。
3. 勾选 **Enable Developer Mode**。
4. 按提示重启客户端。
5. 在新增的 **Developer** 菜单中打开 **Configure third-party inference**。

### 第三步：填写网关配置

| 配置项 | 填写内容 |
| --- | --- |
| **Gateway base URL** | `https://api.zicc.cc` |
| **Gateway API key** | 本站生成的 **CC / Claude 分组** API Key |
| **Gateway auth scheme** | `bearer` |

> Gateway 使用 HTTPS 域名根地址，不额外添加 `/v1`，不接受 IP、端口形式或 HTTP 地址。

填写完成后点击右下角 **Apply locally**，按客户端提示应用配置。需要重启时先退出再重新打开。

### 第四步：验证对话

新建对话并发送简单文本，确认收到回复，再到本站「使用记录」检查调用是否使用了预期分组。确认 **Gateway auth scheme** 填写为 `bearer`，并使用对应分组的 API Key。

### 安装时连接失败

Windows 安装程序需要访问 Anthropic 服务。如果已配置本地代理，但安装程序未使用该代理，可在安装程序所在目录打开 **CMD**，为这次安装设置代理。

下面的 `7897` 是示例端口，请改成自己的代理实际 HTTP 端口；安装程序文件名也要与下载的文件一致：

```bat
set HTTP_PROXY=http://127.0.0.1:7897
set HTTPS_PROXY=http://127.0.0.1:7897
"Claude Setup.exe"
```

这些是 CMD 命令，不能直接当成 PowerShell 或 Bash 命令执行。此设置作用于当前 CMD 窗口及从它启动的程序。

找不到开发者模式或第三方推理菜单时，核对客户端版本，确认已启用 **Developer Mode** 并重启客户端。

## API 调用示例

下面的命令和代码在自己的电脑或服务端执行。将 `YOUR_API_KEY`、`YOUR_MODEL_ID` 替换为自己的 Key 和可用模型 ID。示例不依赖 CC-Switch 或桌面客户端。

### Responses：单次文本请求

```bash
curl --fail-with-body -sS "https://api.zicc.cc/v1/responses" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "YOUR_MODEL_ID",
    "input": "请用三句话介绍你能帮助我完成哪些任务。",
    "stream": false
  }'
```

非流式 Responses 返回 JSON。文本通常位于 `output[]` 中消息项的 `content[]`，筛选 `type: "output_text"` 后读取 `text`；不要按 Chat Completions 的 `choices` 结构解析。

### Chat Completions：消息列表

```bash
curl --fail-with-body -sS "https://api.zicc.cc/v1/chat/completions" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "YOUR_MODEL_ID",
    "messages": [
      {"role": "user", "content": "你好，请简短介绍自己。"}
    ],
    "stream": false
  }'
```

普通文本回复读取 `choices[0].message.content`。应用需要保留上下文时，将前面的用户消息和助手回复按顺序放入后续请求的 `messages`。

### Anthropic Messages：Claude 协议

使用 CC / Claude 分组的 Key，以及该分组实际可用的 Claude 模型：

```bash
curl --fail-with-body -sS "https://api.zicc.cc/v1/messages" \
  -H "x-api-key: YOUR_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "YOUR_MODEL_ID",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "请帮我列一个简单的学习计划。"}
    ]
  }'
```

普通文本回复读取 `content[]` 中 `type: "text"` 的 `text`。系统指令应放在顶层 `system` 字段，不要把 Chat Completions 的 `role: "system"` 消息原样混入此接口。

### Python：OpenAI SDK

安装依赖：

```shell
python -m pip install -U openai
```

下面使用 Responses，隐藏输入 Key，模型由运行时输入。SDK 的 `timeout` 是客户端设置，不是请求 JSON 字段：

```python
import getpass
from openai import OpenAI

client = OpenAI(
    base_url="https://api.zicc.cc/v1",
    api_key=getpass.getpass("请输入本站 API Key：").strip(),
    timeout=120.0,
    max_retries=0,
)

model = input("请输入当前 Key 可用的模型 ID：").strip()
response = client.responses.create(
    model=model,
    input="请用三句话介绍你能帮助我完成哪些任务。",
)
print(response.output_text)
```

此处关闭自动重试，便于先检查超时或连接错误的实际结果，再决定是否重新调用。

## 流式响应与参数

### 常用字段

| 字段 | 适用接口 | 说明 |
| --- | --- | --- |
| `model` | 三种接口 | 当前 Key 可用的完整模型 ID |
| `input` | Responses | 用户输入；最小示例使用字符串 |
| `messages` | Chat Completions / Messages | 按顺序传递对话消息 |
| `instructions` | Responses | 系统级指令，按模型及通道支持使用 |
| `system` | Anthropic Messages | 顶层系统指令 |
| `max_tokens` | Anthropic Messages | 必填的最大输出 token 数，不等于计费预估 |
| `stream` | 三种接口 | `true` 请求流式事件，默认示例使用普通响应 |

`temperature`、推理强度、工具调用、图片输入等能力与字段因模型而异。先使用最小文本请求确认通路，再按对应模型支持范围增加参数；不要把三种协议的字段混在同一个请求中。

### SSE 调用示例

在 Responses 请求中加入 `stream: true`，使用 cURL 的 `-N` 关闭输出缓冲：

```bash
curl --fail-with-body -N "https://api.zicc.cc/v1/responses" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "YOUR_MODEL_ID",
    "input": "请写一个简短的欢迎语。",
    "stream": true
  }'
```

流式响应需要按 SSE 事件解析，不能直接把整个响应当作一个 JSON 对象。

| 协议 | 文本增量 | 完成标记 |
| --- | --- | --- |
| Responses | `response.output_text.delta` 事件中的 `delta` | 正常结束时的 `response.completed`；同时处理失败或未完成事件 |
| Chat Completions | `choices[].delta.content` | 流结束标记 `data: [DONE]`，结合 `finish_reason` 判断停止原因 |
| Anthropic Messages | `content_block_delta` 中的文本增量 | `message_stop`，结合消息的停止原因判断结果 |

网络可能把一个事件拆成多个数据块，也可能一次返回多个事件。优先使用相应 SDK 的流式读取能力；自行实现时应缓存不完整事件，并处理错误事件和连接中断。

## 常见问题

### 客户端配置排查

| 现象 | 处理方法 |
| --- | --- |
| CC-Switch 已启用，Codex 仍用旧地址 | 完全退出客户端和后台进程，在 CC-Switch 重新启用，然后重启 |
| Codex 提示供应商不存在 | 核对 `model_provider` 与配置段名称，注意大小写 |
| Codex 配置更新后失效 | 查看用户级 `config.toml`，重新启用 CC-Switch 中的配置；避免同时用两种方式修改 |
| Claude Desktop 找不到第三方推理入口 | 核对版本，按教程启用 Developer Mode 并重启，查看 Developer 菜单 |
| Claude Desktop 网关地址被拒绝 | 使用 `https://api.zicc.cc` 这样的 HTTPS 域名根地址，不填 `/v1`、HTTP 或 IP 地址 |
| 无法使用某个模型 | 确认 Key 分组、模型权限及完整模型 ID，不能只凭客户端列表判断可用性 |

### 接口错误排查

| 状态码 | 优先检查 |
| --- | --- |
| `400` | JSON 格式、模型参数及所选接口协议 |
| `401` | Key 是否正确、失效或多了空格；Bearer 前缀是否完整 |
| `403` | 分组权限、Key 状态、IP 限制及余额等账户限制 |
| `404` | Base URL、接口路径、重复 `/v1` 或模型名称 |
| `413` | 上下文、图片或附件过大；缩短输入或拆分任务 |
| `429` | 并发、速率、额度或限额；降低频率并检查账户状态 |
| `500` / `502` / `503` | 网关或上游异常；保存错误信息，短暂等待后检查服务状态 |
| `504` / `524` | 上游或网关等待超时；先查看调用记录，再决定是否重试 |

出现错误时同时查看 HTTP 状态码和响应正文。客户端超时不一定意味着服务端没有处理请求，重试前先核对使用记录。

联系平台时提供模型名称、接口路径、请求时间、状态码、错误内容及 Request ID（如有），不要发送完整 API Key。
