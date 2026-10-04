# 上游中心移植说明与验收

移植来源：[dongyaoa/sub2api-pool，pool-v0.2.13.24](https://github.com/dongyaoa/sub2api-pool/releases/tag/pool-v0.2.13.24)。功能源码取自本地 pool 仓库提交 `ad0c09cb9`（`ui: refine upstream monitor cards for pool 0.2.13-pool.24`），目标为 custom 分支。本次按功能文件和接入点移植，保留 custom 原有功能。

最初引入上游中心的 `8ef769f76` 同时移除了模型广场、签到和部分图片/视频功能，因此不能把该提交整体合并进 custom。用户端鹈鹕监测对应 `5f28a0306`，账号 OAuth 入口对应 `c53c8734d`，模型与生命周期管理对应 `c4f0a2aa7`，账号 API Key 监控对应 `4a52b5720`，最终卡片样式对应 `ad0c09cb9`；本次以最终版本的完整文件为准。

## 功能范围

| 入口 / 模块 | 包含功能 |
| --- | --- |
| 上游管理 | 厂家、Key 分组增删改查；从已有 OpenAI、Anthropic、Gemini API Key 账号批量导入；服务端读取并加密凭据；账号绑定、凭据变更后解除旧绑定；厂家与分组手动排序；归档与永久清理。 |
| 监控中心 | 独立目标、已有账号导入、模型列表和多模型选择；OpenAI Chat Completions / Responses、Anthropic、Gemini；定时检测、暂停、立即检测；60 条检测历史、七天成功率、最新延迟、错误详情。 |
| 余额和倍率 | Sub2API / New API 上游余额、配额、订阅额度与有效倍率同步；共享钱包按标识及币种去重；失效凭据快照隔离；后台同步与手动刷新。 |
| 经营数据 | 今日业务请求与 Token、站内用户消费、上游实际消费及今日利润；经营明细保留本地账号计费额供核对，缺少逐笔上游实扣时成本与利润显示待核对；按绑定生效时间固化归属；清理普通使用日志后继续保留独立账本。 |
| 智商监控 | 上游 Key、外部地址和本站分组来源；鹈鹕 HTML 动画生成；`gpt-6-astra` / `gpt-6.1-sol`；Responses / Chat Completions；超时、定时间隔、排序、启停、历史作品及单作品删除。 |
| 糖果测试 | 独立开关、间隔和并发池；结果记录条、评分及原始回复；旧结果本地重评；最近 60 条保留。 |
| OAuth 监控 | 指定真实 OpenAI OAuth 账号执行；复用 Token 刷新、代理和并发限制；账号状态与当前分组；同账号同模型去重；周限额冷却及恢复；永久删除及删除账号时清理关联计划。 |
| 站内监控 | 选择管理员已有有效密钥，或自动创建专用回环监控密钥；经过正常鉴权、路由和计费；记录实际执行账号与上游来源；用户展示配置。 |
| 账号管理 → 监控 | OAuth 账号复用 OAuth 计划与作品；API Key 账号复用现有上游绑定或匹配目标，支持幂等接入上游监控；状态、倍率、站内消费、上游实扣与可核对的今日利润；OpenAI 账号同时展示鹈鹕和糖果。 |
| 用户端鹈鹕监测 | 管理员开启后展示选中的站内计划；标题、说明、公告、隐藏失败作品；按用户分组权限过滤；用户只读查看作品与动画；不返回密钥、账号、内部来源或原始诊断。 |
| 存储与调度 | 检测历史、余额快照和作品保留策略；成本汇总保留；归档与永久清理；数据库租约、多实例并发上限、后台工作队列；服务启动和关停接入。 |

鹈鹕每个计划保留最近 20 条终态记录，活动记录另行展示。用户和管理员复用同一份历史。开启用户展示仅改变可见性，不会开启定时任务或发出模型请求。账号监控弹窗的读取也不触发模型请求；通过账号入口新接入的状态监控默认关闭。

## 财务口径

今日利润 = 同一本站账期内的站内用户消费 - 上游 API Key 的实际消费。Sub2API 上游通过 `/v1/usage?days=1&timezone=<本站时区>` 返回的 `daily_usage[].actual_cost` 汇总实际消费；日期筛选由上游完成，返回行的日期标签可能采用其数据库时区，因此汇总所有返回行。利润只使用带有当日时间窗口且成功同步的快照；同步失败、旧快照没有账期、历史或任意部分时段均不能据此计算利润，应显示待核对。余额同步有延迟，今日利润代表最近一次同步后的值，不是逐请求即时结算。

本地账本的 `account_stats_cost × account_rate_multiplier` 是本站账号计费统计，不是上游实扣，也不能再作为业务成本或利润依据。模型 API Key 接口不提供每次请求对应的上游费用，因此经营明细的逐笔成本和逐笔利润保持待核对。Key 级上游消费可能包含站外调用及监控用量；共享 Key 的消费不能在多个厂家、分组或账号之间无依据地重复分摊，汇总利润也不等同于可审计的本站独占现金利润。

## 页面与接口

| 用途 | 地址 |
| --- | --- |
| 管理员上游中心 | `/admin/upstreams` |
| 用户端鹈鹕监测 | `/pelican-monitor` |
| 上游管理、财务、存储、账号投影 | `/api/v1/admin/upstream-center` |
| 智商 / OAuth / 站内计划、作品、糖果、并发 | `/api/v1/admin/intelligence-monitors` |
| 登录用户鹈鹕配置、列表、作品 | `/api/v1/pelican-monitor` |

管理员从「上游中心 → 站内监控 → 用户展示」开启用户入口并选择计划。用户接口挂在现有 JWT 鉴权路由下，继续执行分组访问权限校验。外部检测使用公网 HTTPS；本站分组使用带一次性内部许可的回环请求，不开放任意内网 URL。

## 数据库迁移

正常启动由现有迁移器按完整文件名顺序执行并记录校验和，不需要手工执行 SQL。以下共 24 个迁移；前 23 个保持来源文件名和内容，`265` 为本次财务口径修正。custom 已有相同数字前缀但不同文件名的迁移继续独立存在；迁移器以完整文件名为主键。

| 文件 | 作用 |
| --- | --- |
| `242_upstream_center.sql` | 厂家、目标、账号绑定、监控历史。 |
| `243_upstream_finance.sql` | 独立财务账本、快照和用量 / 账号变更触发器。 |
| `244_upstream_remote_billing.sql` | 上游计费倍率快照。 |
| `245_intelligence_monitor.sql` | 智商计划与执行记录。 |
| `246_intelligence_monitor_oauth.sql` | OAuth 来源与账号绑定。 |
| `247_upstream_finance_usage_totals.sql` | 业务 Token 及账户计费统计。 |
| `248_upstream_monitor_defaults.sql` | 可用性监控默认值。 |
| `249_intelligence_monitor_interval_seconds.sql` | 秒级定时间隔。 |
| `250_intelligence_monitor_generation_timeout.sql` | 单次生成超时快照。 |
| `251_upstream_manual_order.sql` | 上游、分组和计划手动排序。 |
| `252_intelligence_monitor_creation_defaults.sql` | 新建计划的定时默认值。 |
| `253_upstream_newapi_credentials.sql` | New API 凭据与同步。 |
| `254_upstream_storage_retention.sql` | 存储保留及监控成本汇总。 |
| `255_upstream_storage_policy.sql` | 存储策略与清理状态。 |
| `256_intelligence_upstream_plan_lookup.sql` | 上游计划查询索引。 |
| `257_intelligence_candy_monitor.sql` | 糖果测试及按测试类型的活动任务约束。 |
| `258_intelligence_candy_schedule.sql` | 糖果独立调度。 |
| `259_intelligence_local_key_ownership.sql` | 区分引用密钥与监控专属密钥。 |
| `260_intelligence_candy_grading_version.sql` | 糖果评分版本。 |
| `261_intelligence_candy_fingerprint.sql` | 兼容已有糖果记录结构。 |
| `262_intelligence_monitor_models.sql` | 计划模型与按账号 / 模型去重。 |
| `263_intelligence_deleted_oauth_cleanup.sql` | 清理已删除账号遗留的 OAuth 计划。 |
| `264_upstream_account_monitor_lookup.sql` | 账号监控凭据匹配索引。 |
| `265_upstream_finance_reported_day.sql` | 上游实扣当日金额和账期边界快照，用于今日利润核对；旧快照无账期边界，不反推利润。 |

升级前按现有部署流程备份数据库。新增 `263` 会删除已删除账号关联的历史 OAuth 监控，这是来源版本的既定生命周期行为。原有在用账号、普通渠道监控及 custom 其他功能无需转移到上游中心。

## 部署配置

默认鹈鹕并发为 8、糖果并发为 4，可在配置文件中设置：

```yaml
intelligence_monitor:
  max_concurrency: 8
  candy_max_concurrency: 4
```

也可使用环境变量 `INTELLIGENCE_MONITOR_MAX_CONCURRENCY`（1–256）和 `INTELLIGENCE_MONITOR_CANDY_MAX_CONCURRENCY`（1–128）。同数据库的多个实例共享并发上限，部署默认值应一致；部署配置变更需要重启。在上游中心「并发设置」保存后的数据库值优先，在线调整立即生效，调小不会终止已发出的请求。

计划默认使用 `gpt-6-astra`、`high`，可选择 `gpt-6.1-sol`；新建计划默认每 5 分钟执行，单次最多等待 10 分钟，可选择 5 / 10 / 15 分钟超时。可用性检测与鹈鹕 / 糖果是各自独立的任务。

后端使用现有加密器保存凭据，沿用现有数据库和密钥配置。后台定时执行不依赖浏览器打开。本站分组和 OAuth 的长生成请求保留工作任务的总时限，单独使用响应头等待预算，避免污染普通请求的连接池配置；正常业务计费继续使用独立计费上下文。

## 验证命令

下列命令用于复验，不代表已经接入真实付费上游。Go / Node / pnpm 版本遵循仓库现有要求；在对应目录执行。

```powershell
# backend 目录：功能服务、持久化、handler、鉴权和配置回归
go test ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes ./internal/server/middleware ./internal/config -run 'Test.*(Upstream|Intelligence|ManualOrder|Pelican)' -count=1
go test ./cmd/server -count=1
go build ./cmd/server
```

```powershell
# frontend 目录
pnpm run typecheck
pnpm exec vitest run src/components/admin/upstream src/components/pelican src/components/admin/account/__tests__/AccountMonitorDialog.spec.ts src/views/admin/__tests__/UpstreamCenterView.order.spec.ts src/views/admin/__tests__/AccountsView.lite.spec.ts src/views/user/__tests__/PelicanMonitorView.spec.ts src/api/__tests__/pelicanMonitor.spec.ts src/api/__tests__/admin.upstreamCenter.accountMonitor.spec.ts src/api/__tests__/admin.upstreamCenter.storage.spec.ts src/api/__tests__/admin.intelligenceMonitor.account.spec.ts src/api/__tests__/admin.intelligenceMonitor.candy.spec.ts src/api/__tests__/admin.intelligenceMonitor.controls.spec.ts src/stores/__tests__/pelicanMonitor.spec.ts src/composables/__tests__/useMonitorRefresh.spec.ts src/i18n/__tests__/localeKeyCompleteness.spec.ts
pnpm run build
```

实际 PostgreSQL 回归使用 `UPSTREAM_TEST_DATABASE_URL`；未设置时相关测试会跳过。应指向专用测试实例或空测试库，账户需要创建 / 删除 schema、表、索引、函数和触发器的权限。这组测试不要求 `integration` build tag；每个测试建立随机私有 schema，并为所有连接设置 `search_path`，结束后清理自身 schema。它们不启动后台调度，不调用真实模型。

另有 `TestUpstreamCustomUpgradePostgresPreservesCustomData` 完整升级回归，需要 `CREATEDB` 权限：历史 custom 迁移有明确引用 `public` 的 SQL，因此该测试在新建的 UUID 数据库中执行，传入连接串指向的数据库只用于创建 / 删除测试库。测试先应用全部 242 之前的 custom 迁移，保存账号、模型价格、密钥、批量图片冻结金额及充值促销快照，再应用全部迁移并重跑；验证旧数据、旧校验和和应用时间不变，以及财务触发器在真实使用日志表上生效。该回归已在隔离 PostgreSQL 18.4 上通过。

```powershell
# backend 目录；替换为专用测试 PostgreSQL 的连接串
$env:UPSTREAM_TEST_DATABASE_URL = 'postgres://postgres@127.0.0.1:15463/postgres?sslmode=disable'
go test ./internal/repository -run 'Test(Intelligence|Upstream|ManualOrder)' -count=1
Remove-Item Env:UPSTREAM_TEST_DATABASE_URL
```

本机 `.dev/LOCAL_DATA.md` 指定的 `C:\tmp\sub2api-preview` 是现有预览数据，不应用于一次性初始化。本次数据库验证使用独立 UUID 临时目录和独立本机端口，测试完成后停机清理。Windows PostgreSQL 原生命令在含中文的路径下可能初始化失败，应为一次性测试使用 ASCII 临时路径，保留既有预览目录。

## 人工验收路径

1. 管理员进入上游中心，确认五个 Tab、并发设置和存储管理可用；创建厂家并导入已有 API Key 账号，核对分组、绑定、排序、编辑与归档。
2. 配置一条可用性监控，检查手动检测、定时启停、模型切换、状态历史、余额和倍率同步；上游今日实际消费 1 元、同账期站内用户消费 2 元时，核对今日利润为 1 元；再核对负利润、同步失败和未知值状态。
3. 在账号管理打开 API Key 和 OAuth 账号的「监控」，核对与上游中心共用目标 / 计划，打开弹窗不会自行产生检测；验证暂停、编辑和历史作品同步。
4. 创建站内分组计划，通过有访问权限的正常密钥执行；核对实际执行来源、生成超时、鹈鹕和糖果独立调度及作品保留。
5. 开启用户展示并选择站内计划，分别用有 / 无该分组权限的普通用户检查列表及作品详情；撤销展示后直接详情访问也应失效。
6. 在测试数据上删除单幅终态作品、OAuth 计划和 OAuth 账号，确认关联记录及时消失且正在执行的旧任务不能重新写回。

真实模型生成、OAuth 额度状态、第三方余额与最终费用需要使用部署环境内已授权的线路验收；自动化测试覆盖本地逻辑、模拟上游和数据库行为。还需分别验证同步失败、旧快照、历史及部分时段、逐笔明细均显示待核对；检查站外调用与监控用量已包含在 Key 级上游消费中，共享 Key 不重复分摊。直接上游 / 外部 / OAuth 智商生成未单独计入厂家账本，不能把今日汇总利润当作扣除了全部生成费用的现金利润。
