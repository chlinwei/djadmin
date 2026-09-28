-- 服务级后处理器（ES 包装 pipeline，2026-09-28）：服务 × 日志定义粒度的"解析之后再执行"的
-- 处理器数组（drop_fields / drop / rename / convert 等 ES ingest 处理器）。
--
-- 背景：解析规则的 pipeline 是按规则共享的（Filebeat 片段里 `pipeline:` 指向同一条），
-- 在规则里写 drop_fields 必然影响所有引用该规则的服务。解决方式是拆两层：
--   - 基础 pipeline（解析）仍按规则共享，内容不变；
--   - 配了 extra_processors 的服务，平台自动生成一个**包装 pipeline**（id = 基础名@s<服务id>），
--     内容 = 先用 `pipeline` 处理器调基础 pipeline，再顺序执行这里的处理器；
--     该服务的 Filebeat 片段 `pipeline:` 改指包装，其他服务的片段不变。
--
-- 存法：settings 行本来就有（服务级覆盖的载体），加一个 JSON 列存处理器数组
-- `[{...ES ingest processor...}, ...]`，`[]` = 不启用（包装不存在，片段指基础 pipeline）。
-- 与 000049 的 drop_conditions（已回退的历史遗留列）无关，两列互不读写对方。
-- 一次性影响：存量行该列为默认 `[]`，行为不变。
ALTER TABLE assets_application_service_log_setting
  ADD COLUMN extra_processors jsonb NOT NULL DEFAULT '[]'::jsonb;
-- 反向：删掉服务级后处理器列。
-- 注意 CLI 只有 `migrate`（up）与 `migrate force`，没有 down 子命令；要在真库上清掉这一列，
-- 需手工执行本语句并把 schema_migrations 的版本号退回 49（或直接用 `migrate force 49`
-- 清记录后手工 DROP COLUMN）。
ALTER TABLE assets_application_service_log_setting
  DROP COLUMN extra_processors;
