#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
project_root="$(CDPATH= cd -- "$script_dir/.." && pwd)"

output_dir="offline-bundles"
env_file=""
include_ai=0
include_harness=0
include_thingspanel=0
include_gb26875=0
full=0
ollama_model="qwen3:8b"
skip_ollama_model=0

usage() {
  cat <<'EOF'
用法：
  package-offline.sh [选项]

选项：
  --output-dir DIR       输出父目录，默认 offline-bundles
  --env-file FILE        使用已有正式环境配置；不传则自动生成随机密钥
  --include-ai           打包 Ollama + Weaviate
  --include-harness      打包 DeepSeek Harness
  --include-thingspanel  打包 ThingsPanel
  --include-gb26875      部署时同时启动 GB/T 26875 网关
  --ollama-model MODEL   需要一起打包的 Ollama 模型，默认 qwen3:8b
  --skip-ollama-model    只打包 AI 镜像，不打包模型卷
  --full                 启用全部可选组件
  -h, --help             显示帮助
EOF
}

die() {
  echo "错误：$*" >&2
  exit 1
}

run_docker() {
  echo "> docker $*" >&2
  docker "$@"
}

random_hex() {
  local bytes="${1:-24}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$bytes" | tr -d '\r\n'
  else
    od -An -N "$bytes" -tx1 /dev/urandom | tr -d ' \r\n'
  fi
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print tolower($1)}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print tolower($1)}'
  else
    die "找不到 sha256sum 或 shasum，无法校验镜像包"
  fi
}

env_value() {
  local key="$1"
  local file="$2"
  sed -n "s/^${key}=//p" "$file" | tail -n 1 | tr -d '\r'
}

set_env_value() {
  local file="$1"
  local key="$2"
  local value="$3"
  local tmp
  tmp="$(mktemp "${file}.tmp.XXXXXX")"
  awk -v key="$key" -v value="$value" '
    BEGIN { replaced = 0 }
    $0 ~ "^[[:space:]]*" key "[[:space:]]*=" {
      if (!replaced) { print key "=" value; replaced = 1 }
      next
    }
    { print }
    END { if (!replaced) print key "=" value }
  ' "$file" > "$tmp"
  mv "$tmp" "$file"
}

validate_env() {
  local file="$1"
  local key value
  local required_keys=(
    POSTGRES_PASSWORD REDIS_PASSWORD CLICKHOUSE_PASSWORD
    MINIO_ROOT_PASSWORD MINIO_DR_ROOT_PASSWORD IOT_JWT_SECRET
    IOT_ADMIN_USER IOT_ADMIN_PASSWORD IOT_ADMIN_TENANTS
    IOT_VIDEO_PLATFORM_SECRETS IOT_BACKUP_ADMIN_TOKEN IOT_VIDEO_ZLM_SECRET
    EMQX_DASHBOARD_USER EMQX_DASHBOARD_PASSWORD
    GRAFANA_ADMIN_USER GRAFANA_ADMIN_PASSWORD
  )
  for key in "${required_keys[@]}"; do
    value="$(env_value "$key" "$file")"
    [[ -n "${value//[[:space:]]/}" ]] || die "EnvFile 缺少必填安全配置：$key"
  done
  if grep -Eq '^[A-Za-z_][A-Za-z0-9_]*=.*(change-this|local-iot-|admin123|public-change-me|change-me)' "$file"; then
    die "EnvFile 仍包含示例密码或默认密钥，请先替换后再打包"
  fi
}

write_env() {
  local destination="$1"
  local generated=0
  if [[ -n "$env_file" ]]; then
    [[ -f "$env_file" ]] || die "指定的 EnvFile 不存在：$env_file"
    cp "$env_file" "$destination"
  else
    generated=1
    local postgres_password="pg-$(random_hex 18)"
    local redis_password="redis-$(random_hex 18)"
    local clickhouse_password="ch-$(random_hex 18)"
    local minio_password="minio-$(random_hex 18)"
    local minio_dr_password="minio-dr-$(random_hex 18)"
    local jwt_secret="$(random_hex 32)"
    local admin_password="Admin-$(random_hex 12)"
    local video_secret="$(random_hex 24)"
    local zlm_secret="$(random_hex 24)"
    local harness_token="$(random_hex 32)"
    local backup_token="$(random_hex 32)"
    local emqx_password="Emqx-$(random_hex 12)"
    local grafana_password="Grafana-$(random_hex 12)"
    local ollama_url=""
    local ai_provider=""
    local weaviate_url=""
    local harness_url=""
    if (( include_ai )); then
      ollama_url="http://ollama:11434"
      ai_provider="ollama"
      weaviate_url="http://weaviate:8080"
    fi
    if (( include_harness )); then
      harness_url="http://deepseek-harness:8091"
    fi
    cat > "$destination" <<EOF
# 自动生成的离线部署配置，请限制此文件权限。
POSTGRES_PASSWORD=$postgres_password
REDIS_PASSWORD=$redis_password
CLICKHOUSE_PASSWORD=$clickhouse_password
MINIO_ROOT_USER=iotadmin
MINIO_ROOT_PASSWORD=$minio_password
MINIO_DR_ROOT_USER=iotdradmin
MINIO_DR_ROOT_PASSWORD=$minio_dr_password
IOT_JWT_SECRET=$jwt_secret
IOT_ADMIN_USER=admin
IOT_ADMIN_PASSWORD=$admin_password
IOT_ADMIN_TENANTS=tenant_001
IOT_VIDEO_PLATFORM_SECRETS=video-platform-1:$video_secret
IOT_VIDEO_MEDIA_ALLOWED_HOSTS=
IOT_VIDEO_PREVIEW_ALLOWED_ORIGINS=
IOT_VIDEO_PREVIEW_CSP_SOURCES=
IOT_VIDEO_ZLM_API_URL=http://zlm:80
IOT_VIDEO_ZLM_PLAYBACK_BASE_URL=http://localhost:8090
IOT_VIDEO_ZLM_SECRET=$zlm_secret
IOT_VIDEO_ZLM_VHOST=__defaultVhost__
IOT_VIDEO_ZLM_APP=iot
IOT_ZLM_IMAGE=zlmediakit/zlmediakit:master
IOT_OLLAMA_URL=$ollama_url
IOT_OLLAMA_MODEL=$ollama_model
IOT_AI_PROVIDER=$ai_provider
IOT_AI_BASE_URL=
IOT_AI_MODEL=
IOT_AI_API_KEY=
IOT_AI_PROVIDER_TEST_ALLOWED_ORIGINS=http://ollama:11434
IOT_AI_OLLAMA_URL=http://ollama:11434
DEEPSEEK_API_KEY=
IOT_AI_HARNESS_URL=$harness_url
IOT_AI_HARNESS_TOKEN=$harness_token
IOT_AI_HARNESS_MCP_URL=http://platform-api:8080/mcp/harness
IOT_AI_HARNESS_MODEL=deepseek-v4-flash
IOT_AI_HARNESS_TIMEOUT=90s
IOT_WEAVIATE_URL=$weaviate_url
IOT_BACKUP_ADMIN_TOKEN=$backup_token
IOT_BACKUP_INTERVAL=24h
IOT_BACKUP_INCREMENTAL_INTERVAL=15m
IOT_MQTT_WEBSOCKET_PUBLIC_URL=
IOT_THINGSPANEL_URL=
IOT_THINGSPANEL_USER=
IOT_THINGSPANEL_PASSWORD=
IOT_WEB_PORT=8080
IOT_API_PORT=8081
IOT_CORS_ALLOWED_ORIGINS=http://localhost:8080,http://127.0.0.1:8080
EMQX_DASHBOARD_USER=admin
EMQX_DASHBOARD_PASSWORD=$emqx_password
GRAFANA_ADMIN_USER=admin
GRAFANA_ADMIN_PASSWORD=$grafana_password
EOF
    cat > "$(dirname -- "$destination")/OFFLINE-CREDENTIALS.txt" <<EOF
# 离线部署凭据
# 请将本文件视为密码文件，不要提交 Git 或公开传输。
平台管理员：admin
平台管理员密码：$admin_password
备份服务 Token：$backup_token
EMQX Dashboard：admin / $emqx_password
Grafana：admin / $grafana_password
PostgreSQL 密码：$postgres_password
Redis 密码：$redis_password
ClickHouse 密码：$clickhouse_password
MinIO 主密码：$minio_password
MinIO 灾备密码：$minio_dr_password
ZLMediaKit API Secret：$zlm_secret
EOF
  fi

  set_env_value "$destination" IOT_PLATFORM_API_IMAGE iot-platform-api:offline
  set_env_value "$destination" IOT_PLATFORM_WEB_IMAGE iot-platform-web:offline
  set_env_value "$destination" IOT_BACKUP_IMAGE iot-platform-backup:offline
  set_env_value "$destination" IOT_DEEPSEEK_HARNESS_IMAGE iot-deepseek-harness:offline
  set_env_value "$destination" IOT_THINGSPANEL_BACKEND_IMAGE iot-thingspanel-backend:offline
  set_env_value "$destination" IOT_THINGSPANEL_WEB_IMAGE iot-thingspanel-web:offline

  if (( ! generated )); then
    cat > "$(dirname -- "$destination")/OFFLINE-CREDENTIALS.txt" <<EOF
凭据来自外部 EnvFile：$env_file
本文件不复制外部 EnvFile 的内容，请单独保管原始凭据。
EOF
  fi
  validate_env "$destination"
  printf '%s\n' "$generated"
}

get_image_id_for_service() {
  local service="$1"
  local id
  id="$(docker compose --profile thingspanel images -q "$service" 2>/dev/null | head -n 1 || true)"
  if [[ -z "$id" ]]; then
    id="$(docker image ls \
      --filter label=com.docker.compose.project=iot-platform \
      --filter label=com.docker.compose.service="$service" \
      --format '{{.ID}}' 2>/dev/null | head -n 1 || true)"
  fi
  if [[ -z "$id" ]]; then
    id="$(docker image inspect "iot-platform-$service" --format '{{.Id}}' 2>/dev/null || true)"
  fi
  [[ -n "$id" ]] || die "无法找到 ThingsPanel 服务 $service 构建出的镜像"
  printf '%s\n' "$id"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output-dir) output_dir="${2:-}"; shift 2 ;;
    --env-file) env_file="${2:-}"; shift 2 ;;
    --include-ai) include_ai=1; shift ;;
    --include-harness) include_harness=1; shift ;;
    --include-thingspanel) include_thingspanel=1; shift ;;
    --include-gb26875) include_gb26875=1; shift ;;
    --ollama-model) ollama_model="${2:-}"; shift 2 ;;
    --skip-ollama-model) skip_ollama_model=1; shift ;;
    --full) full=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数：$1；使用 --help 查看帮助" ;;
  esac
done

if (( full )); then
  include_ai=1
  include_harness=1
  include_thingspanel=1
  include_gb26875=1
fi

command -v docker >/dev/null 2>&1 || die "找不到 docker 命令"
docker info >/dev/null || die "Docker Engine 不可用，请先启动 Docker"
command -v git >/dev/null 2>&1 || die "找不到 git 命令"

if [[ -n "$env_file" && "$env_file" != /* ]]; then
  env_file="$project_root/$env_file"
fi
if [[ "$output_dir" = /* ]]; then
  output_parent="$output_dir"
else
  output_parent="$project_root/$output_dir"
fi
mkdir -p "$output_parent"
bundle_root="$output_parent/iot-platform-offline-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$bundle_root"

export COMPOSE_PROJECT_NAME=iot-platform
export IOT_PLATFORM_API_IMAGE=iot-platform-api:offline
export IOT_PLATFORM_WEB_IMAGE=iot-platform-web:offline
export IOT_BACKUP_IMAGE=iot-platform-backup:offline
export IOT_DEEPSEEK_HARNESS_IMAGE=iot-deepseek-harness:offline

profiles=()
compose_profile_args=()
add_profile() {
  profiles+=("$1")
  compose_profile_args+=(--profile "$1")
}
(( include_ai )) && add_profile ai
(( include_harness )) && add_profile harness
(( include_thingspanel )) && add_profile thingspanel
(( include_gb26875 )) && add_profile gb26875

pull_services=(
  zlm postgres postgres-wal-init redis minio minio-dr redpanda redpanda-init
  clickhouse emqx prometheus grafana loki
)
run_docker compose pull "${pull_services[@]}"
run_docker compose build --pull platform-api platform-web backup-service

ollama_archive=""
ollama_volume_name=""
if (( include_ai )); then
  run_docker compose --profile ai pull ollama weaviate
  if (( ! skip_ollama_model )); then
    run_docker compose --profile ai up -d ollama
    ollama_ready=0
    for _ in $(seq 1 30); do
      if docker compose --profile ai exec -T ollama ollama list >/dev/null 2>&1; then
        ollama_ready=1
        break
      fi
      sleep 2
    done
    (( ollama_ready )) || die "Ollama 容器未在规定时间内就绪"
    run_docker compose --profile ai exec -T ollama ollama pull "$ollama_model"
    ollama_volume_name="$(docker volume ls --filter label=com.docker.compose.volume=ollama-data --format '{{.Name}}' | head -n 1)"
    ollama_volume_name="${ollama_volume_name:-iot-platform_ollama-data}"
    run_docker run --rm \
      --mount "type=volume,source=$ollama_volume_name,target=/src,readonly" \
      --mount "type=bind,source=$bundle_root,target=/backup" \
      alpine:3.22 sh -ec 'tar -czf /backup/ollama-data.tgz -C /src .'
    ollama_archive="ollama-data.tgz"
  fi
fi

if (( include_harness )); then
  run_docker compose --profile harness build --pull deepseek-harness
fi

thingspanel_images=()
if (( include_thingspanel )); then
  run_docker compose --profile thingspanel pull thingspanel-postgres thingspanel-db-init
  run_docker compose --profile thingspanel build --pull backend thingspanel
  backend_id="$(get_image_id_for_service backend)"
  thingspanel_web_id="$(get_image_id_for_service thingspanel)"
  run_docker tag "$backend_id" iot-thingspanel-backend:offline
  run_docker tag "$thingspanel_web_id" iot-thingspanel-web:offline
  thingspanel_images+=(iot-thingspanel-backend:offline iot-thingspanel-web:offline)
fi

mkdir -p "$bundle_root/scripts"
generated_credentials="$(write_env "$bundle_root/.env.offline")"
cp "$project_root/compose.yaml" "$bundle_root/"
cp "$project_root/compose.offline.yaml" "$bundle_root/"
cp -R "$project_root/deploy" "$bundle_root/"
cp "$project_root/docs/OFFLINE_DEPLOYMENT.md" "$bundle_root/"
for script_name in deploy-offline.ps1 deploy-offline-windows.ps1 deploy-offline.sh deploy-offline-linux.sh deploy-offline-macos.sh; do
  cp "$script_dir/$script_name" "$bundle_root/scripts/"
done
if [[ -n "$ollama_volume_name" ]]; then
  printf '%s\n' "$ollama_volume_name" > "$bundle_root/ollama-volume.txt"
fi

resolved_images_text="$(docker compose "${compose_profile_args[@]}" config --images)"
offline_images=(iot-platform-api:offline iot-platform-web:offline iot-platform-backup:offline)
(( include_harness )) && offline_images+=(iot-deepseek-harness:offline)
offline_images+=("${thingspanel_images[@]}")
all_images_text="$(printf '%s\n' "$resolved_images_text" "${offline_images[@]}" | awk 'NF && !seen[$0]++' | sort -u)"
images=()
while IFS= read -r image; do
  [[ -n "$image" ]] && images+=("$image")
done <<< "$all_images_text"
(( ${#images[@]} > 0 )) || die "没有解析出可导出的镜像"
for image in "${images[@]}"; do
  docker image inspect "$image" >/dev/null || die "镜像不存在，无法导出：$image"
done

archive_path="$bundle_root/images.tar"
run_docker save -o "$archive_path" "${images[@]}"
archive_hash="$(sha256_file "$archive_path")"
printf '%s  images.tar\n' "$archive_hash" > "$bundle_root/images.tar.sha256"
printf '%s\n' "${profiles[@]}" > "$bundle_root/profiles.txt"

json_array() {
  local separator=""
  printf '['
  for value in "$@"; do
    printf '%s"%s"' "$separator" "$value"
    separator=,
  done
  printf ']'
}

commit="$(git -C "$project_root" rev-parse HEAD 2>/dev/null || printf 'unknown')"
profiles_json="$(json_array "${profiles[@]}")"
images_json="$(json_array "${images[@]}")"
ollama_model_json="null"
ollama_archive_json="null"
ollama_volume_json="null"
if [[ -n "$ollama_archive" ]]; then
  ollama_model_json="\"$ollama_model\""
  ollama_archive_json="\"$ollama_archive\""
  ollama_volume_json="\"$ollama_volume_name\""
fi
cat > "$bundle_root/manifest.json" <<EOF
{
  "format": 1,
  "project": "iot-platform",
  "createdAtUtc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "gitCommit": "${commit//$'\n'/}",
  "profiles": $profiles_json,
  "images": $images_json,
  "imageArchive": "images.tar",
  "imageArchiveSha256": "$archive_hash",
  "envFile": ".env.offline",
  "composeFiles": ["compose.yaml", "compose.offline.yaml"],
  "ollamaModel": $ollama_model_json,
  "ollamaArchive": $ollama_archive_json,
  "ollamaVolume": $ollama_volume_json,
  "generatedCredentials": $([[ "$generated_credentials" = 1 ]] && echo true || echo false)
}
EOF

echo ""
echo "离线包已生成：$bundle_root"
echo "镜像数量：${#images[@]}"
du -h "$archive_path" | awk '{print "镜像包大小：" $1}'
echo "部署方式：按服务器系统运行 scripts/deploy-offline-windows.ps1、deploy-offline-macos.sh 或 deploy-offline-linux.sh"
if [[ "$generated_credentials" = 1 ]]; then
  echo "自动生成的凭据：$bundle_root/OFFLINE-CREDENTIALS.txt"
fi
