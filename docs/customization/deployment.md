# Docker Compose 部署

生产 Compose 使用本地镜像、PostgreSQL 和 Redis。数据库密码、Redis 密码及会话密钥只保存在 Git 忽略的 `.env`，不会写进 `docker-compose.yml` 或提交历史。

## 首次部署

先把源码和已经构建或导入的本地镜像放到服务器，然后执行：

```bash
chmod +x scripts/setup-docker.sh
./scripts/setup-docker.sh
```

脚本会：

1. 检查 Docker Engine 和 `docker compose` 插件。
2. 在 `.env` 不存在时生成三个 32 字节随机密钥，并将文件权限限制为当前用户。
3. 校验最终 Compose 配置。
4. 使用 `docker-compose.yml` 启动服务。

默认镜像为 `tlabcode:20260730`，访问端口为 `8080`。首次运行前可覆盖：

```bash
NEW_API_IMAGE=tlabcode:20260730 NEW_API_PORT=8080 ./scripts/setup-docker.sh
```

脚本检测到已有 `.env` 时不会覆盖，因此升级或重复启动不会导致数据库密码和 `SESSION_SECRET` 变化。

如果服务器已经使用旧版 Compose 创建了 `pg_data`，请先手动建立 `.env`，并把原来的 PostgreSQL、Redis 和会话密钥分别填入 `DB_PASSWORD`、`REDIS_PASSWORD`、`SESSION_SECRET`，再运行脚本。脚本只管理 Compose 配置，不会修改现有 PostgreSQL 数据目录内部保存的密码。

## 重新构建

在 AMD64 服务器本机从当前源码构建：

```bash
docker build -t tlabcode:20260730 .
./scripts/setup-docker.sh
```

在 Apple Silicon Mac 上为 AMD64 服务器构建并导出：

```bash
docker buildx build --platform linux/amd64 -t tlabcode:20260730 --output type=docker .
docker save tlabcode:20260730 -o tlabcode-20260730-amd64.tar
```

将镜像包和项目文件传到服务器后：

```bash
docker load -i tlabcode-20260730-amd64.tar
./scripts/setup-docker.sh
```

## 配置说明

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `NEW_API_IMAGE` | `tlabcode:20260730` | Compose 使用的本地镜像标签 |
| `NEW_API_PORT` | `8080` | 对外监听端口 |
| `DB_PASSWORD` | 自动生成 | PostgreSQL 密码；使用 URL 安全的十六进制字符串 |
| `REDIS_PASSWORD` | 自动生成 | Redis 密码；使用 URL 安全的十六进制字符串 |
| `SESSION_SECRET` | 自动生成 | 登录与会话签名密钥，升级时必须保持不变 |

不要删除已投入使用的 `.env`。修改 `DB_PASSWORD` 而不同时修改数据库内密码会导致无法连接；修改 `SESSION_SECRET` 会使现有登录会话失效。
