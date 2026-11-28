# P5.3 本地 UI 与接口检查记录

环境：`http://127.0.0.1:11201`，数据 `/tmp/p53-ui-data`，管理员 `admin`。
Docker 数据：`127.0.0.1:12390` 回放 P5.2 CI 保存的真实容器列表，不是本机实时 Docker。

## 管理员界面

| 操作 | 结果 | 截图 |
|---|---|---|
| 打开“编辑首页”→点击书签 | 编辑器展示“Docker 容器关联”、当前容器、状态、搜索框和容器列表 | `browser/desktop-light-editor-running.png` |
| 搜索 `socket` | 列表仅保留 `localhub-docker-socket-proxy` | 已人工检查，未单独截图 |
| 选择 `p52-stopped` | 当前关联切换为灰色“已停止” | `browser/desktop-light-editor-stopped.png` |
| 点击“解除关联” | 当前关联清空，按钮消失，保存后接口不再返回该关联 | `browser/desktop-light-editor-unlinked.png` |
| 打开缺失关联书签 | 红色“容器已不存在（关联 ID bbbbbbbbbbbb）”，可解除 | `browser/desktop-light-editor-missing.png` |
| 深色主题 | 状态色、文字、滚动列表一致 | `browser/desktop-dark-editor-running.png` |
| AUTO 主题 | 与浅色系统主题一致 | `browser/desktop-auto-editor-running.png` |
| 移动端浅色/深色 | 单列布局，解除按钮在搜索框下方 | `browser/mobile-light-editor-running.png`、`browser/mobile-dark-editor-running.png` |
| Docker 页面 | `localhub` 显示“关联 1 个书签：NAS 管理台”，无关联容器显示“未关联书签” | `browser/desktop-light-docker-links.png` |
| 首页 | 无 Docker 文字/卡片，书签网格与打开链接未变化 | `browser/desktop-light-home.png` |

## 本地接口检查

- 管理员 `GET /api/admin/docker/bookmark-links`：200，`read_only=true`，返回容器与关联书签。
- 访客访问同一接口：401。
- 修改关联后，原容器关联数量减少，新容器关联数量增加；旧关联不在两个容器重复出现。
- 解除关联后，书签 `docker_container` 为空，容器 `bookmarks` 为空。
- 模拟缺失 ID：`missing` 返回该 ID；容器不存在与 Docker 故障分开处理。

本地检查只验证 UI 与接口行为，真实 Docker 生命周期以 `docs/p5.3/ci/` 为准。
