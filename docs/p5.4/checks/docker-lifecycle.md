# Docker 正常、故障、恢复验证记录

## 正常

真实 Docker，CI `P5.4 AMD64 集成验收`：

- `/api/admin/docker/status` → `connected`，`read_only=true`。
- `/api/admin/docker/containers` → `status=connected`，容器字段完整。
- 真实计数：`total=6, running=4, stopped=1, error=1, unknown=0`。
- `p52-running` → `running`；`p52-stopped` → `stopped`；`p54-restarting` → `error`。
- `p52-running` 关联 2 个书签（`P5.3 运行关联`、`P5.3 共享关联`）。
- 证据：`ci/p54/p5-docker-four-states-real.json`、`ci/p53/links-setup.json`。

## 容器状态变化

- `docker stop p52-running`：关联状态变 `stopped`，关联书签仍在。
- `docker start p52-running`：恢复 `running`，关联书签仍在。
- 证据：`ci/p53/links-container-stopped.json`、`ci/p53/links-container-restarted.json`。

## 容器删除与同名不同 ID

- `docker rm -f p52-stopped`：`missing` 出现 `P5.3 停止关联`，状态仍为 `connected`，不误报连接失败。
- 新建同名 `p52-stopped`：新 ID 与旧 ID 不同；新容器 `bookmarks=[]`；旧关联仍指向旧 ID。
- 证据：`ci/p53/links-container-removed.json`、`ci/p53/links-same-name-new-id.json`、`ci/p53/same-name-old-new-ids.txt`。

## 故障

停止 `localhub-docker-socket-proxy` 后：

- Docker 状态：`connection_failed`。
- 容器列表：`containers=[]`，`counts.stopped=0`，未把故障伪装成“全部停止”。
- 关联视图：`containers=[]`，`missing=[]`，不判定容器不存在。
- DB 中 `P5.3 运行关联/停止关联/共享关联` 的 `docker_container` 均未改变。
- `/health`、`/api/bookmarks`、`/api/auth/status` 均 200。
- 证据：`ci/p54/p5.4-outage.json`、`ci/p53/links-outage.json`、`ci/p51-outage.log`。

## 恢复

重新启动 Proxy 后：

- Docker 状态恢复 `connected`。
- 容器列表恢复，`p52-running` 为 `running`。
- 关联视图恢复，`P5.3 停止关联` 仍在 `p52-stopped`。
- 证据：`ci/p53/links-recovered.json`、`ci/p54/p5.4-same-name-final.json`。
