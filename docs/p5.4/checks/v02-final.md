# P5.1 V-02 只读边界最终验证

- 执行位置：LocalHub 容器网络命名空间内，通过同一个 `http://docker-socket-proxy:2375`。
- 结果：`pass=15 fail=0`。
- 写请求：create / start / stop / restart / delete / exec / network create / network connect / image delete（含 versioned 路径）全部拒绝。
- 读请求：`/version`、`/_ping`、`/containers/json` 按 allowlist 通过；日志接口被拒绝。
- Socket Proxy allowlist 未修改；LocalHub 未直接挂载 Docker Socket。
- 证据：`ci/p51-v02.log`、`ci/p51-v02/v02-matrix.txt`、`ci/p54/compose-config.txt`。
