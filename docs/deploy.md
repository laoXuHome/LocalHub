# 部署与权限说明

本文件对应评估报告 v0.3.1 第 11 章。核心原则：**宿主机路径全由部署者配置，容器内路径固定**。

## 1. 两个宿主机路径

| 路径 | 谁决定 | 说明 |
|---|---|---|
| Compose 文件目录 | 部署者 | 放 `docker-compose.yml` 与 `.env` 的地方，任意位置 |
| 数据目录 | 部署者 | 通过 `.env` 的 `LOCALHUB_DATA_DIR` 指定，可与 compose 目录无关 |

示例（只是示例，不是约定）：

```bash
# compose 放在一个目录，数据放另一个盘
/volume1/docker/localhub/docker-compose.yml
/volume2/appdata/localhub/            <- LOCALHUB_DATA_DIR
```

```bash
# .env
LOCALHUB_DATA_DIR=/volume2/appdata/localhub
PUID=1000
PGID=1000
```

```bash
mkdir -p /volume2/appdata/localhub
cd /volume1/docker/localhub && docker compose up -d
```

## 2. 容器内固定路径

| 用途 | 容器内路径 |
|---|---|
| 数据根 | `/data` |
| 数据库 | `/data/localhub.db` |
| 图标缓存 | `/data/icons`（P2 起使用） |
| 备份 | `/data/backups` |
| 日志（如需落盘） | `/data/logs` |

应用代码只认 `/data`，不因 NAS 品牌或宿主目录差异产生分支。

## 3. 映射与安全边界

- 只挂载一个目录到 `/data`：`${LOCALHUB_DATA_DIR}:/data`。
- 不挂载宿主根目录、`/etc`、docker.sock 或无关目录。
- 数据目录解析为 realpath，必须位于 `LOCALHUB_DATA_ALLOWED_ROOT`（默认即 `/data`）之内；符号链接与 `../` 逃逸会被拒绝并拒绝启动。
- 数据库路径必须位于数据根之内。
- 备份目录固定 `/data/backups`（P6 再落地自定义路径，且仍须落在 `/data` 内）。

## 4. 权限（推荐方案，已选定，不需要部署者再做技术选择）

采用「入口自愈 + 运行期降权」：

1. 容器以 root 启动 `/localhub`（默认 init 角色）。
2. init 校验 `PUID/PGID` → 解析 `/data` → 创建 `icons/backups/logs` → 将 `/data` 顶层、子目录与数据库文件属主改为 `PUID:PGID`。
3. init 通过 `SysProcAttr.Credential` 以 `PUID:PGID` 启动 `--role=app`；**app 不是 root**。
4. init 作为 PID1 转发 `SIGTERM/SIGINT`，并以 app 的退出码退出。

因此：

- **不需要**手工 `chown` 整个数据目录；首次启动会自动修正（可选的 `chown -R 1000:1000 <数据目录>` 只是加速手段）。
- `PUID/PGID` 默认 `1000:1000`。若你的 NAS 用户不是 1000，改 `.env` 即可，入口会自动把属主改成新值（仅顶层与已知子目录；历史文件错乱时用一次性 `CHOWN_RECURSIVE=1`）。
- **不要写 `cap_drop: ALL`**：init 需要 `CAP_CHOWN`、`CAP_SETUID`、`CAP_SETGID`（Docker 默认具备）。
- 默认禁止 `PUID=0`。确需排障才设 `ALLOW_ROOT=1`；此时 `/health` 的 `process.is_root` 为 true，非 root 保证失效。

## 5. 验证

```bash
# 1. 健康状态
curl -s http://127.0.0.1:11030/health | python3 -m json.tool

# 2. 业务 UID 下的真实写权限（关键）
docker exec --user 1000:1000 localhub /localhub --selftest

# 3. 非属主应失败
docker exec --user 1001:1001 localhub /localhub --selftest; echo "exit=$?"

# 4. 容器健康检查（distroless 内无 curl，用二进制子命令）
docker exec localhub /localhub --healthcheck; echo "exit=$?"
```

`--selftest` 输出示例：

```json
{
  "data_writable": true,
  "verified": true,
  "uid": 1000, "gid": 1000, "is_root": false,
  "data_dir": "/data", "data_realpath": "/data", "allowed_root": "/data",
  "allowed_root_ok": true,
  "expected_puid": 1000, "expected_pgid": 1000,
  "reason": "",
  "version": "v0.1.0-p0", "commit": "..."
}
```

## 5.1 管理员与登录（P1.1）

首次启动需要用环境变量提供初始口令：

```bash
# .env
LOCALHUB_ADMIN_USERNAME=admin
LOCALHUB_ADMIN_PASSWORD=<12-72 字节的口令>
```

- 只有**管理员表为空**时才初始化；已存在管理员时该变量被忽略，不会覆盖现有口令。
- 初始化完成后建议从 `.env` 移除 `LOCALHUB_ADMIN_PASSWORD`，后续改密走界面/接口。
- 单管理员，不提供注册与多用户；口令以 bcrypt（cost 12）存库，库中不存在明文。
- 会话：服务端会话（`sessions` 表，令牌以 SHA-256 存库，Cookie 为 HttpOnly + SameSite=Lax），默认 12h 到期，退出即吊销。
- 通过 HTTPS 访问时设 `LOCALHUB_COOKIE_SECURE=1`；在反向代理后设 `LOCALHUB_TRUST_PROXY=1` 以按 X-Forwarded-For 识别客户端 IP。
- 登录失败限制：默认同一 IP+用户名 15 分钟内失败 5 次，锁定 15 分钟（`LOCALHUB_LOGIN_MAX_FAILURES/WINDOW/LOCKOUT`）。

公开与管理员边界：

| 路径 | 访客 | 管理员 |
|---|---|---|
| `/`、`/health`、`/api/auth/status` | 可访问 | 可访问 |
| `/api/auth/login` | 可访问（受限速） | — |
| `/api/auth/logout`、`/api/auth/password`、`/api/admin/*` | 401 | 可访问 |

## 6. 升级与重建

```bash
docker compose pull    # 或本地重新 build
docker compose up -d
```

属主写在宿主机磁盘上，不随容器消失；`PUID/PGID` 固定在 `.env` 中，重启与升级不会改变权限模型；init 每次启动做幂等校验。

## 7. 常见问题

| 现象 | 原因 | 处理 |
|---|---|---|
| 启动即退出，日志 `CHOWN_FAILED` | 缺少 `CAP_CHOWN` | 移除 `cap_drop: ALL`，保留三项目录能力 |
| 日志 `UID_MISMATCH` | 用 `--user` 启动但 UID 与 PUID 不一致 | 让两者一致，或改用 root 启动由入口降权 |
| `/health` 503，`data.writable=false` | 挂载只读或属主不对 | 检查挂载权限与 `PUID/PGID`，执行 `--selftest` 定位 |
| `--selftest` 返回“以 root 运行” | 用了 root 执行，无法代表业务 UID | 加 `--user 1000:1000` 重跑 |
