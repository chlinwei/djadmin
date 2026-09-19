-- 采集过滤的"显式关闭"（服务级 include 覆盖写 0）必须能落库：摘掉 collection_filter_rule_id 上的外键。
--
-- 背景（2026-09-19 现场）：服务级覆盖是**三态** —— NULL 继承模板 / 0 显式关闭该方向 / >0 指定规则
-- （见 db/schema 的列注释与 logcollect/log_collection_filter.go 的 resolveLogFilter）。但 Django 时代的
-- 真库上这一列挂着指向 monitor_log_collection_filter_rule.id 的外键，于是 0 被当成"指向 id=0 的规则"：
-- 「日志中心 → 日志配置 → 采集过滤（保留）」选"不过滤"保存时报外键失败，被 translate() 翻成
-- **「关联资产不存在」**——一句与过滤毫无关系、也指不出该改哪里的报错。000039 的升级说明让用户
-- "不想过滤就把该行改成'不过滤'"，那条路正是被这个外键堵死的。
--
-- 为什么删外键而不是改语义：
--   1) 三态是既定设计：NULL 表示"继承模板"，拿它顶替 0 会让"模板配了白名单、这条服务不想过滤"
--      彻底表达不出来（模板级默认值的存在就是为了让服务继承，不能没有关闭出口）；
--   2) 同表的 exclude 列（000039 新增）**没有**外键，两列语义完全相同却只有一列被约束，本身不自洽；
--   3) `db/schema` 的折叠快照里本来就没有这个外键（快照缺真库外键是老问题，见 SQL_DESIGN §4.7 与
--      计划文档的陷阱 24），同表同型的 processing_rule_id 外键已在 000035 用同一手法删过一次。
-- 引用完整性改由渲染侧降级承担：规则被删 / 停用 / 方向不符 → 该方向不过滤并随下发告警
-- （resolveLogFilter），而不是让 Filebeat 起不来或停采这条日志——"宁可多采不可不采"。
--
-- 真库上有这个外键、按折叠快照建的库上没有 —— 两类库都要能跑，所以**按列现查现删**：
-- 该列上有任何外键就删掉，没有就什么也不做（MySQL 侧同样处理）。PG 侧差异：Django 的截断命名规则
-- 与 MySQL 不同，且 PG 不上 `sqlite_rename_*` 之类的临时名，所以不拼名字、直接按列查 pg_constraint。
DO $$
DECLARE fk record;
BEGIN
  FOR fk IN
    SELECT con.conname
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    JOIN pg_attribute att ON att.attrelid = con.conrelid AND att.attnum = ANY (con.conkey)
    WHERE rel.relname = 'assets_application_service_log_setting'
      AND att.attname = 'collection_filter_rule_id'
      AND con.contype = 'f'
  LOOP
    EXECUTE format('ALTER TABLE assets_application_service_log_setting DROP CONSTRAINT %I', fk.conname);
  END LOOP;
END $$;
