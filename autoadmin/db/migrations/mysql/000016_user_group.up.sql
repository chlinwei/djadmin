-- 用户组：独立于角色的通知/协作分组实体。角色管权限，用户组管"人群"，
-- 后续告警通知接收组等按用户组寻址，避免为收告警而动权限。
CREATE TABLE IF NOT EXISTS `sys_user_group` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `sys_user_group_member` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `group_id` bigint NOT NULL,
  `user_id` int NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `sys_user_group_member_group_user` (`group_id`, `user_id`),
  KEY `idx_user_group_member_user` (`user_id`),
  CONSTRAINT `fk_user_group_member_group` FOREIGN KEY (`group_id`) REFERENCES `sys_user_group` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_user_group_member_user` FOREIGN KEY (`user_id`) REFERENCES `sys_user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 菜单：挂在「系统管理」下（不存在则顶层），按钮权限沿用 system:usergroups:* 口径。
SET @identity_parent := (SELECT id FROM sys_menu WHERE name = '系统管理' AND menu_type = 'M' LIMIT 1);

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, location, is_expanded, create_time, update_time)
SELECT '用户组', 'fa-users-rectangle', COALESCE(@identity_parent, 0), 5, '/sys/user-groups', 'sys/usergroups/index', 'C', 'system:usergroups:view', 1, TRUE, NOW(6), NOW(6)
WHERE NOT EXISTS (SELECT 1 FROM sys_menu WHERE name = '用户组');

SET @usergroup_menu := (SELECT id FROM sys_menu WHERE name = '用户组' AND menu_type = 'C' LIMIT 1);

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, location, is_expanded, create_time, update_time)
SELECT '查询用户组', NULL, @usergroup_menu, 1, '', NULL, 'F', 'system:usergroups:view', 1, FALSE, NOW(6), NOW(6)
WHERE @usergroup_menu IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sys_menu WHERE parent_id = @usergroup_menu AND perms = 'system:usergroups:view');

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, location, is_expanded, create_time, update_time)
SELECT '新增用户组', NULL, @usergroup_menu, 2, '', NULL, 'F', 'system:usergroups:create', 1, FALSE, NOW(6), NOW(6)
WHERE @usergroup_menu IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sys_menu WHERE parent_id = @usergroup_menu AND perms = 'system:usergroups:create');

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, location, is_expanded, create_time, update_time)
SELECT '更新用户组', NULL, @usergroup_menu, 3, '', NULL, 'F', 'system:usergroups:update', 1, FALSE, NOW(6), NOW(6)
WHERE @usergroup_menu IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sys_menu WHERE parent_id = @usergroup_menu AND perms = 'system:usergroups:update');

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, location, is_expanded, create_time, update_time)
SELECT '删除用户组', NULL, @usergroup_menu, 4, '', NULL, 'F', 'system:usergroups:delete', 1, FALSE, NOW(6), NOW(6)
WHERE @usergroup_menu IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sys_menu WHERE parent_id = @usergroup_menu AND perms = 'system:usergroups:delete');

-- 角色授权：已拥有「用户管理」菜单的角色，同步获得用户组菜单与按钮（维持原可见人群）。
INSERT IGNORE INTO sys_role_menu (role_id, menu_id)
SELECT DISTINCT rm.role_id, ug.id
FROM sys_menu user_mgmt
JOIN sys_role_menu rm ON rm.menu_id = user_mgmt.id
JOIN sys_menu ug ON ug.name = '用户组' AND ug.menu_type = 'C'
WHERE user_mgmt.name = '用户管理' AND user_mgmt.menu_type = 'C';

INSERT IGNORE INTO sys_role_menu (role_id, menu_id)
SELECT rm.role_id, btn.id
FROM sys_menu ug
JOIN sys_role_menu rm ON rm.menu_id = ug.id
JOIN sys_menu btn ON btn.parent_id = ug.id AND btn.menu_type = 'F'
WHERE ug.name = '用户组' AND ug.menu_type = 'C';
