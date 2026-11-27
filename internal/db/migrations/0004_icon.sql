-- 0004_icon：P2.2 本地图标文件元数据。
-- 只记录图标文件在 /data/icons 下的文件名与类型/大小，不改变既有书签字段语义。

ALTER TABLE bookmarks ADD COLUMN icon_file TEXT NOT NULL DEFAULT '';
ALTER TABLE bookmarks ADD COLUMN icon_mime TEXT NOT NULL DEFAULT '';
ALTER TABLE bookmarks ADD COLUMN icon_size INTEGER NOT NULL DEFAULT 0;
ALTER TABLE bookmarks ADD COLUMN icon_updated_at TEXT NOT NULL DEFAULT '';
