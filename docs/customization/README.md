# 二开改动总览

本文档记录当前分支相对官方主线的定制范围，作为代码审查、发布和后续合并主线的入口。

## 基线

- 整理日期：2026-07-29
- 官方基线分支：`main`
- 官方基线提交：`1721144221ec5c94dd87891a7ae1bee228e7bb63`
- 基线提交说明：`fix(auth): keep login state on rate-limited or failing token refresh`

当前定制横跨额度数据类型、数据库结构、请求结算、后台任务、渠道管理和前端展示。它不是可以单独复制一个目录的插件，合并官方更新时必须按[主线同步手册](./upstream-sync.md)处理。

## 目标行为

1. 恢复请求分组倍率：组合覆盖倍率优先于基础分组倍率；充值分组倍率仍不使用。
2. 能获取上游账单时，按上游真实美元成本扣除用户额度。
3. 请求完成时暂时拿不到账单，先保留本地预估扣费，后台获取到账单后自动补扣或退回差额。
4. 一个上游登录账号可以绑定多个渠道，每个渠道继续使用各自独立的 API Key。
5. 上游后续修订账单时，在配置的复核期限内再次核对并修正用户扣费。
6. 将额度精度提升到每美元 `100,000,000` 个 quota，即最小可表示 `$0.00000001`。

精准计费的配置、匹配规则和重试行为见[上游精准计费](./upstream-billing.md)；数据库边界见[开发阶段数据库策略](./database.md)；生图工具见[GPT Image Playground 集成](./image-playground.md)。

## 改动地图

| 范围 | 主要文件 | 说明 |
| --- | --- | --- |
| 额度精度与安全换算 | `common/constants.go`、`common/quota_math.go` | `QuotaPerUnit` 提升到 `1e8`；quota 统一使用 `int64`；对外安全上限为 `Number.MAX_SAFE_INTEGER` |
| 数据库结构 | `model/*.go`、`model/main.go` | quota 持久化字段改为 `BIGINT`，ClickHouse 日志 quota 改为 `Int64` |
| 数据库基线 | `model/main.go`、`docs/customization/database.md` | 只保留当前初始结构；全新建库，不执行旧库补列、类型转换或数据回填 |
| 额度字段类型 | `model/`、`service/`、`controller/`、`relay/`、`dto/`、`types/` | 用户、令牌、渠道、日志、订阅、充值、兑换、任务和结算链路由 `int` 改为 `int64` |
| 上游账号 | `model/upstream_billing_account.go`、`controller/upstream_billing_account.go` | 独立保存 New API/Sub2API 账单凭证、连接状态和错误信息；一个账号可供多个渠道使用 |
| 对账记录 | `model/upstream_billing.go` | 保存请求映射、预估/真实金额、调整结果、重试和复核状态 |
| 账单查询与结算 | `service/upstream_billing.go` | 查询上游账单、刷新 Sub2API token、按真实成本换算、幂等调整钱包或订阅额度 |
| 请求 ID 透传 | `relay/channel/api_request.go`、`common/constants.go` | 向上游发送 `X-Request-ID`，并保留响应返回的上游请求 ID |
| 计费接入点 | `relay/helper/price.go`、`service/text_quota.go`、`service/quota.go` | 文本和其他同步结算流程接入真实账单，并按请求时最终分组倍率结算 |
| 自动任务 | `controller/system_task_handlers.go`、`model/system_task.go` | 定时对账、失败重试、账单二次复核、Sub2API 凭证提前刷新 |
| 管理 API | `router/channel-router.go` | 上游账号的增删改查、连接测试、手动对账和账单类型检测 |
| 渠道绑定 UI | `web/src/features/channels/` | “上游账号”页签只管理账号；绑定关系位于“编辑渠道 -> 精准计费” |
| 日志与状态 | `web/src/features/usage-logs/`、`service/log_info_generate.go` | 展示“精确、预估、待确认、失败”，详情记录对账信息 |
| 使用日志 | `controller/log.go`、`model/log.go`、`web/src/features/usage-logs/` | 流水页展示请求数、总消费、精准覆盖率、等待和失败；普通用户隐藏渠道与上游账号 |
| 订阅用户管理 | `controller/subscription.go`、`model/subscription.go`、`web/src/features/subscriptions/` | 集中查看订阅用户、套餐、周期、下次重置、已用和剩余额度，并支持直接分配订阅 |
| GPT Image Playground | `web/src/features/gpt-image-playground/`、`web/rsbuild.config.ts` | 选择当前用户已有 Token 和可用模型，通过同源 `/v1` 生图；图片历史只保存在用户隔离的浏览器 IndexedDB |
| 金额显示 | `web/src/lib/currency.ts`、`web/src/lib/format.ts`、`web/src/features/dashboard/lib/charts.ts` | 适配 `1e8` 精度和 `int64` quota 的格式化与图表汇总 |
| 界面整理 | `web/src/routes/__root.tsx`、`web/src/features/keys/components/api-keys-columns.tsx` | 关闭开发环境 TanStack 浮动工具；修复 API 密钥额度文字拥挤 |
| 分组计费设置 | `web/src/features/system-settings/models/group-billing-section.tsx`、`setting/ratio_setting/group_ratio.go` | 管理基础分组、基础倍率和用户分组到实际使用分组的覆盖倍率；新环境默认只保留 `default` |
| 国际化 | `web/src/i18n/locales/*.json` | 增加上游账号、精准计费、连接状态、复核和对账相关文案 |

## 开发数据库

当前只保留一套初始数据库结构，不提供旧库兼容迁移。数据库重建、当前结构和正式上线前的迁移边界见[开发阶段数据库策略](./database.md)。quota 字段直接采用 `BIGINT`，新建数据库从 `QuotaPerUnit=100000000` 开始。新环境只初始化倍率为 1 的 `default` 分组，其他分组由管理员按需添加。

## 当前边界

- 精准账单目前支持 New API 和 Sub2API 的账号账单接口，不会根据域名猜测或自动改写到另一个站点。
- 只对同步 relay 格式生效：OpenAI Chat、Responses、Responses Compaction、Claude、Gemini、Audio、Image、Rerank 和 Embedding。
- 不在上述列表中的异步任务或自定义计费路径仍使用本站原有结算逻辑，除非后续显式接入。
- 上游账单接口不可用、账单尚未生成或匹配不唯一时，不会伪造“精确”结果；系统保留预估扣费并进入后台重试。
- 前端仍以 JavaScript `number` 接收 quota，因此后端将单个 quota 值限制在 `9,007,199,254,740,991` 内，保证 JSON 往返不会静默丢失整数精度。
- 用户、令牌、渠道和日志中的 `group` 既用于渠道选择，也用于请求计费。管理员可配置 `GroupRatio` 和 `GroupGroupRatio`；普通用户只能看到可用分组说明和最终扣费，看不到倍率、上游成本或管理员毛利。

## 发布前检查

1. 按[开发阶段数据库策略](./database.md)使用全新数据库，主库和独立日志库中没有旧精度数据。
2. 确认 `QuotaPerUnit=100000000`，quota 列为 `BIGINT`，ClickHouse 日志 quota 为 `Int64`。
3. 检查 New API、Sub2API 各至少一个账号的“测试”结果。
4. 检查同一上游账号绑定两个不同 API Key 渠道时，账单仍映射到正确渠道。
5. 分别验证精确、预估、失败后恢复、二次复核改价和订阅额度调整。
6. 执行后端测试、前端类型检查、lint、格式检查和生产构建。
7. 观察系统任务、账号连接状态、消费日志和用户余额至少一个完整复核周期。
