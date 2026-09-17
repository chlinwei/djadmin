-- 归一化 Filebeat 软件包：早期前端表单把 filebeat 包存成了 rpm/rhel，导致选包查询
-- （package_type='filebeat' AND package_format='tar.gz'）永远匹配不到。Filebeat 只支持
-- 官方便携 tar.gz(any)，这里把历史行统一修正；arch 保持不变（决定二进制）。
UPDATE `monitor_software_package`
SET `package_format` = 'tar.gz', `platform_family` = 'any', `platform_major` = ''
WHERE `package_type` = 'filebeat'
  AND (`package_format` <> 'tar.gz' OR `platform_family` <> 'any' OR `platform_major` <> '');
