ALTER TABLE baseline_item ADD COLUMN chapter varchar(64) NOT NULL DEFAULT '通用';

-- PG 侧差异：MySQL 多表 UPDATE ... JOIN ... SET → PG 的 UPDATE ... FROM。
UPDATE baseline_item i
SET chapter = c.name
FROM baseline_category c
WHERE c.id = i.category_id;

-- PG 侧差异：MySQL 的 DROP FOREIGN KEY / DROP KEY 在 PG 分别是 DROP CONSTRAINT / DROP INDEX
-- （两者在 PG 里是各自独立的对象，删除约束不会连带删除索引）。
ALTER TABLE baseline_item DROP CONSTRAINT baseline_item_category_fk;
DROP INDEX baseline_item_category_fk;
ALTER TABLE baseline_item DROP COLUMN category_id;

DROP TABLE baseline_category;
