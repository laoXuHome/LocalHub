# T12 专项验证记录（P0，含 T5 逐项能力与 T6 SIGINT 补充）

脚本：`scripts/t12.sh <image> <platform>`，可用 `T12_ARTIFACT_DIR=<dir>` 保留每个用例的原始日志。
执行环境：GitHub Actions `ubuntu-latest` + Docker + QEMU(binfmt arm64)。

- 代码提交 `1e6a4340b030d1707bbdea36ed8e5d50378d983d`
- 验证运行 [45983204744](https://github.com/laoXuHome/LocalHub/actions/runs/45983204744)
- 主镜像 `localhub:amd64`（20.9 MB），平台 `linux/amd64`
- 汇总：**28 条断言全部 PASS，RESULT: ALL PASS**
- 原始日志：artifact `p0-verification-1e6a434.../t12-logs/`（cap-CAP_CHOWN.log、cap-CAP_SETUID.log、cap-CAP_SETGID.log、caps.txt、sigint.log）

## 一、T5 逐项能力验证（三项独立用例）

每项只丢弃**一个** capability，其余保持 Docker 默认，实际退出码与错误代码如下。

### 5a 仅移除 CAP_CHOWN（保留 SETUID/SETGID）

```
docker run --rm --cap-drop CHOWN -v <dir>:/data -e PUID=1000 -e PGID=1000 <image>
```

| 项 | 实测 |
| --- | --- |
| 退出码 | `1` |
| 错误代码 | `CHOWN_FAILED` |
| 关键日志 | `{"level":"ERROR","msg":"属主修正失败","error":"修正属主 \"/data\" -> 1000:1000 失败：chown /data: operation not permitted","code":"CHOWN_FAILED","hint":"需要 CAP_CHOWN；如已 cap_drop: ALL，请保留 CHOWN/SETUID/SETGID 或改用匹配属主挂载"}` |
| 结论 | 属主修正失败被精确归因到 CAP_CHOWN，PASS |

### 5b 仅移除 CAP_SETUID（保留 CHOWN/SETGID）

| 项 | 实测 |
| --- | --- |
| 退出码 | `1` |
| 错误代码 | `PRIVILEGE_DENIED` |
| 前序行为 | `属主修正完成 uid=1000 gid=1000`（CHOWN 正常） |
| 关键日志 | `{"level":"ERROR","msg":"业务进程启动失败","error":"fork/exec /localhub: operation not permitted","code":"PRIVILEGE_DENIED","hint":"降权失败：切换到 PUID/PGID 需要 CAP_SETUID 与 CAP_SETGID，两者缺一不可；请勿使用 cap_drop: ALL"}` |
| 结论 | 降权失败被明确报告并指明所需能力，PASS |

### 5c 仅移除 CAP_SETGID（保留 CHOWN/SETUID）

| 项 | 实测 |
| --- | --- |
| 退出码 | `1` |
| 错误代码 | `PRIVILEGE_DENIED` |
| 前序行为 | `属主修正完成 uid=1000 gid=1000` |
| 关键日志 | 同 5b：`fork/exec /localhub: operation not permitted`，`code=PRIVILEGE_DENIED`，hint 指明 `CAP_SETUID 与 CAP_SETGID` |
| 结论 | PASS |

### 5d 全部能力移除（原有粗粒度用例，保留）

| 项 | 实测 |
| --- | --- |
| 退出码 | `1` |
| 错误代码 | `DATA_DIR_CREATE_FAILED`（无 CAP_DAC_OVERRIDE，连子目录都建不出） |
| 结论 | PASS。该用例只作整体兜底，**不作为三项能力验证的替代** |

> 说明：Docker 只能按 capability 丢弃，无法从内核错误中区分“缺 SETUID”与“缺 SETGID”（两者同报 EPERM），因此 5b/5c 的错误码同为 `PRIVILEGE_DENIED`，错误提示同时列出两项必需能力；能力归因由用例的 drop 参数确定。

## 二、T6 信号验证（SIGTERM 保留 ＋ SIGINT 独立）

### T6a/T6b SIGTERM（原有，保留）

```
PASS | T6a init 记录并转发 SIGTERM | 日志含“转发给业务进程”
PASS | T6b 优雅退出码=0             | exit=0
```

### T6c–T6f SIGINT（独立容器，端口 18083）

| 项 | 实测 |
| --- | --- |
| 前置 | 容器启动后 `/health` HTTP 200 |
| init 日志 | `{"msg":"收到信号，转发给业务进程","signal":"interrupt","app_pid":12}` |
| app 日志 | `{"msg":"收到信号，开始优雅关闭","signal":"interrupt"}` → `{"msg":"HTTP 服务已关闭"}` |
| init 收尾 | `{"msg":"业务进程正常退出","exit_code":0}` |
| 容器状态 | `stopped=yes`，`ExitCode=0`（≤15s 内退出） |
| 结论 | T6d/T6e/T6f 全部 PASS，退出码与“优雅处理信号后正常退出”一致 |

完整日志：artifact `t12-logs/sigint.log`。

## 三、完整 T12 输出（本次运行）

```
T1 distroless 启动 / 无 Shell
PASS | T1a --version 可执行          | localhub v0.1.0-p0 commit=1e6a434... build_time=2026-11-27T10:35:01Z
PASS | T1b 镜像内无 /bin/sh          | exec: "/bin/sh": stat /bin/sh: no such file or directory

T2 init 修正 /data 属主
PASS | T2a 容器启动并 healthy         | HTTP 200
PASS | T2b /data 属主=1000:1000       | 1000:1000
PASS | T2c icons 属主                 | 1000:1000
PASS | T2c backups 属主               | 1000:1000
PASS | T2c logs 属主                  | 1000:1000

T3/T4 app 进程身份
PASS | T3 app uid/gid 与 PUID/PGID 一致 | uid=1000 gid=1000
PASS | T4 app 非 root                   | is_root=False

T5 能力缺失逐项验证（独立用例，互不替代）
PASS | T5 仅缺 CAP_CHOWN                | exit=1 code=CHOWN_FAILED 命中=CAP_CHOWN
PASS | T5 仅缺 CAP_SETUID               | exit=1 code=PRIVILEGE_DENIED 命中=CAP_SETUID
PASS | T5 仅缺 CAP_SETGID               | exit=1 code=PRIVILEGE_DENIED 命中=CAP_SETGID
PASS | T5 全部能力移除时非零退出         | exit=1 DATA_DIR_CREATE_FAILED

T6 SIGTERM 转发
PASS | T6a init 记录并转发 SIGTERM     | 日志含“转发给业务进程”
PASS | T6b 优雅退出码=0                 | exit=0

T6 SIGINT 转发（独立容器）
PASS | T6c SIGINT 前置：服务已就绪      | HTTP 200
PASS | T6d init 收到并转发 SIGINT       | signal=interrupt
PASS | T6e app 收到转发的 SIGINT        | signal=interrupt
PASS | T6f SIGINT 后优雅退出，exit=0     | stopped=yes exit=0

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

## 四、十二项映射

| 项 | 要求 | 覆盖用例 | 结论 |
| --- | --- | --- | --- |
| T1 | distroless 可启动、无 Shell | T1a、T1b | PASS |
| T2 | init 修正 `/data` 权限 | T2a–T2c | PASS |
| T3 | app UID/GID = PUID/PGID | T3 | PASS |
| T4 | app 非 root | T4 | PASS |
| T5 | 缺能力时明确报错（逐项） | T5 × 4 | PASS |
| T6 | PID1 转发 SIGTERM / SIGINT | T6a–T6f | PASS |
| T7 | 退出码透传 | T7、T7b | PASS |
| T8 | 不可写时健康降级 | T8 | PASS |
| T9 | `--selftest` 在业务 UID 下验证 | T9a–T9c | PASS |
| T10 | 符号链接/越界路径拒绝 | T10a、T10b | PASS |
| A5 | 重建后数据仍在（附加） | A5 | PASS |

## 五、`--selftest` 输出字段（PUID=1000）

```json
{"data_writable":true,"verified":true,"uid":1000,"gid":1000,"is_root":false,
 "data_dir":"/data","data_realpath":"/data","allowed_root":"/data","allowed_root_ok":true,
 "expected_puid":1000,"expected_pgid":1000,"reason":"","version":"v0.1.0-p0","commit":"..."}
```

root 执行且 `PUID≠0` 时 `verified=false` 并 exit 1，避免“root 能写就认定业务能写”。

## 六、arm64（QEMU 模拟）

```
arm64 /health HTTP 200
{"status":"ok","database":{"status":"ok","schema_version":1},"data":{"writable":true},
 "process":{"role":"app","uid":1000,"gid":1000,"is_root":false}, ...}
exit_code=0   （docker kill -s TERM 后）
```

arm64 为 QEMU 模拟运行，非真机 ARM 硬件；已验证启动、健康检查、SIGTERM 与退出码。
