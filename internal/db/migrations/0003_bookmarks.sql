-- 0003_bookmarks：P2.1 书签数据模型。
-- 只建立书签主表，不含图标抓取、导入导出、Docker 发现等业务逻辑。

CREATE TABLE IF NOT EXISTS bookmarks (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    name              TEXT NOT NULL,
    lan_url           TEXT NOT NULL DEFAULT '',
    wan_url           TEXT NOT NULL DEFAULT '',
    description       TEXT NOT NULL DEFAULT '',
    icon_type         TEXT NOT NULL DEFAULT 'none',
    icon_value        TEXT NOT NULL DEFAULT '',
    enabled           INTEGER NOT NULL DEFAULT 1,
    visibility        TEXT NOT NULL DEFAULT 'public',
    open_mode         TEXT NOT NULL DEFAULT 'new_tab',
    docker_container  TEXT NOT NULL DEFAULT '',
    sort_order        INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_bookmarks_sort ON bookmarks(sort_order, id);
CREATE INDEX IF NOT EXISTS idx_bookmarks_visibility ON bookmarks(visibility, enabled);
