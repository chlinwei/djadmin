INSERT INTO sys_config (create_time, update_time, `key`, `value`, value_type, name, description, is_readonly, default_value)
VALUES (UTC_TIMESTAMP(6), UTC_TIMESTAMP(6), 'sys.automation.host_options.max_page_size', '1000', 'int',
        '自动化主机选项每页上限', '仅限制自动化主机选项接口单页返回数量，填正整数；不限制 Inventory 主机总数。', FALSE, '1000');
