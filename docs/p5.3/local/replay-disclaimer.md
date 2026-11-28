# 本地 UI 证据口径说明

本机执行环境没有 Docker CLI/权限，不能创建真实容器，也没有运行真实 Docker API。

`docs/p5.3/browser/` 下的截图来自本地 `127.0.0.1:11201` LocalHub：

- Docker 容器数据由 `/tmp/p52_replay.py` 在 `127.0.0.1:12390` 回放 P5.2 CI 保存的真实 `/containers/json` 响应。
- 截图验证的是前端渲染、交互和样式，不是本机实时 Docker 发现。
- 真实 Docker 关联建立/修改/解除、状态变化、容器删除、同名不同 ID 和故障恢复，以 `docs/p5.3/ci/` 的 GitHub Actions 证据为准。

本地回放仅用于 UI 截图，没有被描述为真实 Docker 验证。
