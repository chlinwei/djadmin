-- 解析规则只由部署模板的日志定义决定：服务级 processing_rule_id 覆盖作废并删列。
--
-- 语义变化（有意为之，**直接覆盖、不留观察期**）：升级后所有服务的有效解析规则 = 模板日志定义
-- 上挂的那条规则，服务上原来自选的规则不再生效。后果要清楚：
--   1. 相关主机的期望配置变化 → 进入"待下发"（页面「待下发（已变更）」），重新下发后主机上的
--      pipeline 才真正换掉；
--   2. 已入库的历史数据不重新解析，新旧解析产物会混在同一个 data stream 里；
--   3. 若某条模板日志定义**没有**挂规则，那它本来就不采集（渲染会跳过并告警），删掉覆盖后
--      服务侧也无法再"自救"——必须回模板给它挂规则。
-- 覆盖值删除后不可恢复（down 只能恢复列结构，值一律为 NULL = 继承模板）。
--
-- 删列前必须先摘掉这一列上的外键：Django 时代的库上它挂着 FK
-- （`assets_application_s_processing_rule_id_b56ded30_fk_monitor_l` → monitor_log_processing_rule.id，
-- 名字按 Django 的截断规则生成），而 `db/schema` 的折叠快照里没有这个外键（快照缺真库外键是
-- 老问题，见 SQL_DESIGN §4.7 陷阱 24）。两类库都要能跑，所以**按列现查现删**：
-- 该列上有任何外键就删掉，没有就什么也不做（MySQL 侧同样处理）。
DO $$
DECLARE fk record;
BEGIN
  FOR fk IN
    SELECT con.conname
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    JOIN pg_attribute att ON att.attrelid = con.conrelid AND att.attnum = ANY (con.conkey)
    WHERE rel.relname = 'assets_application_service_log_setting'
      AND att.attname = 'processing_rule_id'
      AND con.contype = 'f'
  LOOP
    EXECUTE format('ALTER TABLE assets_application_service_log_setting DROP CONSTRAINT %I', fk.conname);
  END LOOP;
END $$;

ALTER TABLE assets_application_service_log_setting DROP COLUMN processing_rule_id;
