-- 彻底移除 Fluent Bit：软件仓库不再支持 package_type=fluent_bit（采集器已改为 Filebeat）。
-- 删除历史遗留的 Fluent Bit 软件包记录；关联的 playbook 模板不在此处删除（可能被复用）。
-- 采集目标的安装/卸载历史保留原样（exporter_type_snapshot 是历史快照，不应改写）。
DELETE FROM `monitor_software_package` WHERE `package_type` = 'fluent_bit';
