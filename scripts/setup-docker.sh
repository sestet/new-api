#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname "$script_dir")
env_file="$project_dir/.env"

random_hex() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return
  fi
  if [ -r /dev/urandom ]; then
    od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
    return
  fi
  echo "无法生成安全随机密钥：请安装 openssl。" >&2
  exit 1
}

if ! command -v docker >/dev/null 2>&1; then
  echo "未找到 Docker，请先安装 Docker Engine 和 Compose 插件。" >&2
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  echo "未找到 docker compose，请先安装 Compose 插件。" >&2
  exit 1
fi

cd "$project_dir"

if [ ! -f "$env_file" ]; then
  umask 077
  temp_env=$(mktemp "${TMPDIR:-/tmp}/new-api-env.XXXXXX")
  trap 'rm -f "$temp_env"' EXIT HUP INT TERM

  image=${NEW_API_IMAGE:-tlabcode:20260730}
  port=${NEW_API_PORT:-8080}
  db_password=$(random_hex)
  redis_password=$(random_hex)
  session_secret=$(random_hex)

  {
    echo "NEW_API_IMAGE=$image"
    echo "NEW_API_PORT=$port"
    echo "DB_PASSWORD=$db_password"
    echo "REDIS_PASSWORD=$redis_password"
    echo "SESSION_SECRET=$session_secret"
  } >"$temp_env"

  mv "$temp_env" "$env_file"
  trap - EXIT HUP INT TERM
  echo "已生成 .env（权限仅当前用户可读写）。"
else
  echo "检测到已有 .env，保留现有配置。"
fi

docker compose config >/dev/null
docker compose up -d

port=$(sed -n 's/^NEW_API_PORT=//p' "$env_file" | tail -n 1)
port=${port:-8080}
echo "服务已启动：http://localhost:$port"
