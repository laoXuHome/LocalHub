-- 0005_folder：P2.3-R1 书签文件夹层级。
-- folder_path 存 JSON 数组（如 ["工作","项目A"]），空数组表示根层级；
-- 不引入分类/标签表，只保存来源文件中的层级路径。

ALTER TABLE bookmarks ADD COLUMN folder_path TEXT NOT NULL DEFAULT '[]';
