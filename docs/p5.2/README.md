# P5.2 验收证据目录

真实 Docker 环境验证与本地 UI 回放严格区分：

- `p52-containers-api-response.json`、`p52-docker-inspect.json`、`real-discovery-counts.log`、`ci-image-job.log`、`ci-run-*.txt`：GitHub Actions 宿主真实 Docker 环境结果。
- `browser/*.png`：使用上述真实 API 响应字段投影做本地回放，验证前端布局、主题和状态；不是本机实时 Docker 发现。
- `go-test.log`、`frontend-test.log`、`tsc.txt`、`frontend-build.log`：本地测试与构建。
- `status-mapping.txt`：四态映射单元测试记录。
- `auto-refresh-requests.log`：10 秒自动刷新请求记录。
- `docker-outage-recovery.log`、`p51-outage.log`：Docker 故障与恢复记录。
- `p51-v02/`、`p51-v02.log`：P5.1 V-02 只读边界回归。

完整说明见 `../p5.2-report.md`。
