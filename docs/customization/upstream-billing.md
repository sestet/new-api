# 上游精准计费

## 计费原则

启用精准计费的渠道以“上游账号实际产生的美元成本”为最终结算依据：

```text
最终 quota = 上游真实美元成本 x 请求时最终分组倍率 x 请求时 QuotaPerUnit
```

最终分组倍率按以下优先级确定：如果存在 `GroupGroupRatio[用户分组][实际使用分组]`，直接使用该覆盖值；否则使用 `GroupRatio[实际使用分组]`。两者不会相乘。

本站模型价格、token 数、缓存倍率和工具费用仍用于请求前的额度预估及上游账单暂不可用时的临时结算，预估金额同样乘最终分组倍率。拿到真实账单后，系统按上游真实成本乘最终分组倍率，对用户钱包或订阅额度补扣/退款差额，并同步修改消费日志。充值分组倍率不参与这条计费链路。

请求创建时会保存用户分组、实际使用分组、最终倍率、倍率来源和 `QuotaPerUnit`。即时结算、后台对账和账单二次复核始终读取这份快照，管理员后来修改倍率或额度精度不会追溯改变历史请求。

## 上游成本倍率

渠道绑定上游账号后，可以自动探测上游 API Key 声明的成本倍率，也可以切换为手动倍率。自动模式默认每 360 分钟刷新一次；探测失败时保留最后一次有效倍率，并记录错误供管理员查看。

请求计费读取渠道中已经保存的倍率，不会在每次请求过程中等待远程探测。自动模式也允许管理员预先填写本地默认倍率：尚未取得探测结果时使用该值；没有填写时回退为 `1`。探测成功后以探测值覆盖当前倍率，探测失败则继续保留当前有效值。手动模式始终使用管理员填写的倍率。系统先确定实际渠道，再在向上游发送请求前按该渠道倍率完成预扣，不存在“未拿到远程倍率就不扣费”的窗口。

探测顺序：

- Sub2API 使用渠道 API Key 请求 `GET /v1/sub2api/billing`。
- New API 优先使用渠道 API Key 请求 `GET /v1/new-api/billing`。
- 旧版 New API 没有公开声明接口时，兼容路径会使用已配置的上游账号凭证确认 Key 所有权和 Key 分组。普通账号会读取该账号在同一使用分组下最新一条消费日志中的 `group_ratio`；目标 Key 自身不必已经产生记录。上游 root 账号则直接读取当前 `GroupRatio` / `GroupGroupRatio` 配置。
- 消费日志倍率是最近一次实际使用时的观测值，不是上游当前配置的声明值。上游修改倍率后，普通账号要等同一分组再次产生消费记录才能探测到新值；分组从未产生过记录时继续使用渠道手动默认倍率。
- 多 Key 渠道要求所有 Key 返回相同倍率；不一致时拒绝更新，避免随机路由导致计费口径变化。

该倍率只修正“尚未取得真实账单时”的本地临时预估：

```text
临时预估 = 本地模型预估成本 x 上游成本倍率 x 本地成员分组倍率
```

上游返回的 `actual_cost` 或由上游日志 quota 换算出的真实美元成本已经包含上游定价，因此精确结算不会再次乘上游成本倍率：

```text
精确结算 = 上游真实成本 x 本地成员分组倍率
```

本站同时公开 `GET /v1/new-api/billing`。API Key 持有者可读取该 Key 当前解析出的有效倍率，但不会获得完整分组配置或其他 Key 信息。

## 账号与渠道关系

- “上游账号”保存一个上游站点登录账号的账单访问凭证。
- 一个上游账号可以绑定多个渠道。
- 渠道继续保存自己用于模型请求的 API Key；账单账号不会替代渠道 API Key。
- 绑定入口位于“渠道 -> 编辑渠道 -> 精准计费 -> 当前上游账号”。
- “上游账号”页签只负责创建、编辑、测试、查看状态、主动对账和删除账号，不在此处维护绑定关系。
- 已被渠道引用的账号不能删除，必须先在各渠道中选择“不绑定”。
- 上游账号本身没有代理配置。实际账单查询会沿用对应渠道的代理；账号列表中的独立连接测试为直接访问。
- 上游账号列表的“使用统计”按 7 天或 30 天汇总请求数、token、精确覆盖率、上游成本和成员扣费，并提供每日及模型维度明细。

每个渠道可以单独控制是否二次复核及复核期限，因此同一个账号下的 OpenAI 渠道和 Claude 渠道可以共享登录凭证，同时保留各自的 API Key 和复核策略。

## 支持的上游

### New API

必填信息：

- 账单 API Base URL，例如 `https://example.com`，不要包含 `/v1`、查询参数或锚点。
- 账号访问 token。
- 某些版本还要求账号用户 ID，用于请求头 `New-Api-User`。

使用的接口：

- `GET /api/status`：读取上游 `quota_per_unit`。
- `GET /api/log/self`：读取当前账号消费日志。

真实美元成本按下面的公式计算，使用十进制定点运算，不经过 `float64` 金额舍入：

```text
上游美元成本 = 上游日志 quota / 上游 quota_per_unit
```

### Sub2API

必填信息：

- 账单 API Base URL。
- `refresh_token`。保存账号后由系统换取并维护 access token。

使用的接口：

- `POST /api/v1/auth/refresh`：轮换 refresh token 并获取 access token。
- `GET /api/v1/usage`：读取真实 `actual_cost`。
- `GET /api/v1/keys`：按渠道 API Key 自动解析对应的上游 key ID。

Sub2API 刷新操作会返回新的 refresh token，旧 token 可能立即失效。系统在数据库事务和账号级互斥锁内保存新 token，同一个上游账号绑定多个渠道时只刷新一次。不要把同一个 refresh token 同时交给其他程序独立轮换。

access token 会在剩余寿命约四分之一时提前刷新；提前量最少 5 分钟、最多 6 小时。后台默认每 15 分钟检查一次，不会等到过期后才刷新。

### 自动识别

`auto` 只在已配置的同一个 Base URL 上尝试受支持的账单协议。系统不会根据 `api-cn.example.com` 推测或改写为 `api.example.com`，也不会跨域寻找账号中心。无法稳定识别时应明确选择 New API 或 Sub2API 并填写实际账单 API 地址。

## 账单匹配规则

系统首先向上游透传本站请求 ID，并优先按请求 ID 精确匹配。如果上游替换了请求 ID，则使用消费特征做唯一回退匹配。

| 项目 | New API | Sub2API |
| --- | --- | --- |
| 首选匹配 | `request_id` | `request_id` |
| 回退模型 | 模型名 | 模型名 |
| 回退输入 | prompt tokens | input + cache creation + cache read tokens |
| 回退输出 | completion tokens | output tokens |
| 时间容差 | 完成时间前后 5 秒，未命中时回退到请求生命周期前后 5 秒 | 完成时间前后 5 秒，未命中时回退到请求生命周期前后 5 秒 |
| 金额来源 | `quota / quota_per_unit` | `actual_cost` |
| API Key 限定 | 上游账号日志权限 | 自动解析并传入 `api_key_id` |

候选账单会先排除已经被同一上游账号下其他本站请求认领的记录。完成时间窗口优先于整个请求生命周期窗口：只有完成时间附近没有候选时，才会考虑生命周期内的候选。

回退匹配只有一条候选时，会同时确认真实金额和上游账单 ID。多条候选金额完全相同时，可以确认真实金额，但会标记“金额精确、账单 ID 待确认”，不会认领其中任意一个 ID；多条候选金额不同时继续保留预估状态。这样长耗时生图、流式文本和普通对话共用同一套规则，同时避免把别人的账单扣到当前用户。

token 数用于定位账单，不用于覆盖上游真实金额。上游的模型价格、缓存倍率、服务层级、额外工具费或后续账单修订，只要最终体现在上游真实金额中，都会随对账结果进入最终扣费。

## 状态说明

| 状态 | 含义 | 当前扣费 |
| --- | --- | --- |
| 待确认 `pending` | 已创建对账记录，当前查询尚未完成 | 本地预估 |
| 预估 `estimated` | 上游接口正常，但账单暂未出现或无法唯一匹配 | 本地预估，后台继续查 |
| 精确 `exact` | 已取得唯一账单，或多个候选的真实金额完全一致 | 上游真实成本 |
| 失败 `failed` | 首次对账发生鉴权、网络、超时或响应格式错误 | 保留预估扣费，后台重试 |

状态会显示在消费日志金额下方。管理员可在日志详情查看上游请求 ID、供应商、真实美元金额、最终倍率、用户实扣、计费差额、尝试次数和后台复核信息。普通用户只看到最终实扣，不会看到倍率或上游成本。上游账号列表会显示最近连接状态；错误状态可悬停查看完整失败原因。

## 自动对账和复核

默认行为：

- 请求完成时立即查询上游账单，单次查询总超时 15 秒，最多尝试 3 次。
- 后台对账任务默认每 1 分钟调度，只在存在待处理记录时创建任务。
- 自动处理最近 30 天的记录。失败记录不会删除，但超过回看期限后不再自动扫描。
- 失败重试间隔：最初指数退避 1、2、4、8、16、30 分钟；记录超过 1 天后每 6 小时；超过 7 天后每天一次。
- 首次精确后可继续二次复核，默认复核 24 小时，渠道可配置 1 到 720 小时。
- 已精确记录的二次复核暂时失败时保持 `exact` 和原实扣不变，并按重试策略继续复核，不会降级为失败或重新按预估扣费。
- 二次复核间隔依次为 5 分钟、30 分钟、2 小时、6 小时、12 小时，之后每天一次，直到复核期限结束。
- 上游账号从错误恢复为正常后，该账号回看期限内的失败记录会立即重新入队。
- “立即对账”会为指定账号启动强制任务，最多扫描 200 条记录，也会复核已经精确的记录。

环境变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `UPSTREAM_BILLING_RECONCILE_TASK_ENABLED` | `true` | 是否启用自动对账任务 |
| `UPSTREAM_BILLING_RECONCILE_TASK_INTERVAL_MINUTES` | `1` | 自动对账调度间隔，最小 1 分钟 |
| `UPSTREAM_BILLING_RECONCILE_LOOKBACK_DAYS` | `30` | 自动和手动对账的回看天数，非法值回退到 30 |
| `SUB2API_TOKEN_REFRESH_TASK_ENABLED` | `true` | 是否启用 Sub2API token 自动刷新 |
| `SUB2API_TOKEN_REFRESH_TASK_INTERVAL_MINUTES` | `15` | token 刷新检查间隔，非法值回退到 15 |
| `UPSTREAM_COST_RATE_REFRESH_TASK_ENABLED` | `true` | 是否启用上游成本倍率自动刷新 |
| `UPSTREAM_COST_RATE_REFRESH_TASK_INTERVAL_MINUTES` | `360` | 上游成本倍率刷新间隔，最小 5 分钟 |

## 数据和幂等性

`upstream_billing_accounts` 保存账号凭证和连接状态；API 响应只返回“是否已配置”，不会回传 token 明文。

`upstream_billing_records` 以本站 `local_request_id` 建立唯一记录，保存：

- 本站/上游请求 ID、请求开始和完成毫秒时间、渠道、账号、用户和供应商。
- 金额是否已精确但具体上游账单 ID 仍存在歧义。
- 预估 quota、真实美元金额、上游 quota、最终实扣 quota。
- 用户分组、实际使用分组、最终倍率、倍率来源和 `QuotaPerUnit` 快照。
- 钱包或订阅的调整量、是否已应用、日志是否已更新。
- 查询次数、错误、下次重试、复核次数、修订次数和复核截止时间。

差额调整在数据库事务中锁定记录和用户/订阅，`adjustment_applied` 防止同一结果重复扣款。再次复核发现上游金额变化时，以当前已扣金额为基准只应用新的差额。

## 管理 API

接口均位于渠道管理路由下，并沿用渠道读写权限：

```text
POST   /api/channel/upstream_billing/detect
GET    /api/channel/upstream_billing/accounts
POST   /api/channel/upstream_billing/accounts
PUT    /api/channel/upstream_billing/accounts/:id
DELETE /api/channel/upstream_billing/accounts/:id
POST   /api/channel/upstream_billing/accounts/:id/test
POST   /api/channel/upstream_billing/accounts/:id/reconcile
GET    /api/channel/upstream_billing/accounts/:id/stats?days=30
POST   /api/channel/:id/upstream_billing/rate/detect
```

## 常见错误

- `New-Api-User header not provided`：该 New API 版本要求填写上游账号用户 ID。
- `401 Unauthorized`：token 无效、已过期，或 Sub2API refresh token 已被其他客户端轮换。
- `404`：账单 Base URL 或该上游实现的接口路径不正确；系统不会擅自切换域名。
- `configured new-api account does not own the channel API key`：旧版 New API 的账号接口只能搜索当前账号自己的 Key。改用 Key 所属账号；失败期间仍保留渠道当前的手动/最近有效倍率。
- `has no usable billing log for this key group`：普通上游账号可以从同一使用分组的最新消费日志观测倍率，但该分组还没有包含 `group_ratio` 的记录。先让该分组完成一次请求再探测，或继续使用手动默认倍率。
- `context deadline exceeded`：账单接口在 15 秒内未完成，当前请求先按预估结算，后台继续重试。
- `record not available`：账单尚未生成，或请求 ID/token/时间特征无法对应到唯一记录，不等同于永久丢单。
- `signature is not unique`：同一时间窗口存在多条完全相同的候选账单，为防误扣不会自动选择。
