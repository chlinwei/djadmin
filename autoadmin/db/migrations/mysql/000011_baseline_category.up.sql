-- 基线条目分组升级：chapter 自由文本 → baseline_category 类目实体。
-- 类目成为一等对象（改名/排序/空类目稳定存在），策略（原条目）挂到类目下。

CREATE TABLE `baseline_category` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(64) NOT NULL,
  `sort` int unsigned NOT NULL DEFAULT 0,
  `baseline_id` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `baseline_category_baseline_fk` (`baseline_id`),
  CONSTRAINT `baseline_category_baseline_fk` FOREIGN KEY (`baseline_id`) REFERENCES `baseline` (`id`) ON DELETE CASCADE
);

-- 存量 chapter 按基线内首次出现顺序建类目（sort 从 0 递增）。
INSERT INTO `baseline_category` (create_time, update_time, name, baseline_id, sort)
SELECT NOW(6), NOW(6), chapter, baseline_id, rn - 1
FROM (
  SELECT chapter, baseline_id, ROW_NUMBER() OVER (PARTITION BY baseline_id ORDER BY MIN(id)) AS rn
  FROM baseline_item
  GROUP BY baseline_id, chapter
) t;

-- 回填 item.category_id（chapter 与类目名一一对应）。
ALTER TABLE `baseline_item` ADD COLUMN `category_id` bigint NULL;
UPDATE `baseline_item` i
JOIN `baseline_category` c ON c.baseline_id = i.baseline_id AND c.name = i.chapter
SET i.category_id = c.id;

ALTER TABLE `baseline_item`
  MODIFY COLUMN `category_id` bigint NOT NULL,
  ADD KEY `baseline_item_category_fk` (`category_id`),
  ADD CONSTRAINT `baseline_item_category_fk` FOREIGN KEY (`category_id`) REFERENCES `baseline_category` (`id`);

ALTER TABLE `baseline_item` DROP COLUMN `chapter`;
