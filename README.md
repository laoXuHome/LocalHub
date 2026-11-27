# LocalHub

本地优先的 NAS 书签与服务面板。当前处于 **P0 基础设施阶段**：只交付一个可通过 Docker Compose 启动的空服务，不含任何产品业务功能。

- 设计依据：《LocalHub 技术可行性评估报告 v0.3.1》
- 准入批次：P0（项目基础建设）
- 后端：Go 1.23 + 纯 Go SQLite（modernc.org/sqlite，无 CGO）
- 前端：React 18 + TypeScript + Vite，构建产物 embed 进 Go 二进制
- 镜像：`gcr.io/distroless/static-debian12`，单二进制双角色（init / app），无 Shell、无包管理器

## 快速开始

```bash
cp .env.example .env
# 编辑 .env：至少改 LOCALHUB_DATA_DIR 为你自己的宿主机目录
docker compose up -d --build
curl -s http://127.0.0.1:11030/health | python3 -m json.tool
```

期望：`"status": "ok"`、`database.status == "ok"`、`data.writable == true`、`process.uid == 1000`、`process.is_root == false`。

## 配置

| 环境变量 | 默认 | 说明 |
|---|---|---|
| `PUID` / `PGID` | 1000 / 1000 | 业务进程 UID/GID；入口会把 `/data` 属主修正为该值 |
| `ALLOW_ROOT` | 0 | 仅排障用；设为 1 才允许 PUID=0，并会在日志与 `/health` 标注 |
| `TZ` | Asia/Shanghai | 时区 |
| `LOCALHUB_DATA_DIR` | `/data` | **容器内**数据根，固定约定，不要改 |
| `LOCALHUB_DATA_ALLOWED_ROOT` | 同 `LOCALHUB_DATA_DIR` | 允许访问的根，用于拒绝符号链接逃逸 |
| `LOCALHUB_DB_PATH` | `/data/localhub.db` | 必须位于数据根之内 |
| `LOCALHUB_HTTP_ADDR` / `LOCALHUB_HTTP_PORT` | 0.0.0.0 / 11030 | 监听地址与端口 |
| `LOCALHUB_LOG_LEVEL` | info | debug / info / warn / error |
| `WRITE_CHECK_FATAL` | 1 | 启动写自检失败时是否退出（0 则 degraded 运行，用于排障） |
| `WRITE_CHECK_INTERVAL` | 60s | 周期性写自检间隔 |
| `CHOWN_RECURSIVE` | 0 | 仅当历史属主错乱需要一次性递归修正时设为 1 |

宿主机路径（`LOCALHUB_DATA_DIR` 冒号左侧、compose 文件位置）全部由部署者决定，仓库中不写死任何 NAS 实际路径。详见 [docs/deploy.md](docs/deploy.md)。

## 目录结构

```
cmd/localhub/           入口：init / app / --selftest / --healthcheck / --version
internal/config/        配置加载与校验（非法配置启动即失败）
internal/datadir/       /data 解析、越界与符号链接校验、属主修正、写自检
internal/initrole/      PID1：修属主 → Credential 降权启动 app → 转发信号 → 透传退出码
internal/approle/       app：写自检、SQLite + 迁移、/health、静态页面
internal/db/            纯 Go SQLite 连接与版本化迁移
internal/static/        前端产物 embed
web/                    Vite + React + TS（P0 仅占位页）
scripts/t12.sh          T12 十项专项验证脚本
.github/workflows/      CI：测试、前端构建、多架构镜像、T12、arm64 运行验证
```

## 验证命令

```bash
# 健康检查（容器内自检，无需 curl）
docker exec localhub /localhub --healthcheck; echo $?

# 以“业务进程真实身份”验证 /data 可写（必须在业务 UID 下执行）
docker exec --user 1000:1000 localhub /localhub --selftest

# 反向验证：非属主 UID 应当失败
docker exec --user 1001:1001 localhub /localhub --selftest; echo $?
```

> 以 root 执行 `--selftest` 会返回非零并说明“无法证明业务 UID 的写权限”，这是刻意设计：root 能写不代表 PUID/PGID 能写。

## 本地开发

```bash
# 后端（无需前端产物即可编译，embed 内有占位页）
go build ./cmd/localhub
LOCALHUB_DATA_DIR=/tmp/localhub-data PUID=$(id -u) PGID=$(id -g) ./localhub

# 前端热更新（/health 代理到 11030）
cd web && npm install && npm run dev
```

## P0 边界（不做）

账户/登录/权限、书签 CRUD 与导入导出、图标抓取、首页布局与编辑模式、LAN/WAN 判断、Docker 发现与 Socket Proxy、备份恢复与 WebDAV、通知中心。这些属于 P1–P7，本批不得出现。

## 文档

- [部署与权限说明](docs/deploy.md)
- [P0 验收对照表](docs/p0-acceptance.md)
- [T12 专项验证记录](docs/t12-results.md)
