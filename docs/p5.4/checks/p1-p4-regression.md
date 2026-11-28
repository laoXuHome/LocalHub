# P1–P4 核心回归结果

真实 Compose + 真实 Docker 环境，CI job `P5.4 AMD64 集成验收`。

## P1 权限

- 访客 `GET /api/admin/whoami` → 401。
- 访客 `GET /api/bookmarks` → 200。
- 访客 `GET /api/auth/status` → 200。
- 访客 `POST /api/admin/bookmarks` → 401。
- 管理员登录 → 200；登录后管理员接口可用；退出后 401；错误密码 401。
- 证据：`ci/p54/p1-auth.json`、`ci/auth-smoke.txt`。

## P2 书签管理

- 创建 `P5.4 回归书签` → 201。
- 管理员详情 LAN/WAN 地址保留。
- 修改描述 → 200。
- 公开列表可见。
- 删除 → 200，公开列表移除。
- 图标上传/读取/重启后读取/SSRF 拦截/删除清理通过。
- 导入预览/执行、HTML/JSON 导出、访客拒绝通过。
- 证据：`ci/p54/p2-p3-bookmarks-layout.json`、`ci/icon-smoke.txt`、`ci/transfer-smoke.txt`。

## P3 首页

- `GET /api/bookmarks/layout` → 200，`desktop`/`mobile` 结构存在。
- 管理员保存当前布局 → 200，重新读取结构不变。
- 首页访客截图无 Docker 状态/卡片/入口。
- 书签网格、排序、尺寸、搜索入口和链接未变化。
- 证据：`ci/p54/p2-p3-bookmarks-layout.json`、`browser/desktop-guest-home.png`。

## P4 网络

- 管理员网络目标读取 → 200，3 个目标。
- 保存当前目标 → 200，重新读取地址不变。
- 公开网络目标 → 200；公开网络状态 → 200（隔离 CI 中结果为 `unknown`，因无可达探测目标，不改变 P4 既有语义）。
- 书签 LAN/WAN 地址同时保留，P4.4 地址选择数据源未改变。
- 证据：`ci/p54/p4-network.json`、`ci/p54/p2-p3-bookmarks-layout.json`、`local/frontend-test.log`。
