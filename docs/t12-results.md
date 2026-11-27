# T12 专项验证记录（P0）

脚本：`scripts/t12.sh`（可用 `bash scripts/t12.sh <镜像> <平台>` 复现）。
执行环境：GitHub Actions `ubuntu-latest` + Docker + QEMU(binfmt arm64)。
本次执行：run [45979386270](https://github.com/laoXuHome/LocalHub/actions/runs/45979386270)，镜像 `localhub:amd64`（平台 `linux/amd64`），构建提交 `1a3fb9a3280cefb66dca2b64be76277677391180`。

汇总：**21 条断言全部 PASS，RESULT: ALL PASS**。

## 实测输出

```
T1 distroless 启动 / 无 Shell
PASS | T1a --version 可执行          | localhub v0.1.0-p0 commit=1a3fb9a3... build_time=2026-11-27T10:07:44Z
PASS | T1b 镜像内无 /bin/sh          | OCI runtime create failed: exec: "/bin/sh": stat /bin/sh: no such file or directory

T2 init 修正 /data 属主
PASS | T2a 容器启动并 healthy         | HTTP 200
PASS | T2b /data 属主=1000:1000       | 1000:1000
PASS | T2c icons 属主                 | 1000:1000
PASS | T2c backups 属主               | 1000:1000
PASS | T2c logs 属主                  | 1000:1000

T3/T4 app 进程身份
PASS | T3 app uid/gid 与 PUID/PGID 一致 | uid=1000 gid=1000
PASS | T4 app 非 root                   | is_root=False

T5 缺少能力时入口明确报错
PASS | T5 能力缺失时非零退出且错误明确 | exit=1 CAP_CHOWN

T6 SIGTERM 转发
PASS | T6a init 记录并转发 SIGTERM     | 日志含“转发给业务进程”
PASS | T6b 优雅退出码=0                 | exit=0

T7 子进程失败时退出码透传
PASS | T7 子进程退出码透传              | 容器 exit=1（非法监听地址导致 app 启动失败）
PASS | T7b 容器重启后恢复服务            | HTTP 200

T8 不可写时 health=503 且 writable=false
PASS | T8 不可写时降级                  | HTTP 503, writable=False

T9 selftest 以业务 UID 验证
PASS | T9a PUID=1000 selftest 通过      | exit=0
PASS | T9b 非属主 UID selftest 失败      | exit=1
PASS | T9c selftest 输出 data_writable=true | PUID 1000

T10 符号链接逃逸
PASS | T10a 逃逸被拒绝                   | exit=1
PASS | T10b init 拒绝逃逸路径            | exit=1

A5 持久化验证
PASS | A5 重建后数据仍在                 | db 存在, schema_version=1

===== T12 汇总 =====
RESULT: ALL PASS
```

## 十项验证与实现对应

| 项 | 要求 | 做法 | 结论 |
| --- | --- | --- | --- |
| T1 | distroless 镜像可正常启动 `/localhub` | `--version` 正常输出；`docker run ... /bin/sh` 被 OCI 拒绝 | 通过 |
| T2 | init 正确处理 `/data` 权限 | 宿主机目录以 root 属主挂载，init 改为 `1000:1000`，子目录同步 | 通过 |
| T3 | app 实际 UID/GID 与 PUID/PGID 一致 | `/health` 与 `/proc` 对照，均为 1000 | 通过 |
| T4 | app 非 root | `process.is_root=false`，app 角色内再次拒绝 root | 通过 |
| T5 | 缺 CAP_CHOWN/SETUID/SETGID 时明确报错 | `--cap-drop` 后 init exit=1，日志指明 `CAP_CHOWN` | 通过 |
| T6 | PID1 接收并转发 SIGTERM/SIGINT | `docker kill -s TERM`，init 记录转发、子进程优雅退出、容器 ExitCode=0 | 通过 |
| T7 | 子进程正常/异常退出时容器退出码正确 | 非法监听地址使 app 启动失败 → 容器 exit=1；正常退出 → 0 | 通过 |
| T8 | `/data` 不可写时自检失败且健康异常 | 只读挂载 + `WRITE_CHECK_FATAL=false` → `/health` 503、`writable=false` | 通过 |
| T9 | `--selftest` 以业务 UID 验证写入 | `docker exec --user 1000:1000 /localhub --selftest` exit 0；`--user 1001:1001` exit 1 | 通过 |
| T10 | 符号链接逃逸与越界路径被拒绝 | `/data/escape -> /etc`：selftest 与 init 均 exit=1（`DATA_PATH_INVALID`） | 通过 |
| A5 | 附加：重建后数据仍在 | 删除容器后同一数据目录重建，`schema_version=1` | 通过 |

## `--selftest` 输出字段（PUID=1000 场景）

```
{"data_writable":true,"verified":true,"uid":1000,"gid":1000,"is_root":false,
 "data_dir":"/data","data_realpath":"/data","allowed_root":"/data","allowed_root_ok":true,
 "expected_puid":1000,"expected_pgid":1000,"reason":"","version":"v0.1.0-p0","commit":"..."}
```

root 执行且 `PUID≠0` 时 `verified=false` 并 exit 1，避免“root 能写就认定业务能写”。

## arm64（QEMU 模拟）

```
arm64 /health HTTP 200
{"status":"ok","version":"v0.1.0-p0","database":{"status":"ok","schema_version":1},
 "data":{"writable":true,"path":"/data"},"process":{"role":"app","uid":1000,"gid":1000,"is_root":false}, ...}
exit_code=0   （docker kill -s TERM 后）
```

说明：arm64 通过 QEMU 在 x86 runner 上模拟运行，非真机 ARM 硬件；已验证启动、健康检查、信号与退出码。
