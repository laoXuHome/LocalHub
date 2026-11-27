#!/usr/bin/env bash
# T12：distroless + Go 入口 + 非 root 业务进程的十项专项验证。
# 用法：scripts/t12.sh <image> [platform]
# 退出码：0 = 全部通过；1 = 有用例失败（每条都会打印实际输出）。
set -uo pipefail

IMAGE="${1:-localhub:amd64}"
PLATFORM="${2:-linux/amd64}"
NAME="lh-t12-$$"
PORT=18080
TMP="$(mktemp -d)"
FAILED=0

log()  { printf '\n%s\n' "$*"; }
pass() { printf 'PASS | %-46s | %s\n' "$1" "$2"; }
fail() { printf 'FAIL | %-46s | %s\n' "$1" "$2"; FAILED=1; }

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker rm -f "${NAME}-2" >/dev/null 2>&1 || true
  rm -rf "$TMP"
}
trap cleanup EXIT

jget() { # $1=file $2=dotted.path
  python3 - "$1" "$2" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
for k in sys.argv[2].split('.'):
    d=d[int(k)] if isinstance(d,list) else d[k]
print(d)
PY
}
wait_health() { # $1=port $2=outfile $3=期望状态码（默认 200）
  local i code
  for i in $(seq 1 40); do
    code=$(curl -s -o "$2" -w '%{http_code}' "http://127.0.0.1:$1/health" || true)
    if [ "$code" = "${3:-200}" ]; then echo "$code"; return 0; fi
    sleep 0.5
  done
  echo "$code"; return 1
}

# ---------- T1：distroless 镜像可启动，且确实没有 Shell ----------
log "T1 distroless 启动 / 无 Shell"
if docker run --rm --platform "$PLATFORM" "$IMAGE" /localhub --version > "$TMP/version.txt" 2>&1; then
  pass "T1a --version 可执行" "$(head -1 "$TMP/version.txt")"
else
  fail "T1a --version 可执行" "$(tail -2 "$TMP/version.txt")"
fi
if docker run --rm --platform "$PLATFORM" --entrypoint /bin/sh "$IMAGE" -c true > "$TMP/shell.txt" 2>&1; then
  fail "T1b 镜像内不应存在 /bin/sh" "意外执行成功"
else
  pass "T1b 镜像内无 /bin/sh" "$(head -1 "$TMP/shell.txt")"
fi

# ---------- T2：init 修正 /data 属主 ----------
log "T2 init 修正 /data 属主"
mkdir -p "$TMP/data1"
chmod 0755 "$TMP/data1"
docker run -d --name "$NAME" --platform "$PLATFORM" \
  -p "${PORT}:11030" -v "$TMP/data1:/data" \
  -e PUID=1000 -e PGID=1000 -e TZ=Asia/Shanghai \
  "$IMAGE" > /dev/null || fail "T2 容器启动" "docker run 失败"
CODE=$(wait_health "$PORT" "$TMP/health.json" 200)
if [ "$CODE" = "200" ]; then pass "T2a 容器启动并 healthy" "HTTP 200"; else fail "T2a 容器启动并 healthy" "HTTP $CODE"; fi
OWNER=$(stat -c '%u:%g' "$TMP/data1")
if [ "$OWNER" = "1000:1000" ]; then pass "T2b /data 属主=1000:1000" "$OWNER"; else fail "T2b /data 属主=1000:1000" "实际 $OWNER"; fi
for d in icons backups logs; do
  O=$(stat -c '%u:%g' "$TMP/data1/$d")
  [ "$O" = "1000:1000" ] && pass "T2c $d 属主" "$O" || fail "T2c $d 属主" "实际 $O"
done

# ---------- T3/T4：app 实际 UID/GID 与 PUID/PGID 一致，且非 root ----------
log "T3/T4 app 进程身份"
UID_ACT=$(jget "$TMP/health.json" process.uid)
GID_ACT=$(jget "$TMP/health.json" process.gid)
ROOT=$(jget "$TMP/health.json" process.is_root)
[ "$UID_ACT" = "1000" ] && [ "$GID_ACT" = "1000" ] \
  && pass "T3 app uid/gid 与 PUID/PGID 一致" "uid=$UID_ACT gid=$GID_ACT" \
  || fail "T3 app uid/gid 与 PUID/PGID 一致" "uid=$UID_ACT gid=$GID_ACT"
[ "$ROOT" = "False" ] && pass "T4 app 非 root" "is_root=$ROOT" || fail "T4 app 非 root" "is_root=$ROOT"

# ---------- T5：缺少 CAP_CHOWN/SETUID/SETGID 时给出明确错误 ----------
log "T5 缺少能力时入口明确报错"
mkdir -p "$TMP/data2"; chmod 0755 "$TMP/data2"
docker run --rm --platform "$PLATFORM" --cap-drop ALL \
  -v "$TMP/data2:/data" -e PUID=1000 -e PGID=1000 "$IMAGE" > "$TMP/caps.txt" 2>&1
RC=$?
if [ "$RC" -ne 0 ] && grep -qE 'CHOWN_FAILED|CAP_|PRIVILEGE_DENIED|属主修正失败' "$TMP/caps.txt"; then
  pass "T5 能力缺失时非零退出且错误明确" "exit=$RC $(grep -oE 'CHOWN_FAILED|PRIVILEGE_DENIED|CAP_[A-Z]+' "$TMP/caps.txt" | head -1)"
else
  fail "T5 能力缺失时非零退出且错误明确" "exit=$RC $(tail -1 "$TMP/caps.txt")"
fi

# ---------- T6：PID1 接收并转发 SIGTERM ----------
log "T6 SIGTERM 转发"
docker kill -s TERM "$NAME" > /dev/null 2>&1
for i in $(seq 1 20); do
  RUNNING=$(docker inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null || echo false)
  [ "$RUNNING" = "false" ] && break
  sleep 0.5
done
EXIT=$(docker inspect -f '{{.State.ExitCode}}' "$NAME")
LOGS=$(docker logs "$NAME" 2>&1)
echo "$LOGS" | grep -q '转发给业务进程' \
  && pass "T6a init 记录并转发 SIGTERM" "日志含“转发给业务进程”" \
  || fail "T6a init 记录并转发 SIGTERM" "$(echo "$LOGS" | tail -2)"
[ "$EXIT" = "0" ] && pass "T6b 优雅退出码=0" "exit=$EXIT" || fail "T6b 优雅退出码=0" "exit=$EXIT"

# ---------- T7：子进程异常退出时容器退出码正确 ----------
log "T7 子进程失败时退出码透传"
mkdir -p "$TMP/data3"; chmod 0755 "$TMP/data3"
docker run --rm --platform "$PLATFORM" -v "$TMP/data3:/data" \
  -e PUID=1000 -e PGID=1000 -e LOCALHUB_HTTP_PORT=80 "$IMAGE" > "$TMP/fail.txt" 2>&1
RC=$?
[ "$RC" = "1" ] && pass "T7 子进程退出码透传" "容器 exit=1（非 root 绑定 80 端口失败）" \
  || fail "T7 子进程退出码透传" "exit=$RC $(tail -1 "$TMP/fail.txt")"

# ---------- T8：/data 不可写时健康状态反映异常 ----------
log "T8 不可写时 health=503 且 writable=false"
mkdir -p "$TMP/data4/icons" "$TMP/data4/backups" "$TMP/data4/logs"; chmod 0755 "$TMP/data4"
docker run -d --name "${NAME}-2" --platform "$PLATFORM" -p "18082:11030" \
  -v "$TMP/data4:/data:ro" --user 1000:1000 \
  -e PUID=1000 -e PGID=1000 -e WRITE_CHECK_FATAL=false "$IMAGE" > /dev/null
CODE=$(wait_health 18082 "$TMP/health-ro.json" 503)
W=$(jget "$TMP/health-ro.json" data.writable)
if [ "$CODE" = "503" ] && [ "$W" = "False" ]; then
  pass "T8 不可写时降级" "HTTP 503, writable=$W"
else
  fail "T8 不可写时降级" "HTTP $CODE, writable=$W"
fi

# ---------- T9：--selftest 在业务 UID 下验证写入 ----------
log "T9 selftest 以业务 UID 验证"
docker exec --user 1000:1000 "$NAME" /localhub --selftest > "$TMP/self-1000.json" 2>&1
RC=$?
if [ "$RC" -eq 0 ]; then pass "T9a PUID=1000 selftest 通过" "exit=0"; else fail "T9a PUID=1000 selftest 通过" "exit=$RC $(tail -1 "$TMP/self-1000.json")"; fi
docker exec --user 1001:1001 "$NAME" /localhub --selftest > "$TMP/self-1001.json" 2>&1
RC=$?
if [ "$RC" -ne 0 ]; then pass "T9b 非属主 UID selftest 失败" "exit=$RC"; else fail "T9b 非属主 UID 应失败" "exit=0（不应通过）"; fi
docker exec --user 1000:1000 "$NAME" /localhub --selftest 2>/dev/null | grep -q '"data_writable": true' \
  && pass "T9c selftest 输出 data_writable=true" "PUID 1000" \
  || fail "T9c selftest 输出 data_writable=true" "$(tail -1 "$TMP/self-1000.json")"

# ---------- T10：符号链接逃逸被拒绝 ----------
log "T10 符号链接逃逸"
ln -sfn /etc "$TMP/data1/escape"
docker exec --user 1000:1000 \
  -e LOCALHUB_DATA_DIR=/data/escape -e LOCALHUB_DATA_ALLOWED_ROOT=/data \
  "$NAME" /localhub --selftest > "$TMP/escape.json" 2>&1
RC=$?
if [ "$RC" -ne 0 ] && grep -q '越界' "$TMP/escape.json"; then
  pass "T10a 逃逸被拒绝" "exit=$RC"
else
  fail "T10a 逃逸被拒绝" "exit=$RC $(tail -1 "$TMP/escape.json")"
fi
docker run --rm --platform "$PLATFORM" -v "$TMP/data1:/data" \
  -e LOCALHUB_DATA_DIR=/data/escape -e LOCALHUB_DATA_ALLOWED_ROOT=/data \
  -e PUID=1000 -e PGID=1000 "$IMAGE" > "$TMP/escape-run.txt" 2>&1
RC=$?
[ "$RC" -ne 0 ] && grep -q 'DATA_PATH_INVALID' "$TMP/escape-run.txt" \
  && pass "T10b init 拒绝逃逸路径" "exit=$RC" \
  || fail "T10b init 拒绝逃逸路径" "exit=$RC $(tail -1 "$TMP/escape-run.txt")"

# ---------- A5：数据持久化（容器重建后仍在） ----------
log "A5 持久化验证"
docker rm -f "$NAME" >/dev/null 2>&1
docker run -d --name "$NAME" --platform "$PLATFORM" -p "${PORT}:11030" \
  -v "$TMP/data1:/data" -e PUID=1000 -e PGID=1000 "$IMAGE" > /dev/null
CODE=$(wait_health "$PORT" "$TMP/health2.json" 200)
SV=$(jget "$TMP/health2.json" database.schema_version)
if [ "$CODE" = "200" ] && [ -f "$TMP/data1/localhub.db" ] && [ "$SV" = "1" ]; then
  pass "A5 重建后数据仍在" "db 存在, schema_version=$SV"
else
  fail "A5 重建后数据仍在" "HTTP $CODE, schema_version=$SV"
fi

log "===== T12 汇总 ====="
if [ "$FAILED" -eq 0 ]; then
  echo "RESULT: ALL PASS"
  exit 0
else
  echo "RESULT: FAILED"
  exit 1
fi
