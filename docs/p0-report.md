# LocalHub P0 交付说明

依据：《LocalHub 技术可行性评估报告 v0.3.1》＋《LocalHub P0 施工任务书》。
范围：只建项目基础设施与“空服务”，不含任何 P1 及后续业务功能。

## 一、交付内容

| 项 | 内容 |
| --- | --- |
| 仓库 | https://github.com/laoXuHome/LocalHub （public，main 分支） |
| 验证提交 | `1a3fb9a3280cefb66dca2b64be76277677391180` |
| CI 运行 | https://github.com/laoXuHome/LocalHub/actions/runs/45979386270 |
| 验证产物 | Actions artifact `p0-verification-1a3fb9a...`（image-inspect.json、images.txt、version-amd64/arm64.txt、t12.log、health-arm64.json、health-compose.json、compose-config.txt、compose-ps.txt） |
| 验收对照 | [docs/p0-acceptance.md](p0-acceptance.md) |
| T12 记录 | [docs/t12-results.md](t12-results.md) |
| 部署说明 | [docs/deploy.md](deploy.md) |

## 二、关键文件

```
cmd/localhub/main.go                入口：--role=init|app、--selftest、--version、--healthcheck
internal/config/config.go           环境配置与集中校验（非法配置一次性报错退出）
internal/initrole/initrole.go       init：路径校验、属主修正、SysProcAttr 降权、信号转发、退出码透传
internal/approle/approle.go         app：非 root 二次校验、写自检、/health、静态资源
internal/datadir/datadir.go         EvalSymlinks + 允许根 Within 校验、目录创建、属主修正、可写检查
internal/db/{db.go,migrate.go}      纯 Go SQLite 连接与版本化迁移（schema_migrations）
internal/db/migrations/0001_init.sql 空初始化迁移（app_meta）
internal/logging/logging.go         结构化 JSON 日志
Dockerfile                          node 构建前端 → golang 静态编译 → distroless/static-debian12
docker-compose.yml / .env.example   部署模板（容器内固定 /data，宿主机路径可配）
.github/workflows/ci.yml            Go 质量 / 前端构建 / 多架构镜像 + T12 + compose 烟测
scripts/t12.sh                      T12 十项 ＋ 持久化验证脚本
web/                                React 18 + TypeScript + Vite
```

## 三、镜像与构建结果（CI，ubuntu-latest）

| 镜像 | 架构 | 大小 | 验证 |
| --- | --- | --- | --- |
| `localhub:amd64` | linux/amd64 | 20.9 MB | `--version` 正常；完整 T12（21 条断言全 PASS）；compose 启动 + `/health` 200 |
| `localhub:arm64` | linux/arm64 | 4.89 MB | QEMU 下启动、`/health` 200、SIGTERM 后 `ExitCode=0` |

基础镜像 `gcr.io/distroless/static-debian12`，无 Shell、无包管理器；镜像内仅 `/localhub`，默认 `--role=init`。

compose 实测：`docker compose up -d` → 容器 `localhub` Up（`0.0.0.0:18090->11030`）→ `/health` HTTP 200 → `docker compose down`。

`/health` 实测 body（compose 场景）：

```json
{"status":"ok","version":"v0.1.0-p0","database":{"status":"ok","schema_version":1,"path":"/data/localhub.db"},
 "data":{"path":"/data","allowed_root":"/data","writable":true,"last_check":"...","error":""},
 "process":{"role":"app","uid":1000,"gid":1000,"is_root":false},"tz":"Asia/Shanghai"}
```

## 四、已知问题

1. **arm64 为 QEMU 模拟验证**，未在真机 ARM NAS 上运行过；真机验收需部署者执行一次 `docker compose up -d` 与 `--selftest`。
2. **未在真实 NAS（Unraid / 飞牛 / NAS 共享目录）实测**；SMB/NFS 挂载下的属主与权限行为需现场确认。
3. T12 只对 amd64 完整执行；arm64 只做启动、健康与信号验证（QEMU 下跑完 T12 成本过高）。
4. 前端目前是占位首页，无业务 UI（P0 范围内允许）。
5. 镜像未推送 registry：任务书只要求构建成功与运行验证，部署时需本地 `docker build` 或指定 `LOCALHUB_IMAGE`。
6. compose 健康检查 `interval 30s / start_period 15s`，容器刚启动的十几秒内可能显示 `health: starting`。
7. `/data/logs` 目录已创建但 P0 未实现文件日志轮转，日志输出到 stdout（JSON 行）。
8. T12 脚本在 CI 中使用 `sudo` 创建符号链接（`/data` 属主为 PUID，runner 用户无权限），仅影响测试脚本，不影响产品。
9. 版本号由构建参数注入，当前固定 `v0.1.0-p0`；版本策略留给后续批次。

## 五、P1 施工建议（仅建议，未实施）

1. 账户：单管理员口令 + 服务端会话（Cookie + SameSite），口令哈希存 SQLite；不引入多用户与 RBAC。
2. 书签 CRUD 与导入导出：先做 Netscape HTML 导入，再做 JSON 导出；表结构在迁移 `0002` 中加入。
3. 图标抓取：域名 → favicon，落 `/data/icons`，必须做 SSRF 防护（禁私网/回环/重定向跳转）与尺寸/超时限制。
4. LAN/WAN 探测：以请求来源与可配置判定规则对 `/health` 之外的 `context` 接口暴露，规则写入 `app_meta`。
5. Docker 发现：优先只读 Socket Proxy（不做业务接入），先做容器列表与状态映射。
6. 备份：打包 `/data` 内白名单（db + icons + 配置）到 `/data/backups`，带校验和与保留份数。
7. V-01 / V-02 门禁按报告定义的验证项单独执行，不与业务功能混批。
8. 工程增强（非业务）：镜像推 GHCR、ARM 真机验证、日志文件轮转、健康信息增加磁盘余量与备份年龄指标。
