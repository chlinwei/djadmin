-- 采集过滤（include/exclude）接入渲染所需的结构改动（2026-09-19）。
--
-- 背景：采集过滤规则在 Fluent Bit 时代是真生效的（`backend/djadmin/monitor/fluent_bit.py`
-- 渲染 `[FILTER] grep`），换 Filebeat 时这一环漏了：规则能建、服务级能选，但渲染不读它。
-- 本次把结构补齐，让"模板给默认 → 服务可覆盖/关闭 → 日志中心随时改"这条链立起来。
--
-- 三处改动：
--  1) 规则加 `rule_type`：方向必须显式声明。靠"用在哪个槽位"推方向，会让白名单落进 exclude 槽时
--     **反转语义**（只采噪声、丢掉正常日志），是丢数据级别的误用。存量一律 include——迁移前建的
--     规则（现场只有 common-error：error/fatal/失败/错误… 的白名单）本来就是白名单语义。
--  2) 模板日志定义加两个引用：该方向不过滤就是 NULL，两条各自独立（一条日志可同时有白名单与黑名单）。
--  3) 服务级覆盖补一列 exclude；原有 `collection_filter_rule_id` 承担 include 方向。
--     **三态语义**：NULL = 继承模板；0 = 显式关闭该方向；>0 = 指定规则。
--
-- 升级影响（写给运维看）：库里已存在的服务级 `collection_filter_rule_id`（现场：服务 `tomcat` ×
-- 日志定义 15 → `common-error`）在本次升级后**会开始真正生效**，那条日志将只采集匹配
-- error/failed/失败/错误… 的记录。这正是它当年被选中的意图（Fluent Bit 时代生效过），所以迁移
-- 不做清空；不想过滤就在「日志中心 → 日志配置」把该行改成"不过滤"。

--
-- PG 侧差异：保留字/类型同名（varchar/text 一致）；`AFTER` 是 MySQL 专有，PG 无列序概念，去掉；
--   boolean 的 DEFAULT 写法一致。其余逐字对应。

ALTER TABLE monitor_log_collection_filter_rule
  ADD COLUMN rule_type varchar(16) NOT NULL DEFAULT 'include';

ALTER TABLE assets_application_log_definition
  ADD COLUMN filter_include_rule_id bigint DEFAULT NULL,
  ADD COLUMN filter_exclude_rule_id bigint DEFAULT NULL;

ALTER TABLE assets_application_service_log_setting
  ADD COLUMN collection_exclude_filter_rule_id bigint DEFAULT NULL;
