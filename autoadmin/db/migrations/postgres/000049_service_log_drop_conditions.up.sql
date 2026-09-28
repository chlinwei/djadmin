-- 「按字段丢弃」（Filebeat 层，2026-09-25）：服务 × 日志定义粒度的结构化丢弃条件。
--
-- **功能代码已删除（2026-09-25 晚）**：该功能连回它的迁移一起回退过一次，但开发库已经执行过
-- 这个 up（schema_migrations 记到 49、列已存在），迁移文件必须与记录共存，否则迁移器拒启。
-- 现状：`drop_conditions` 列留在表上，应用代码**不再读写它**（所有列清单都是显式的），
-- 纯粹是历史遗留；down 可用于清掉它（需配 `migrate force` 回退版本号，CLI 没有 down 子命令）。
-- 存法：settings 行本来就有（服务级覆盖的载体），加一个 JSON 列存结构化条件
-- `{"logic":"all"|"any","conditions":[{"field","op","value"}]}`，`{}` = 不丢弃。
-- 一次性影响：存量行该列为默认 `{}`，行为不变。
ALTER TABLE assets_application_service_log_setting
  ADD COLUMN drop_conditions jsonb NOT NULL DEFAULT '{}'::jsonb;
