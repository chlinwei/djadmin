ALTER TABLE `baseline_item` ADD COLUMN `chapter` varchar(64) NOT NULL DEFAULT '通用';
UPDATE `baseline_item` i
JOIN `baseline_category` c ON c.id = i.category_id
SET i.chapter = c.name;

ALTER TABLE `baseline_item` DROP FOREIGN KEY `baseline_item_category_fk`;
ALTER TABLE `baseline_item` DROP KEY `baseline_item_category_fk`;
ALTER TABLE `baseline_item` DROP COLUMN `category_id`;

DROP TABLE `baseline_category`;
