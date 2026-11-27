-- 0001_init：P0 最小初始化迁移。
-- 只建立基础设施所需的元数据表，不创建账户、书签、Docker、备份等业务表。
CREATE TABLE IF NOT EXISTS app_meta (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    key        TEXT NOT NULL UNIQUE,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
