# LocalHub P0 验收对照表

对应文档：《LocalHub 技术可行性评估报告 v0.3.1》＋《LocalHub P0 施工任务书》。

验证环境：GitHub Actions `ubuntu-latest` runner（自带 Docker 24+/buildx/QEMU）。开发机（Linux 云主机）无 Docker，因此**全部容器级验证在 CI runner 内实际执行**，日志与产物可在 Actions 运行记录中复核。

- 验证运行（含补充验证）：[run 45983204744](https://github.com/laoXuHome/LocalHub/actions/runs/45983204744)（代码提交 `1e6a4340b030d1707bbdea36ed8e5d50378d983d`）
- 产物：`p0-verification-1e6a434...`（image-inspect.json、t12.log、t12-logs/cap-*.log、t12-logs/sigint.log、health-arm64.json、health-compose.json、compose-config.txt 等）

## 一、P0 验收标准逐项对照

| # | 验收项 | 结论 | 证据 |
| --- | --- | --- | --- |
| 1 | Docker Compose 可正常启动空服务 | 通过 | CI 步骤「Docker Compose 启动与健康检查」：`docker compose up -d` → 容器 `localhub` Up → `docker compose down` 正常；`compose-up.txt` |
| 2 | `/health` 返回正确状态 | 通过 | compose 场景 `HTTP 200`，body 中 `status=ok`、`database.status=ok`、`schema_version=1`、`data.writable=true`、`process.uid=1000`、`is_root=false`；`health-compose.json` |
| 3 | SQLite 可连接、空迁移可执行 | 通过 | `schema_version=1`（迁移 `0001_init.sql` 已执行）；A5 重建容器后仍为 1；`internal/db` 单测通过 |
| 4 | `/data` 持久化有效，容器重建后数据仍在 | 通过 | T12-A5：删除容器后用同一宿主机目录重建，`localhub.db` 存在且 `schema_version=1` |
| 5 | PUID/PGID 权限机制实际生效 | 通过 | T2b `/data` 属主 `1000:1000`；T2c `icons/backups/logs` 属主 `1000:1000`；T3 app `uid=1000 gid=1000` |
| 6 | 主业务进程默认非 root | 通过 | T4 `is_root=False`；健康信息 `process.is_root=false`；代码在 app 角色内再次拒绝 root（纵深防御） |
| 7 | `/data` 写自检与 `--selftest` 通过 | 通过 | T9a `docker exec --user 1000:1000 /localhub --selftest` → exit 0、`data_writable=true`；T9b 非属主 1001 失败 exit 1 |
| 8 | T12 十项验证全部完成并有记录 | 通过 | `scripts/t12.sh` **28 条断言全 PASS**；T5 为 3 项独立能力用例 + 1 项整体兜底，T6 为 SIGTERM 与 SIGINT 两套独立用例，见 [T12 记录](t12-results.md) 与 `t12.log`、`t12-logs/` |
| 9 | amd64 / arm64 镜像构建成功 | 通过 | `localhub:amd64`（20.9MB）、`localhub:arm64`（4.89MB），`image-inspect.json` 记录 `Os/Arch` |
| 10 | 两种架构均完成可运行性验证 | 通过（arm64 为 QEMU 模拟） | amd64：完整 T12 原生执行；arm64：QEMU 下容器启动、`/health` HTTP 200、SIGTERM 后 `ExitCode=0`、`--version` 输出正确 |
| 11 | CI 全部通过 | 通过 | run 45979386270：Go 质量与测试 ✓、前端构建 ✓、多架构镜像与 T12 验证 ✓ |
| 12 | 没有夹带 P1 或后续业务功能 | 通过 | 代码中无账户/会话/书签/图标抓取/Dashboard/LAN-WAN/Docker 发现/Socket Proxy/备份恢复/通知中心任何路由与表；数据库仅 `schema_migrations`（迁移元数据）与 `app_meta`（空初始化表），无业务表 |
| 13 | 文档、Compose 示例与实际代码一致 | 通过 | `.env.example`、`docker-compose.yml`、`docs/deploy.md` 的变量名/端口/路径与 `internal/config` 一致；`--selftest` 命令按实现输出 |

## 二、施工范围核对（第二章）

| 范围项 | 结论 | 落点 |
| --- | --- | --- |
| 前后端目录结构 | 完成 | `cmd/localhub`、`internal/*`、`web/`、`docs/`、`scripts/`、`bin/`(CI) |
| Go 技术栈（已批准） | 完成 | Go 1.23 + `modernc.org/sqlite`（纯 Go，无 CGO） |
| React + TS + Vite 前端 | 完成 | `web/`，`npm run build` → `dist/` |
| 前端产物嵌入 Go 二进制 | 完成 | `internal/static` 用 `//go:embed dist`；Dockerfile 构建阶段覆盖占位页 |
| 统一配置/日志/测试/构建/文档目录 | 完成 | `internal/config`、`internal/logging`、`*_test.go`、`Dockerfile`、`docs/` |
| `.env.example` 与 compose 示例 | 完成 | 仓库根 |
| 多阶段构建 + distroless + 双架构 | 完成 | `Dockerfile`；`gcr.io/distroless/static-debian12` |
| 单一 `/localhub`，默认 init 角色 | 完成 | `cmd/localhub/main.go` |
| 镜像版本信息 | 完成 | `--version` 与 `/health.version/commit/build_time`（ldflags 注入） |
| PUID/PGID 默认 1000、参数校验 | 完成 | `internal/config`、`internal/initrole` |
| 默认禁 root，`ALLOW_ROOT=1` 例外并告警 | 完成 | init 校验 + app 角色二次拒绝 |
| init 检查/修正 `/data` 属主 | 完成 | `internal/datadir.FixOwnership` |
| `SysProcAttr.Credential` 降权，不 setuid | 完成 | `internal/initrole` 仅通过 `exec.Cmd` 凭据字段降权 |
| SIGTERM/SIGINT 转发、回收、退出码透传 | 完成 | T6a/T6b/T7 |
| 不引入 tini | 完成 | 镜像内仅 `/localhub` |
| `/data` 固定、`/data/backups` 固定 | 完成 | 配置与 `EnsureDirs` |
| 宿主机路径由部署者配置、不写死 | 完成 | compose 变量 `${LOCALHUB_DATA_DIR}`；代码内无 NAS 路径 |
| 越界/符号链接逃逸拒绝 | 完成 | `datadir.Resolve` + T10a/T10b |
| 配置项与默认值、非法配置报错 | 完成 | `internal/config` + 单测 `TestInvalidConfigIsRejected` |
| 结构化日志 | 完成 | JSON 行日志（time/level/msg/context） |
| `/health` 字段完整（含 `data_writable`/`data_last_check`） | 完成 | `internal/approle`，字段名 `data.writable`、`data.last_check` |
| 不可写时不伪装健康 | 完成 | T8：HTTP 503 + `writable=false` |
| 纯 Go SQLite + 连接管理 + 版本化迁移 | 完成 | `internal/db`、`internal/db/migrations/0001_init.sql` |
| 首次/重复启动、迁移重复执行 | 完成 | 迁移幂等（`schema_migrations` 版本表 + 事务）；A5 重建后仍为 1 |
| 启动写自检 + `--selftest` JSON | 完成 | `cmd/localhub`，字段见 T12 记录 |
| `--selftest` 在业务 UID 下验证 | 完成 | CI 用 `docker exec --user 1000:1000`；root 且 PUID≠0 时判定 `verified=false` 并 exit 1 |
| GitHub Actions 全链路 | 完成 | `.github/workflows/ci.yml`（go / web / image+T12 三 job） |

## 三、验证覆盖范围说明（如实标注）

| 维度 | 覆盖情况 |
| --- | --- |
| T5 能力缺失 | 逐项独立验证：仅缺 CAP_CHOWN → `CHOWN_FAILED`；仅缺 CAP_SETUID / 仅缺 CAP_SETGID → `PRIVILEGE_DENIED` 并指明所需能力；另保留 `--cap-drop ALL` 兜底用例 |
| T6 信号 | SIGTERM（原有，保留）与 SIGINT（独立容器）各一组；SIGINT 用例校验 init 转发、app 优雅关闭、容器在超时内退出且 `ExitCode=0` |
| amd64 | 原生完整执行 T12 全部 28 条断言 + Docker Compose 启动与健康检查 |
| arm64 | QEMU 模拟运行：启动、`/health` 200、SIGTERM 后 `ExitCode=0`、`--version`；未在真机 ARM 硬件上验证 |
| 真实 NAS | 未执行；宿主机文件系统/共享目录权限行为未实测，需部署者现场验收 |

## 四、明确禁止事项核对（第四章）

以下内容在本批中均**不存在**：管理员账户与登录会话权限、书签 CRUD 与导入导出与图标抓取、首页 Dashboard 与布局拖拽编辑、LAN/WAN 自动判断、Docker 容器发现/关联/状态、Docker Socket Proxy 接入、备份/恢复/WebDAV、通知中心与完整系统设置、V-01/V-02 正式验证。

数据库仅有 `schema_migrations`（迁移框架元数据）与 `app_meta`（0001 迁移建立的基础表，未写入业务数据），无任何业务表。HTTP 路由仅 `/health` 与静态资源（含 SPA 回退）。

## 五、结论

P0 验收标准 13 项全部通过；施工范围与禁止事项核对无偏差。**P0 可提交验收**，未启动 P1。
