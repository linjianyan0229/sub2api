#!/usr/bin/env bash
# 在部署目录运行；参数原样传给 docker compose，例如 -f docker-compose.standalone.yml。
set -euo pipefail

readonly image='ccr.ccs.tencentyun.com/linjianyan/sub2api:latest'
command -v flock >/dev/null || { echo '缺少 flock，请安装 util-linux。' >&2; exit 1; }
command -v python3 >/dev/null || { echo '缺少 python3，请先安装。' >&2; exit 1; }
exec 9>"${SUB2API_UPDATE_LOCK_FILE:-/run/lock/sub2api-ccr-update.lock}"
flock -n 9 || { echo '已有镜像更新任务正在运行。'; exit 0; }

# config 会同时返回依赖服务，因此从 JSON 精确读取应用，且不输出其中的凭据。
configured_image=$(docker compose "$@" config --format json | python3 -c 'import json, sys; print(json.load(sys.stdin)["services"]["sub2api"]["image"])')
if [[ "$configured_image" != "$image" ]]; then
  echo "未更新：sub2api 必须使用 $image；固定版本请通过 Compose 手动维护。" >&2
  exit 1
fi

# 拉取失败时立即退出，正在运行的容器不受影响。
docker pull "$image"
# Compose 根据镜像 ID 判断是否重建；仅操作应用，沿用现有环境和数据卷。
docker compose "$@" up -d --no-deps --pull never --wait --wait-timeout 180 sub2api
