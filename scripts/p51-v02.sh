#!/usr/bin/env bash
# P5.1 V-02：通过 LocalHub 使用的 Docker Socket Proxy 发起真实写请求。
# 任何写操作成功（HTTP < 400）都会使脚本失败。只读 /version 与 /containers/json
# 必须可用，未授权路径（镜像列表、容器日志）必须被拒绝。
set -u

BASE_URL="${P51_DOCKER_PROXY_URL:-http://127.0.0.1:12375}"
OUT_DIR="${P51_ARTIFACT_DIR:-./p51-v02}"
mkdir -p "$OUT_DIR"
MATRIX="$OUT_DIR/v02-matrix.txt"
: > "$MATRIX"

pass=0
fail=0
request() {
  method="$1"
  path="$2"
  body="$3"
  name="$4"
  expect="$5"
  if [ -n "$body" ]; then
    code="$(curl -sS --max-time 10 -X "$method" -H 'Content-Type: application/json' \
      --data "$body" -o "$OUT_DIR/$name.body" -w '%{http_code}' "$BASE_URL$path" || echo 000)"
  else
    code="$(curl -sS --max-time 10 -X "$method" \
      -o "$OUT_DIR/$name.body" -w '%{http_code}' "$BASE_URL$path" || echo 000)"
  fi
  printf '%-6s %-52s -> %s %s\n' "$method" "$path" "$code" "$(head -c 140 "$OUT_DIR/$name.body" 2>/dev/null | tr '\n' ' ')" | tee -a "$MATRIX"
  case "$expect" in
    deny)
      if [ "$code" -ge 400 ] 2>/dev/null; then pass=$((pass+1)); else fail=$((fail+1)); fi
      ;;
    allow)
      if [ "$code" = "200" ]; then pass=$((pass+1)); else fail=$((fail+1)); fi
      ;;
  esac
}

echo '=== P5.1 V-02 write requests (must be denied) ===' | tee -a "$MATRIX"
request POST   /containers/create '{"Image":"alpine","Cmd":["true"]}' create deny
request POST   /containers/p51fixture/start '' start deny
request POST   /containers/p51fixture/stop '' stop deny
request POST   /containers/p51fixture/restart '' restart deny
request DELETE /containers/p51fixture '' delete deny
request POST   /containers/p51fixture/exec '{"AttachStdout":true,"Cmd":["sh"]}' exec deny
request DELETE /images/alpine:latest '' image-delete deny
request POST   /networks/create '{"Name":"p51net"}' network-create deny
request POST   /networks/p51net/connect '{"Container":"p51fixture"}' network-connect deny
request POST   /v1.52/containers/p51fixture/start '' versioned-start deny
request DELETE /v1.52/images/alpine:latest '' versioned-image-delete deny

echo '=== read-only allowlist checks ===' | tee -a "$MATRIX"
request GET /version '' version allow
request GET /containers/json '' containers-list allow
request GET /images/json '' images-list-deny deny
request GET /containers/p51fixture/logs '' container-logs-deny deny

{
  echo "pass=$pass fail=$fail"
  echo "base_url=$BASE_URL"
  echo "checked_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
} | tee -a "$MATRIX"

if [ "$fail" -ne 0 ]; then
  echo "V-02 FAIL: write or allowlist expectation was not met" >&2
  exit 1
fi
echo "V-02 PASS: all write requests denied; read allowlist as expected"
