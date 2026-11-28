-- 0007_network：P4.1 网络探测目标配置。
-- 三个固定目标由服务端维护，前端只能编辑地址与启用状态；
-- 地址实际值由管理员配置，初始只使用不指向真实网络的占位主机名。

CREATE TABLE IF NOT EXISTS network_targets (
    target_key  TEXT NOT NULL PRIMARY KEY CHECK (target_key IN ('router', 'bypass_router', 'nas')),
    address     TEXT NOT NULL DEFAULT '',
    enabled     INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    updated_at  TEXT NOT NULL
);

INSERT OR IGNORE INTO network_targets (target_key, address, enabled, updated_at) VALUES
    ('router', 'router.local', 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    ('bypass_router', '', 0, strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    ('nas', '', 0, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'));
