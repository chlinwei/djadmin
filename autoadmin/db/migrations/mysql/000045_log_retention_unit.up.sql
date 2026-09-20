-- 保留期支持"小时"单位（见 docs/architecture/LOG_COLLECTION_ARCHITECTURE.md §4.5）。
--
-- 背景：档位原先只有 `retention_days`（天），而 ILM 的 `delete.min_age` 本来就接受
-- `s/m/h/d` —— 小时级保留（临时排障、大流量短期留存）只需要把单位带进 min_age，
-- 不用改数据流：流名与 ILM 策略名用的都是档位 **code**，不是天数，所以
-- 换单位/改保留期**不影响已有流**，只影响该档位新数据的到期时间。
--
-- 三步：
--   0) 先摘掉真库上 Django 4.1 给 PositiveIntegerField 自动生成的 CHECK
--      `monitor_log_retention_tier_chk_1 CHECK ((retention_days >= 0))`：
--      **MySQL 不允许重命名被 CHECK 引用的列**，不摘掉第 1 步必挂 ——
--      `Error 3959: Check constraint 'monitor_log_retention_tier_chk_1' uses column
--      'retention_days', hence column cannot be dropped or renamed`（2026-09-20 现场）。
--      与 000040 / 000044 同一手法：约束名是 Django 生成的（`<表>_chk_<n>`），**不写死**，
--      按 information_schema 现查现删，按折叠快照建的库没有这个约束时走 `SELECT 1` 空操作。
--      摘掉它不损失校验语义：这一列是 `int unsigned`，`>= 0` 恒真；折叠快照
--      （`db/schema/*/003_monitor_inspection.sql`）本就没有这个约束，两类库由此收敛。
--   1) 列改名 `retention_days` → `retention_value`：值 + 单位才是完整语义，
--      留着"days"这个名字在两列并存时会误导（README：重命名走显式迁移）。
--   2) 新增 `retention_unit`（`d` / `h`），存量行一律按天 —— 非破坏、不改任何现有档位的保留期。
--
-- 一次性影响：无。存量档位 `retention_unit='d'`、`retention_value` 原值不变，
-- 生成的 ILM min_age 与改动前逐字节相同（`30d`）。
--
-- 注意：本文件是多语句批次（golang-migrate 把整份文件作为**一个** Exec 下发，
-- 依赖 DSN 的 `multiStatements=true`，驱动已强制打开）。MySQL 的 DDL 不能回滚，
-- 若在 1) 之后失败，版本表会停在脏标记上：重跑前先 `SHOW CREATE TABLE` 确认列的实际状态
-- （见 SQL_DESIGN §4.6.2 ②）。
SET @retention_chk := (
  SELECT cc.CONSTRAINT_NAME
  FROM information_schema.CHECK_CONSTRAINTS cc
  JOIN information_schema.TABLE_CONSTRAINTS tc
    ON tc.CONSTRAINT_SCHEMA = cc.CONSTRAINT_SCHEMA
   AND tc.CONSTRAINT_NAME = cc.CONSTRAINT_NAME
  WHERE cc.CONSTRAINT_SCHEMA = DATABASE()
    AND tc.TABLE_NAME = 'monitor_log_retention_tier'
    AND tc.CONSTRAINT_TYPE = 'CHECK'
    AND cc.CHECK_CLAUSE LIKE '%retention_days%'
  LIMIT 1
);
SET @drop_retention_chk := IF(
  @retention_chk IS NULL,
  'SELECT 1',
  CONCAT('ALTER TABLE `monitor_log_retention_tier` DROP CHECK `', @retention_chk, '`')
);
PREPARE drop_retention_chk_stmt FROM @drop_retention_chk;
EXECUTE drop_retention_chk_stmt;
DEALLOCATE PREPARE drop_retention_chk_stmt;

ALTER TABLE `monitor_log_retention_tier`
  CHANGE COLUMN `retention_days` `retention_value` int unsigned NOT NULL;

ALTER TABLE `monitor_log_retention_tier`
  ADD COLUMN `retention_unit` varchar(4) NOT NULL DEFAULT 'd';
