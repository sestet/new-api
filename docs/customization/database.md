# 开发阶段数据库策略

当前分支仍处于开发阶段，数据库只保留一套“当前初始结构”，不维护历史版本迁移。

## 结构来源

- 主库结构由 `model/*.go` 的 GORM 模型定义。
- 启动时由 `model.initializeDBSchema()` 在全新数据库上创建主库表。
- 关系型日志库由 `model.initializeLogDBSchema()` 创建 `logs` 表。
- ClickHouse 日志库由 `model.initializeClickHouseLogDB()` 使用当前建表 SQL 创建。
- 不创建 `schema_migrations`、版本号表或后台补迁任务。

启动时不会对已有表执行 `AutoMigrate` 或 `ALTER TABLE`。数据库完全为空时，系统按当前 GORM 模型一次性创建全部表；如果只存在部分表，则启动失败并要求重建开发库。

## 不再保留的内容

- `bin/migration_v0.2-v0.3.sql`、`bin/migration_v0.3-v0.4.sql` 等历史 SQL。
- 旧字段类型转换和补列逻辑，例如额度列、`model_limits`、订阅金额字段和 ClickHouse quota 列转换。
- 旧前端配置、旧 Telegram 绑定、旧用户鉴权版本和上游账号健康状态的启动时回填。
- 并行的第二套数据库迁移入口。

## 更新代码后的操作

开发环境更新代码后，应删除并重建：

1. 主数据库；
2. 独立关系型日志数据库（如果配置了 `LOG_SQL_DSN`）；
3. ClickHouse 日志表（如果使用 ClickHouse）。

旧库中的 quota 整数不能与当前 `QuotaPerUnit=100000000` 和 `BIGINT/Int64` 结构混用。不要手工放大旧额度，也不要在旧库上验证精确计费结果。

## 正式上线前

正式上线并产生需要保留的数据后，不能继续使用“删除重建”的开发策略。届时应以当时的稳定提交为基线，单独设计可审计、可回滚、逐数据库验证的生产迁移，并明确数据备份和停机窗口。
