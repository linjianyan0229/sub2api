#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
export SUB2API_UPDATE_LOCK_FILE="$scratch/update.lock"
export DOCKER_CALLS="$scratch/calls"
export MOCK_IMAGE='ccr.ccs.tencentyun.com/linjianyan/sub2api:latest'
export PATH="$scratch:$PATH"

cat >"$scratch/docker" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$DOCKER_CALLS"
case "$*" in
  *'config --format json') printf '{"services":{"sub2api":{"image":"%s"},"postgres":{"image":"postgres:18-alpine"},"redis":{"image":"redis:8-alpine"}}}\n' "$MOCK_IMAGE" ;;
  'pull '*) exit "${PULL_EXIT:-0}" ;;
  *'up -d '*) exit "${UP_EXIT:-0}" ;;
  *) exit 90 ;;
esac
MOCK
chmod +x "$scratch/docker"

run_update() {
  bash "$root/update-ccr.sh" -f 'external db.yml' --env-file 'secrets.env'
}

run_update
grep -Fx 'compose -f external db.yml --env-file secrets.env config --format json' "$DOCKER_CALLS"
grep -Fx "pull $MOCK_IMAGE" "$DOCKER_CALLS"
grep -Fx 'compose -f external db.yml --env-file secrets.env up -d --no-deps --pull never --wait --wait-timeout 180 sub2api' "$DOCKER_CALLS"
[[ $(wc -l <"$DOCKER_CALLS") -eq 3 ]]

: >"$DOCKER_CALLS"
export PULL_EXIT=42
if run_update; then echo '拉取失败时不应成功' >&2; exit 1; fi
! grep -q 'up -d' "$DOCKER_CALLS"
unset PULL_EXIT

: >"$DOCKER_CALLS"
export UP_EXIT=43
if run_update; then echo '健康检查失败应向调用者报错' >&2; exit 1; fi
unset UP_EXIT

for invalid in 'weishaw/sub2api:latest' 'ccr.ccs.tencentyun.com/linjianyan/sub2api:0.2.8'; do
  : >"$DOCKER_CALLS"
  if MOCK_IMAGE="$invalid" run_update; then echo '不应更新其他来源或固定版本' >&2; exit 1; fi
  [[ $(wc -l <"$DOCKER_CALLS") -eq 1 ]]
done

: >"$DOCKER_CALLS"
(
  flock -n 8
  run_update
) 8>"$SUB2API_UPDATE_LOCK_FILE"
[[ ! -s "$DOCKER_CALLS" ]]
echo '更新脚本验证通过：指定服务、参数保留、拉取/启动失败、来源限制、互斥。'
