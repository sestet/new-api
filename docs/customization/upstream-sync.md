# 合并官方主线更新

## 分支和远程建议

长期维护二开时，让官方主线和定制代码分开：

```text
upstream/main                 官方只读主线
main                          自有仓库的稳定发布分支
custom/exact-billing          长期定制开发分支
integration/upstream-日期     每次同步官方更新的临时集成分支
```

当前仓库的 `origin` 仍指向官方仓库，当前 `main` 已包含精准计费、运营界面和分组倍率的定制提交，工作区还有后续开发改动。开始同步官方主线前，必须先提交或暂存当前工作，并建议从当前提交创建长期定制分支；不要在脏工作区直接拉取或切换分支。

如果已经创建自己的远程 fork，建议把官方仓库命名为 `upstream`，自己的仓库命名为 `origin`。下面命令中的 `YOUR_FORK_URL` 需要替换成真实地址：

```bash
git remote rename origin upstream
git remote add origin YOUR_FORK_URL
git remote -v
```

如果暂时只有本地仓库，可以保留当前 `origin` 指向官方，但不要向官方仓库推送定制分支。

## 整理当前改动

从当前定制提交创建长期维护分支，后续继续按职责拆分提交。不要把数据库、后端计费、前端工具和文档压成一个无法审查的提交。

建议提交顺序：

1. `refactor(quota): use int64 quota with 1e8 precision`
2. `feat(billing): add upstream billing accounts and reconciliation models`
3. `feat(billing): settle and recheck against upstream actual cost`
4. `feat(web): manage upstream accounts and bind channels`
5. `feat(web): expose reconciliation status and exact amounts`
6. `fix(web): clean up billing-related layouts and developer tools`
7. `docs: document custom billing and upstream sync workflow`

数据库基线清理、GPT Image Playground、订阅用户管理和使用日志整理应各自保持独立提交，方便主线冲突时按职责审查。

拆分时只暂存属于当前提交的文件或代码块；每个提交完成后运行对应测试。所有提交完成、工作区干净后再进行第一次官方同步。

## 日常同步流程

已经发布或多人共享的定制分支使用 merge，不要 rebase 或 force push。merge 会保留官方更新边界，也避免每次同步都重放全部额度类型改造。

```bash
git status --short
git fetch upstream --prune
git switch custom/exact-billing
git switch -c integration/upstream-2026-07-23
git merge --no-ff upstream/main
```

只有工作区完全干净时才能开始。发生冲突后逐个理解双方语义，不要使用“全部采用 ours/theirs”。解决并验证后：

```bash
git add <resolved-files>
git commit
```

集成分支验证通过后，再将它合并回定制分支和发布分支：

```bash
git switch custom/exact-billing
git merge --no-ff integration/upstream-2026-07-23
git switch main
git merge --no-ff custom/exact-billing
```

分支名中的日期只用于示例，每次同步使用实际日期。

## 冲突处理优先级

### 1. 额度类型和换算

重点检查：

- 官方新加的 quota、amount、balance、pre-consume、refund 字段是否仍为 `int` 或数据库 `INT`。
- 官方新加的 `int(float64(...))`、`int(math.Round(...))`、`decimal.IntPart()` 是否直接进入扣费。
- 新增 JSON DTO、缓存结构、批量更新和日志汇总是否完整改为 `int64`。
- 前端是否把 quota 转为 32 位整数，或在求和、图表格式化时提前舍入。

所有 quota 换算继续使用 `common/quota_math.go` 的集中函数，并保持 `MaxQuota = Number.MAX_SAFE_INTEGER`。不要恢复官方旧的 `int32` 饱和边界。

### 2. 数据库结构

重点文件：

- `model/*.go` 中所有额度字段的 GORM 类型。
- `model/main.go` 的主库、关系型日志库和 ClickHouse 建表逻辑。
- 新增 model 的 quota 字段是否使用 `BIGINT`/`Int64`。

当前还在开发阶段，数据库只保留当前初始结构。**不要重新引入旧数据按比例放大、`schema_migrations` 标记、历史 SQL 或后台补迁任务**。官方主线如果增加新的字段，只需更新当前 GORM 模型，并在全新开发库上验证建表。数据库重建规则见[开发阶段数据库策略](./database.md)。

### 3. 结算链路

重点文件：

- `relay/helper/price.go`
- `service/text_quota.go`
- `service/quota.go`
- `service/billing_session.go`
- `service/upstream_billing.go`
- `service/log_info_generate.go`

官方若增加新的 relay 格式、分层计费或异步结算路径，需要判断它是否支持上游账单。如果支持，接入请求 ID、真实账单和差额结算；如果不支持，必须明确保留预估计费，不能仅因渠道已绑定账号就跳过正常扣费。

### 4. 渠道配置和缓存

重点检查 `dto.ChannelOtherSettings`、渠道增删改、渠道缓存、分发中间件和敏感字段脱敏。`UpstreamBilling` 必须在渠道缓存和 relay context 中保留，但 token 不能通过普通管理 API 明文返回前端。

### 5. 前端渠道表单

官方经常调整渠道编辑抽屉和 schema。合并时保证：

- 上游账号只在独立页签创建和维护。
- 绑定只在“编辑渠道 -> 精准计费”完成，并可选择“不绑定”。
- 编辑账号仍使用当前页抽屉，不跳转到渠道编辑器。
- 账号状态能显示完整错误原因，主动对账按钮仍可用。
- 新文案同步到全部 locale，不能只更新中英文。

## 合并后静态审计

以下搜索用于找出官方更新中新出现的高风险代码，不代表每一处命中都错误：

```bash
rg -n 'Quota\s+int\b|quota int\b|Amount\s+int\b|RemainQuota\s+int\b' --glob '*.go'
rg -n 'int\((math\.Round|decimal\.|.*QuotaPerUnit|.*quota)' --glob '*.go'
rg -n 'gorm:"[^"]*(type:int|default:)' model --glob '*.go'
rg -n 'UpstreamBilling|upstream_billing' dto model middleware relay service controller router web/src
rg -n 'QuotaPerUnit|MaxQuota|QuotaFromDecimal|QuotaRound' common model service relay
```

还应查看完整差异，而不仅是冲突文件：

```bash
git diff --stat custom/exact-billing...integration/upstream-2026-07-23
git diff custom/exact-billing...integration/upstream-2026-07-23 -- model service relay dto web/src/features/channels
git diff --check
```

## 验证矩阵

### 后端

```bash
go test ./common ./dto ./model ./service ./controller ./relay/...
go test ./...
```

至少覆盖：

- SQLite、MySQL 和 PostgreSQL 的全新建库；当前开发阶段不验证旧库数据转换。
- 主库与日志库分离，以及 ClickHouse 日志库。
- 新库 `QuotaPerUnit=100000000` 下的金额显示、余额、API Key 和图表汇总。
- 用户钱包、无限/有限 API Key、订阅、充值、兑换、退款和异步任务。
- New API 请求 ID 命中与 token/时间回退命中。
- Sub2API refresh token 轮换、两个渠道共享账号、API Key ID 解析。
- 上游账单未生成、401、404、超时、多条候选、失败后恢复和账单改价。
- 差额调整幂等，重复执行不会二次扣款或退款。

### 前端

优先使用项目约定的 Bun；环境没有 Bun 时才使用仓库现有 Node 工具链的等价命令。

```bash
cd web
bun run typecheck
bun run lint
bun run format:check
bun run build
```

手工回归桌面和移动宽度下的渠道列表、上游账号、渠道编辑、消费日志、API 密钥额度、仪表盘和系统额度设置。

## 开发库重建

当前分支不保留历史数据库迁移。更新代码后，在开发环境删除并重建主库和独立日志库/ClickHouse；不要尝试手工把旧 quota 乘以 200，也不要在旧库上继续验证金额。完整边界见[开发阶段数据库策略](./database.md)。

发布到正式环境前，必须先决定是否需要保留已有数据；那将是一个独立的生产迁移项目，不属于本次开发阶段改动。

## 每次同步记录

建议在合并提交或发布记录中固定写明：

- 同步前后的官方 commit。
- 冲突文件及采用的业务语义。
- 新增 quota 字段及其 `int64/BIGINT` 处理。
- 使用的是全新开发库还是正式数据迁移方案。
- New API/Sub2API 对账样例结果。
- 执行过的测试命令和未覆盖风险。
