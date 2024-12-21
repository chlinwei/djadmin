insert into `sys_menu`(`id`,`name`,`icon`,`parent_id`,`order_num`,`path`,`component`,`menu_type`,`perms`,`create_time`,`update_time`,`remark`) values
(1,'系统管理','system',0,1,'/sys','','M','','2024-07-04','2024-07-04','系统管理目录'),
(2,'业务管理','monitor',0,2,'/bsns','','M','','2024-07-04','2024-07-04','业务管理目录'),
(3,'用户管理','user',1,1,'/sys/user','sys/user/index','C','system:user:list','2024-07-04','2024-07-04','用户管理菜单'),
(4,'角色管理','peoples',1,2,'/sys/role','sys/role/index','C','system:role:list','2024-07-04','2024-07-04','角色管理菜单'),
(5,'菜单管理','treetable',1,3,'/sys/menu','sys/menu/index','C','system:menu:list','2024-07-04','2024-07-04','菜单管理菜单'),
(6,'部门管理','tree',2,1,'/bsns/department','bsns/Department','C','','2024-07-04','2024-07-04','部门管理菜单'),
(7,'岗位管理','post',2,2,'/bsns/post','bsns/Post','C','','2024-07-04','2024-07-04','岗位管理菜单');
