-- 日志格式"一次认证 + 指纹失效"：在 (逻辑服务 × 日志定义) 上记录认证结果。
--
-- 模型（2026-09-19 定）：新增服务/开启采集时**抽样校验一次**日志格式能否被该规则解析出必备字段
-- （log_level / log_message / error_fingerprint）；通过后不再持续检查，只在**格式指纹变化**时
-- 要求重新认证。指纹只由库里的配置算出（模板日志定义 + 规则更新时间 + 服务宏 + 应用版本），
-- 不依赖主机也不查 ES，所以"是否过期"是纯读库比对。
--
-- 四类会让认证失效的变化（前两类正是"模板变了"）：
--   换模板 / 模板增删日志定义 / 改名 / 改路径   → 指纹含 ld.id / ld.name / ld.path_pattern
--   改规则（pipeline_body、多行参数、首行正则） → 指纹含 rule.update_time
--   换挂另一条规则                             → 指纹含 ld.processing_rule_id
--   应用/中间件版本升级                        → 指纹含 s.application_version_id
-- 另加服务级 macro_values（改宏 = 换了一个文件在采，格式可能不同）。
-- 实例级 runtime_variables 不进指纹（逐实例而异，无法在服务级定义），改它需要人工重新认证。
ALTER TABLE assets_application_service_log_setting
  ADD COLUMN format_verified_at timestamp(6) DEFAULT NULL,
  ADD COLUMN format_verified_fingerprint varchar(64) NOT NULL DEFAULT '',
  -- 认证依据：instance（取实例最近 N 行真实日志）/ sample_log（规则自带样例）/ waiver（人工确认豁免）
  ADD COLUMN format_verified_source varchar(16) NOT NULL DEFAULT '',
  ADD COLUMN format_verified_by varchar(150) NOT NULL DEFAULT '';
