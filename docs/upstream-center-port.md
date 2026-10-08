# 上游中心移植说明与验收

移植来源：[dongyaoa/sub2api-pool，pool-v0.2.13.24](https://github.com/dongyaoa/sub2api-pool/releases/tag/pool-v0.2.13.24)。功能源码取自本地 pool 仓库提交 `ad0c09cb9`（`ui: refine upstream monitor cards for pool 0.2.13-pool.24`），目标为 custom 分支。本次按功能文件和接入点移植，保留 custom 原有功能。

最初引入上游中心的 `8ef769f76` 同时移除了模型广场、签到和部分图片/视频功能，因此不能把该提交整体合并进 custom。用户端鹈鹕监测对应 `5f28a0306`，账号 OAuth 入口对应 `c53c8734d`，模型与生命周期管理对应 `c4f0a2aa7`，账号 API Key 监控对应 `4a52b5720`，最终卡片样式对应 `ad0c09cb9`；本次以最终版本的完整文件为准。

## 功能范围

| 入口 / 模块 | 包含功能 |
| --- | --- |
| 上游管理 | 厂家、Key 分组增删改查；从已有 OpenAI、Anthropic、Gemini API Key 账号批量导入；服务端读取并加密凭据；账号绑定、凭据变更后解除旧绑定；厂家与分组手动排序；归档、恢复与永久清理；可选充值比例换算。 |
| 监控中心 | 独立目标、已有账号导入、模型列表和多模型选择；OpenAI Chat Completions / Responses、Anthropic、Gemini；定时检测、暂停、立即检测；60 条检测历史、七天成功率、最新延迟、错误详情。 |
| 余额和倍率 | Sub2API / New API 上游余额、配额、订阅额度与有效倍率同步；共享钱包按标识及币种去重；失效凭据快照隔离；后台同步与手动刷新。 |
| 经营数据 | 今日业务请求与 Token、站内用户消费、上游实际消费及今日利润；分组与账目展示今日和最近 30 天（含今日）的消费与利润汇总，取消逐笔明细；按绑定生效时间固化站内消费归属；清理普通使用日志后继续保留独立账本。 |
| 智商监控 | 上游 Key、外部地址和本站分组来源；鹈鹕 HTML 动画生成；`gpt-6-astra` / `gpt-6.1-sol`；Responses / Chat Completions；超时、定时间隔、排序、启停、历史作品及单作品删除。 |
| 糖果测试 | 独立开关、间隔和并发池；结果记录条、评分及原始回复；旧结果本地重评；最近 60 条保留。 |
| OAuth 监控 | 指定真实 OpenAI OAuth 账号执行；复用 Token 刷新、代理和并发限制；账号状态与当前分组；同账号同模型去重；周限额冷却及恢复；永久删除及删除账号时清理关联计划。 |
| 站内监控 | 选择管理员已有有效密钥，或自动创建专用回环监控密钥；经过正常鉴权、路由和计费；记录实际执行账号与上游来源；用户展示配置。 |
| 账号管理 → 监控 | OAuth 账号复用 OAuth 计划与作品；API Key 账号复用现有上游绑定或匹配目标，支持幂等接入上游监控；状态、倍率、站内消费、上游实扣与可核对的今日利润；OpenAI 账号同时展示鹈鹕和糖果。 |
| 用户端鹈鹕监测 | 管理员开启后展示选中的站内计划；标题、说明、公告、隐藏失败作品；按用户分组权限过滤；用户只读查看作品与动画；不返回密钥、账号、内部来源或原始诊断。 |
| 存储与调度 | 检测历史、余额快照和作品保留策略；成本汇总保留；归档与永久清理；数据库租约、多实例并发上限、后台工作队列；服务启动和关停接入。 |

鹈鹕每个计划保留最近 20 条终态记录，活动记录另行展示。用户和管理员复用同一份历史。开启用户展示仅改变可见性，不会开启定时任务或发出模型请求。账号监控弹窗的读取也不触发模型请求；通过账号入口新接入的状态监控默认关闭。

## 财务口径

利润 = 本站用户消费 - 上游 API Key 实际消费。Sub2API 上游通过 `/v1/usage` 获取统计，同时传入 `days=30`、`timezone=<本站时区>` 和含今日的 `start_date` / `end_date`；今日成本直接使用 `usage.today.actual_cost`，最近 30 天成本优先使用 `daily_usage[].actual_cost` 之和，未返回逐日数据时兼容同一日期范围的 `model_stats[].actual_cost`。日期筛选由上游完成，今日口径遵循上游返回值。本站消费按绑定生效时间记录，绑定前的使用不自动归入当前 Key 分组。

New API 的 `/api/usage/token/` 只提供 Key 累计额度，今日与 30 天消费改从实际使用日志汇总，并按本站时区划分日期。仅有模型 Key 时读取 `/api/log/token`；该接口通常只返回近期记录，只有能确认覆盖完整时段时才计算消费。记录不足时，在 Key 分组中开启「消费、余额与倍率授权」，填写同一账号的用户 ID 和控制台个人访问令牌；服务端校验 Key 归属后，分页读取 `/api/log/self` 并按准确的 `token_id` 分别统计多个 Key，不使用可重名的 Key 名称或整个账号消费代替。

New API 原始消费先按 `/api/status` 返回的 `quota_per_unit` 转成 USD 额度，再应用可选充值比例；缺少额度单位时不猜测金额。同一账号的日志读取共享缓存，今日与历史分开同步；日志较多时分批跨同步轮次续读，完成后才发布金额。成功后的今日刷新补读新增日志，并定期完整校准。接口不兼容、上游限流或超时会提示记录未完整同步，不把部分日志总和作为完整消费；今日已成功取得的金额不因历史失败而丢失。

全站与供应商汇总仅包含当前未归档、且在统计期内有本站业务调用的 Key：今日查看今日调用，最近 30 天查看对应 30 天调用。参与范围依据业务账本记录，零收入请求也算业务调用；仅有上游可用性检测记录的 Key 不参与总成本和总利润，也不会使总计等待同步。全部 Key 都未使用时，汇总收入、成本和利润为零。显式选择单个 Key 时仍可查看其实际消费，方便核对监控及站外用量。

归档不会清空其他 Key 的统计，历史账本保留，恢复后重新纳入；修改供应商归属前的收入不会混入新的供应商。有本站业务调用的 Key 若缺少实际消费，显示其余参与 Key 的已知消费和缺失数量，待数据完整后再计算利润，不把缺失消费按零计算。

供应商支持可选的充值比例 `recharge_ratio`：实付 1 单位可获得 N 单位上游额度，实际成本 = 上游额度消费 ÷ N。例如设置 1:10，上游消费 100、本站用户消费 15，实际成本为 10、利润为 5。未设置时沿用原金额，页面仅对已设置的站点显示比例和换算说明；原始消费快照不修改。此设置是额度成本换算，不自动查询货币汇率；调整比例会按新比例重算今日和 30 天的展示。

本站有新请求不会清空上游消费；同日同步暂时失败或超过刷新间隔时，保留最近成功获取的实际金额并显示同步时间及旧数据提示。跨日不会把昨日快照作为今日消费，也不会把缺失金额当成零或用账号倍率估算。余额同步有延迟，利润随最近成功同步的上游数据更新。

本地账本的 `account_stats_cost × account_rate_multiplier` 是本站账号计费统计，不是上游实扣，不能作为成本或利润依据。页面不再展示无法匹配的逐笔上游成本与利润，也不再请求逐笔账目接口。Key 级上游消费可能包含站外调用及监控用量；同一汇总范围内相同上游地址和 Key 的实际消费只计一次，汇总利润也不等同于本站独占的现金利润。

## 页面与接口

| 用途 | 地址 |
| --- | --- |
| 管理员上游中心 | `/admin/upstreams` |
| 用户端鹈鹕监测 | `/pelican-monitor` |
| 上游管理、财务、存储、账号投影 | `/api/v1/admin/upstream-center` |
| 智商 / OAuth / 站内计划、作品、糖果、并发 | `/api/v1/admin/intelligence-monitors` |
| 登录用户鹈鹕配置、列表、作品 | `/api/v1/pelican-monitor` |

管理员从「上游中心 → 站内监控 → 用户展示」开启用户入口并选择计划。用户接口挂在现有 JWT 鉴权路由下，继续执行分组访问权限校验。外部检测使用公网 HTTPS；本站分组使用带一次性内部许可的回环请求，不开放任意内网 URL。

站内监控支持可选的鹈鹕提示词：每个监控可填写自己的提示词，选择本站分组后还可获取该分组的上游渠道（账号）并逐个覆盖。执行优先级为「实际调用渠道的提示词 → 该监控的提示词 → 默认鹈鹕提示词」，留空即可继承；提示词应要求输出 HTML 或 SVG 动画，以便继续使用作品预览。渠道调度与重试沿用原逻辑，每次转发按照实际选中的渠道应用提示词。配置在任务入队时保存快照，作品历史记录实际使用的提示词，之后修改配置不改变旧作品；糖果测试继续使用固定题目。

`GET /api/v1/admin/intelligence-monitors/local-channels?group_id=<ID>` 仅向管理员返回分组内渠道的基本名称与状态，不返回凭据。保存时校验渠道属于选中的分组；更换分组后需重新配置渠道提示词。单个提示词最多 8000 个字符，最多设置 200 个渠道，全部提示词合计最多 64000 个字符。新增迁移 `268_intelligence_monitor_prompts.sql` 为原计划补充空配置，原监控继续使用默认提示词。

「存储管理 → 归档数据 → 恢复」调用 `POST /api/v1/admin/upstream-center/storage/restore`，请求为 `{kind,id}`，支持 `supplier`、`target` 和 `intelligence`。上游恢复只包含与其同批归档的 Key，先前单独归档的 Key 仍留在归档列表。原账号绑定若仍有效且未被其他 Key 占用，则从恢复时刻建立新的绑定区间，不回填归档期间消费；绑定冲突时整个恢复操作回滚并提示。

恢复后的监控保持暂停，保留原有作品记录和历史账本，不自动发出付费检测。智商监控归档时已经清除的外部密钥、删除的专用站内密钥或失效来源，需要恢复后重新配置；页面会提示。永久清理或已按保留策略清除的记录不在可恢复范围内。

## 数据库迁移

正常启动由现有迁移器按完整文件名顺序执行并记录校验和，不需要手工执行 SQL。`265`、`266` 保存财务统计范围，`267` 增加可选充值比例；不修改已应用的历史迁移。custom 已有相同数字前缀但不同文件名的迁移继续独立存在；迁移器以完整文件名为主键。

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
| `266_upstream_finance_reported_30_days.sql` | 保存上游最近 30 天实际消费与对应日期范围，旧快照等待下一次同步补全。 |
| `267_upstream_supplier_recharge_ratio.sql` | 上游可选充值比例；空值不换算，原始消费快照保持不变。 |
| `268_intelligence_monitor_prompts.sql` | 站内监控可选提示词与按渠道覆盖配置；空配置继承默认提示词，作品保留实际使用的提示词。 |

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

真实模型生成、OAuth 额度状态、第三方余额与最终费用需要使用部署环境内已授权的线路验收；自动化测试覆盖本地逻辑、模拟上游和数据库行为。核对今日和 30 天汇总、同日失败保留金额、跨日旧快照不复用，以及页面不再展示逐笔明细；检查站外调用与监控用量已包含在 Key 级上游消费中，共享 Key 不重复计费。直接上游 / 外部 / OAuth 智商生成未单独计入厂家账本，不能把今日汇总利润当作扣除了全部生成费用的现金利润。
