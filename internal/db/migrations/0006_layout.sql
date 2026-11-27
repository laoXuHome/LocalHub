-- 0006_layout：P3.2 首页布局（设备维度）。
-- 每个书签在 desktop / mobile 各有一行，position 决定顺序，size 限定 large/medium/small；
-- 不修改既有 bookmarks.sort_order，P2 排序数据保持原样。

CREATE TABLE IF NOT EXISTS bookmark_layouts (
    device       TEXT NOT NULL CHECK (device IN ('desktop', 'mobile')),
    bookmark_id  INTEGER NOT NULL,
    position     INTEGER NOT NULL,
    size         TEXT NOT NULL CHECK (size IN ('large', 'medium', 'small')),
    updated_at   TEXT NOT NULL,
    PRIMARY KEY (device, bookmark_id)
);

CREATE INDEX IF NOT EXISTS idx_bookmark_layouts_order ON bookmark_layouts(device, position);
