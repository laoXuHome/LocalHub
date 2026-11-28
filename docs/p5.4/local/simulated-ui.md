# P5.4 本地 UI 证据口径与四态模拟说明

本机没有 Docker CLI / Docker Socket 权限，不能启动真实容器。`docs/p5.4/browser/` 中的截图来自本地 `127.0.0.1:11202`：

- LocalHub 使用 `LOCALHUB_DOCKER_API_URL=http://127.0.0.1:12392` 指向一个只实现 `/version`、`/containers/json?all=1`、`/containers/{id}/json` 的测试 HTTP 服务。
- 测试服务返回 4 个容器：`alpha-app`（running）、`beta-job`（exited/stopped）、`gamma-unhealthy`（running + unhealthy → error）、`delta-mystery`（未知 State → unknown）。
- 这些容器 ID 是固定测试 ID，不对应真实 Docker 容器。
- 截图只验证前端渲染、搜索、关联展示、主题和响应式布局，**不是真实 Docker 环境验证**。

真实 Docker 验证以 `docs/p5.4/ci/p54/` 与 `docs/p5.4/ci/p53/` 为准：

- CI 真实创建 `p52-running`（running）、`p52-stopped`（stopped）。
- CI 真实创建 `p54-restarting`（restart policy 导致 restarting → error）。
- `unknown` 未在真实 Docker 中构造，只由 Go `containerStatus` 单元测试和前端 `containerStatusTone/Label` 单元测试覆盖。
