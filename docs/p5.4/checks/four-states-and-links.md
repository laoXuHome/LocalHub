# 容器四态与关联状态验证

## 真实 Docker（CI）

| 状态 | 真实容器 | 结果 | 颜色 |
|---|---|---|---|
| running | `p52-running` | `running`；关联 2 个书签 | `#16A34A` |
| stopped | `p52-stopped` | `stopped`；关联 1 个书签 | `#9CA3AF` |
| error | `p54-restarting` | `error`；无关联 | `#DC2626` |
| unknown | 未在真实 Docker 中构造 | 仅 Go/前端单元测试 | `#6B7280` |

- 真实四态计数：`total=6, running=4, stopped=1, error=1, unknown=0`。
- 证据：`ci/p54/p5-docker-four-states-real.json`、`ci/p53/links-setup.json`、`ci/p53/links-container-stopped.json`。

## unknown 单元测试

- Go：`internal/dockerclient/containers_test.go` 覆盖 `paused/restarting/dead → error`、`""/mystery → unknown`。
- 前端：`web/src/docker.test.ts` 覆盖 `unknown` 文案“未知”和色调 `unknown`。
- 本地日志：`local/go-test.log`、`local/frontend-test.log`。

## 关联状态

| 场景 | 结果 | 证据 |
|---|---|---|
| 未关联 | `docker_container=''`，链接视图 unlinked 计数 | `ci/p53/links-setup.json` |
| 已关联运行中 | running 容器下含 2 个书签 | `ci/p53/links-setup.json` |
| 已关联停止 | p52-stopped 下含 1 个书签，状态 stopped | `ci/p53/links-setup.json` |
| 已关联异常 | `p54-restarting` status=error | `ci/p54/p5-docker-four-states-real.json` |
| 已关联未知 | 本地模拟容器 `delta-mystery`；真实环境未构造 | `browser/desktop-light-editor-unknown-simulated.png` |
| 已关联不存在 | missing 含旧 ID；红色“容器已不存在” | `ci/p53/links-container-removed.json`、`browser/desktop-light-editor-missing-simulated.png` |
| Docker 故障 | missing=[]，关联保留 | `ci/p53/links-outage.json`、`ci/p54/p5.4-outage.json` |
