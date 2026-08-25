#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
bundle_dir="${1:-$(dirname -- "$script_dir")}"
bundle_dir="$(CDPATH= cd -- "$bundle_dir" && pwd)"

env_file="$bundle_dir/.env.offline"
compose_file="$bundle_dir/compose.yaml"
offline_compose_file="$bundle_dir/compose.offline.yaml"
archive_file="$bundle_dir/images.tar"
hash_file="$bundle_dir/images.tar.sha256"
profiles_file="$bundle_dir/profiles.txt"
ollama_volume_file="$bundle_dir/ollama-volume.txt"

for file in "$env_file" "$compose_file" "$offline_compose_file" "$archive_file" "$hash_file"; do
  if [ ! -f "$file" ]; then
    echo "离线包缺少文件：$file" >&2
    exit 1
  fi
done

command -v docker >/dev/null 2>&1 || { echo "找不到 docker 命令" >&2; exit 1; }
docker info >/dev/null

if command -v sha256sum >/dev/null 2>&1; then
  actual_hash="$(sha256sum "$archive_file" | awk '{print tolower($1)}')"
elif command -v shasum >/dev/null 2>&1; then
  actual_hash="$(shasum -a 256 "$archive_file" | awk '{print tolower($1)}')"
else
  echo "找不到 sha256sum 或 shasum，无法校验镜像包" >&2
  exit 1
fi
expected_hash="$(awk '{print tolower($1)}' "$hash_file")"
if [ "$actual_hash" != "$expected_hash" ]; then
  echo "镜像包 SHA256 校验失败：期望 $expected_hash，实际 $actual_hash" >&2
  exit 1
fi

docker load -i "$archive_file"

ollama_archive="$bundle_dir/ollama-data.tgz"
ollama_volume="iot-platform_ollama-data"
if [ -f "$ollama_volume_file" ]; then
  ollama_volume="$(head -n 1 "$ollama_volume_file" | tr -d '\r')"
fi
if [ -f "$ollama_archive" ]; then
  if docker volume inspect "$ollama_volume" >/dev/null 2>&1; then
    echo "检测到已有 $ollama_volume，跳过 Ollama 模型恢复以避免覆盖现有数据。"
  else
    docker volume create "$ollama_volume" >/dev/null
    docker run --rm \
      --mount "type=volume,source=$ollama_volume,target=/dst" \
      --mount "type=bind,source=$bundle_dir,target=/backup" \
      alpine:3.22 sh -ec 'tar -xzf /backup/ollama-data.tgz -C /dst'
  fi
fi

export COMPOSE_PROJECT_NAME=iot-platform
compose=(docker compose --env-file "$env_file" -f "$compose_file" -f "$offline_compose_file")
if [ -f "$profiles_file" ]; then
  while IFS= read -r profile; do
    [ -n "$profile" ] && compose+=(--profile "$profile")
  done < "$profiles_file"
fi

"${compose[@]}" config --quiet
"${compose[@]}" up -d --no-build --pull never
"${compose[@]}" ps

api_port="$(awk -F= '$1 == "IOT_API_PORT" {print $2; exit}' "$env_file" | tr -d "\r\"'")"
api_port="${api_port:-8081}"
health_url="http://127.0.0.1:${api_port}/health/live"
if command -v curl >/dev/null 2>&1; then
  healthy=0
  for _ in $(seq 1 60); do
    if curl --fail --silent --show-error "$health_url" >/dev/null 2>&1; then
      healthy=1
      break
    fi
    sleep 2
  done
  if [ "$healthy" -ne 1 ]; then
    "${compose[@]}" logs --tail=100 platform-api postgres redpanda emqx || true
    echo "平台健康检查失败：$health_url" >&2
    exit 1
  fi
  echo "平台健康检查通过：$health_url"
else
  echo "未安装 curl，已跳过 HTTP 健康检查；请手动访问 $health_url。"
fi

echo "离线部署完成。Web 默认地址：http://127.0.0.1:8080"
