-- 删除「存储水位」菜单：该页的能力已全部并入「日志中心」（迁移 000037 的 /monitor/logging/center），
-- 两页消费的是同一个接口（GET /monitor/elasticsearch-clusters/:id/log-storage-overview/），
-- 数字永远一致，留着只会让人在两个入口之间猜"该看哪个"。
--
-- 并入的能力清单（逐条对照旧页，无遗漏）：集群 + 数据时间、节点磁盘水位、统计（流数/文档数/
-- 总占用拆分活跃与历史/健康异常流）、流明细（占用/文档数/ILM/状态/后备索引展开）、
-- 未识别流的提示与标注、按层级的容量聚合（旧页是左侧层级树，本页是左侧服务树 + 聚合表）。
-- 旧页的层级树不丢：日志中心左侧服务树同样能选项目/业务系统/环境，水位 tab 会按选中层级过滤并聚合。
--
-- 顺带说明为什么删的是"菜单 + 页面"而不只是菜单：菜单没了页面就没有入口，留一个只有 URL 能到的
-- 文件页是死代码；旧地址 /monitor/logging/overview 在前端静态路由里保留 redirect 兜底
-- （收藏/书签不断链）。接口本身**不删**——日志中心在用。
--
-- 该菜单（迁移 000019）的 perms 为空，删它不影响任何权限点。
-- SQL 幂等（按 path 定位）；先清授权再删菜单，避免 sys_role_menu 留孤儿行。
--
-- PG 侧差异：DELETE ... WHERE id IN (SELECT ...) 的写法与 MySQL 一致（PG 无同表改写限制，
--   保留派生表只是为了让两方言逐行对齐）。

DELETE FROM sys_role_menu WHERE menu_id IN (
    SELECT id FROM (SELECT id FROM sys_menu WHERE path = '/monitor/logging/overview') t
);
DELETE FROM sys_menu WHERE path = '/monitor/logging/overview';
