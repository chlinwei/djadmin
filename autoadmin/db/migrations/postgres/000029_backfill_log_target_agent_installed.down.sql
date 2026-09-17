-- 不可逆：回填丢失了变更前的原始 agent_installed（Django 时代可能已正确）。
-- 反向置回会重新制造"已安装被判成未安装"的故障。SELECT 占位让 golang-migrate 有合法语句。
SELECT 1;
