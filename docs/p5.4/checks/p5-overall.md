# P5.1–P5.3 整体功能检查记录

| 阶段 | 检查项 | 结果 | 证据 |
|---|---|---|---|
| P5.1 | LocalHub 只通过 Socket Proxy 访问 Docker API | 通过；Compose 仅 Proxy 挂载 `/var/run/docker.sock:ro` | `docs/p5.1-docker-proxy.md`、`ci/p54/compose-config.txt` |
| P5.1 | LocalHub 不挂载/不直接访问 Socket | 通过；`docker-compose.yml` 中 LocalHub 无 Socket volume | `ci/p54/compose-config.txt` |
| P5.1 | Proxy 只读最小权限 | 通过；GET `version/_ping/containers`，写方法未配置 | `ci/p51-v02.log`、`ci/p51-v02/v02-matrix.txt` |
| P5.1 | Docker 写操作全部拒绝 | 15 PASS / 0 FAIL | `ci/p51-v02.log` |
| P5.1 | 连接状态、错误提示、恢复 | 通过；connected → connection_failed → connected | `ci/p54/p5.4-outage.json`、`ci/p53/links-recovered.json` |
| P5.1 | Docker 不可用不影响其他模块 | 通过；health/bookmarks/auth 均 200 | `ci/p54/p5.4-outage.json`、`ci/p51-outage.log` |
| P5.2 | 发现运行与停止容器 | 通过；真实 p52-running / p52-stopped | `ci/p53/containers-before.json` |
| P5.2 | 名称、镜像、ID、端口、标签 | 通过；ID 64 位、端口列表、labels 对象 | `ci/p54/p5-docker-four-states-real.json`、`ci/p53/containers-before.json` |
| P5.2 | running/stopped/error/unknown | running/stopped/error 真实通过；unknown 单元测试覆盖 | `ci/p54/p5-docker-four-states-real.json`、`local/go-test.log` |
| P5.2 | 自动/手动刷新 | 自动 30s 与手动刷新实现未改；CI 多次重新读取 | `ci/p54/p5.4-pre-summary.json` |
| P5.2 | 刷新失败保留上次数据 | 既有 P5.2 前端行为；P5.4 未改 | `docs/p5.2-report.md` |
| P5.2 | 故障不误报全部停止 | 通过；故障时 containers=[]、stopped=0 | `ci/p51-outage.log` |
| P5.3 | 建立/修改/解除关联 | 通过 | `ci/p53/links-setup.json`、`links-changed.json`、`links-unlinked.json` |
| P5.3 | 一书签一容器、一容器多书签 | 通过；p52-running 2 个，p52-stopped 1 个 | `ci/p53/links-setup.json` |
| P5.3 | 稳定容器 ID | 通过；DB 单字段 64 位 ID | `ci/p53/p53-state.json` |
| P5.3 | 不迁移同名新容器 | 通过；旧 ID 与新 ID 不同，新容器 bookmarks=[] | `ci/p53/links-same-name-new-id.json`、`same-name-old-new-ids.txt` |
| P5.3 | missing 识别 | 通过；真实删除后 missing | `ci/p53/links-container-removed.json` |
| P5.3 | 故障不误判 missing、不删关联 | 通过；故障时 missing=[]，关联仍在 | `ci/p53/links-outage.json`、`ci/p54/p5.4-outage.json` |
| P5.3 | Docker 页关联数量与名称 | 通过 | `ci/p53/links-setup.json`、`browser/desktop-light-docker-four-states-simulated.png` |
| P5.3 | 访客不能管理关联 | 通过；401 | `ci/p53/permission-check.txt`、`ci/p54/p1-auth.json` |
| P5.4 | 重启后关联仍存在 | 通过 | `ci/p53/links-after-restart.json` |
