-- 保留已有业务引用，不删除服务或部署；非 HA 内置模型不再具有特殊类型。
UPDATE assets_cluster_profile p
SET p.profile_type = 'custom', p.cluster_type = 'custom'
WHERE p.profile_type = 'builtin'
  AND p.code IN ('mysql', 'redis', 'elasticsearch', 'nacos')
  AND EXISTS (
    SELECT 1 FROM assets_application_service s WHERE s.cluster_profile_id = p.id
  );

DELETE FROM assets_cluster_profile
WHERE profile_type = 'builtin'
  AND code IN ('mysql', 'redis', 'elasticsearch', 'nacos');
