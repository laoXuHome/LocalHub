-- 0008_network_probe：P4.2 HTTP 探测的最新结果。
-- 结果与 P4.1 的 network_targets 配置分表保存，探测失败不会修改配置。
-- 仅保存真实探测过的启用目标；未探测、已禁用由查询层根据配置与结果是否存在推导。

CREATE TABLE IF NOT EXISTS network_probe_results (
    target_key  TEXT NOT NULL PRIMARY KEY CHECK (target_key IN ('router', 'bypass_router', 'nas')),
    address     TEXT NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('success', 'http_error', 'timeout', 'connection_failed', 'probe_error')),
    http_status INTEGER NOT NULL DEFAULT 0,
    latency_ms  INTEGER NOT NULL DEFAULT 0,
    checked_at  TEXT NOT NULL,
    reason      TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_network_probe_results_checked_at
    ON network_probe_results (checked_at);
