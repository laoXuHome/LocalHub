# P5.4 本地 UI 检查记录

环境：`http://127.0.0.1:11202`，数据 `/tmp/p54-ui-data`，管理员 `admin`。
Docker 数据：本地测试 HTTP 服务，**模拟**，详见 `simulated-ui.md`。

| 检查 | 结果 | 截图 |
|---|---|---|
| 访客首页 | 显示公开书签；无 Docker 状态文字、无容器卡片、无 Docker 入口 | `browser/desktop-guest-home.png` |
| Docker 页面浅色 | 4 容器，运行 1 / 停止 1 / 异常 1 / 未知 1；每行显示关联书签数量与名称；无启停按钮 | `browser/desktop-light-docker-four-states-simulated.png` |
| Docker 页面深色 | 同上，深色主题一致 | `browser/desktop-dark-docker-four-states-simulated.png` |
| Docker 页面 AUTO | 当前系统浅色，AUTO 解析为浅色，布局与状态一致 | `browser/desktop-auto-docker-four-states-simulated.png` |
| 移动端 Docker | 单列卡片，状态色、关联书签名称可见 | `browser/mobile-light-docker-four-states-simulated.png` |
| 编辑抽屉异常关联 | 当前容器红色“异常”，列表可选 alpha/beta/gamma/delta | `browser/desktop-light-editor-error-simulated.png` |
| 编辑抽屉未知关联 | 当前容器深灰“未知” | `browser/desktop-light-editor-unknown-simulated.png` |
| 编辑抽屉缺失关联 | 红色“容器已不存在（关联 ID eeeeeeeeeeee）”，可解除 | `browser/desktop-light-editor-missing-simulated.png` |
| 移动端编辑 | 单列，四个状态可见，解除按钮可用 | `browser/mobile-light-editor-error-simulated.png` |
| Docker 故障页面 | 停止测试 HTTP 服务后显示红色“连接失败”，0 容器，不误报停止 | `browser/desktop-light-docker-outage-simulated.png` |
| 首页回归 | 书签网格、排序、尺寸、搜索入口、打开链接未变化；P4 网络按钮仍为 LAN/AUTO | 同上访客/桌面截图 |

说明：故障截图为本地模拟断连；真实 Socket Proxy 停止后的行为由 CI `p5.4-outage.json` 和 `p51-outage.log` 证明。
