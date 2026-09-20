-- 保留期支持"小时"单位（见 docs/architecture/LOG_COLLECTION_ARCHITECTURE.md §4.5）。
-- 完整背景与取舍见同版本 MySQL 迁移头注释。
--
-- PG 侧差异：不需要 MySQL 那一步"先摘掉 Django 生成的 CHECK"。
--   ① PG 允许重命名被 CHECK 引用的列：约束在内部按列序号（attnum）绑定，`RENAME COLUMN`
--      之后自动作用在 `retention_value` 上（`pg_get_constraintdef` 显示新列名），不会像 MySQL
--      那样报 Error 3959 —— 所以这里只做改名即可。
--   ② 就算库上有 Django 4.1 给 PositiveIntegerField 生成的 `<表>_chk_1 CHECK (col >= 0)`，
--      PG 侧这一列是**有符号** `integer`（PG 没有 unsigned，见 SQL_DESIGN §4.7），
--      `>= 0` 在那边是**真校验**，留着比摘掉好。MySQL 侧相反（`int unsigned` 上恒真）：
--      摘掉是为了让真库与折叠快照收敛，不是为了改语义。
ALTER TABLE monitor_log_retention_tier RENAME COLUMN retention_days TO retention_value;

ALTER TABLE monitor_log_retention_tier
  ADD COLUMN retention_unit varchar(4) NOT NULL DEFAULT 'd';
