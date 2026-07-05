# 测试报告

- **生成时间**: 2026-07-05 13:15:48
- **整体结果**: ✅ 全部通过
- **总耗时**: 55.959 秒

## 汇总

| 指标 | 数量 |
| --- | --- |
| 总用例数 | 122 |
| ✅ 通过 | 122 |
| ❌ 失败 | 0 |
| 🔥 错误 | 0 |
| ⏭️ 跳过 | 0 |
| 通过率 | 100.0% |

## 模块汇总

| 模块 | 用例数 | 耗时(秒) |
| --- | --- | --- |
| assets | 46 | 30.512 |
| audit | 14 | 0.265 |
| automation | 18 | 0.376 |
| menu | 3 | 0.031 |
| role | 8 | 0.049 |
| scheduler | 9 | 0.517 |
| user | 24 | 23.522 |

## 用例明细

| 模块 | 测试类 | 用例 | 说明 | 耗时(秒) | 结果 |
| --- | --- | --- | --- | --- | --- |
| assets | ApplicationTest | test_batch_delete_applications | 批量删除应用 | 0.007 | ✅ 通过 |
| assets | ApplicationTest | test_create_application | 新增应用 | 0.006 | ✅ 通过 |
| assets | ApplicationTest | test_list_applications | 应用列表返回分页格式 | 0.004 | ✅ 通过 |
| assets | CredentialTest | test_batch_delete_credentials | 批量删除凭证 | 0.008 | ✅ 通过 |
| assets | CredentialTest | test_create_credential | 新增凭证 | 0.005 | ✅ 通过 |
| assets | CredentialTest | test_get_credential_detail | 获取凭证详情格式正确 | 0.004 | ✅ 通过 |
| assets | CredentialTest | test_list_credentials_paginated | 凭证列表返回分页格式 | 0.005 | ✅ 通过 |
| assets | CredentialTest | test_list_without_token | 无 token 返回 301 | 0.001 | ✅ 通过 |
| assets | CredentialTest | test_search_credentials | 搜索凭证 | 0.005 | ✅ 通过 |
| assets | CredentialTest | test_update_credential | 编辑凭证 | 0.006 | ✅ 通过 |
| assets | HostCollectTest | test_collect_auth_failure_reaches_lock_threshold | 连续认证失败达到阈值后，应触发自动采集保护期。 | 0.026 | ✅ 通过 |
| assets | HostCollectTest | test_collect_host_with_invalid_connection | 采集主机时凭证错误应该返回 failed 状态 | 30.049 | ✅ 通过 |
| assets | HostCollectTest | test_collect_host_without_credential | 采集主机时没有配置凭证应该返回 failed 状态 | 0.012 | ✅ 通过 |
| assets | HostCollectTest | test_collect_linux_info_excludes_optical_device_sr0 | 磁盘采集应忽略 /dev/sr0，避免无意义光驱统计进入结果。 | 0.002 | ✅ 通过 |
| assets | HostCollectTest | test_collect_persists_failed_status_on_host | 采集失败（无凭证）应把 collect_status 持久化为 failed，并写入原因和时间 | 0.004 | ✅ 通过 |
| assets | HostCollectTest | test_collect_persists_success_status_on_host | 采集成功应把 collect_status 持久化为 success 并清空失败原因 | 0.011 | ✅ 通过 |
| assets | HostCollectTest | test_scheduled_batch_collect_updates_status | 定时任务批量采集 collect_all_hosts_info 应更新每台主机的 collect_status | 0.006 | ✅ 通过 |
| assets | HostCollectTest | test_scheduled_collect_skips_locked_host | 定时任务应跳过处于认证保护期的主机，避免持续触发账号锁定。 | 0.006 | ✅ 通过 |
| assets | HostGroupTest | test_create_host_group | 新增主机分组 | 0.009 | ✅ 通过 |
| assets | HostGroupTest | test_create_nested_group | 创建嵌套子分组 | 0.014 | ✅ 通过 |
| assets | HostGroupTest | test_delete_host_group | 删除主机分组 | 0.008 | ✅ 通过 |
| assets | HostGroupTest | test_get_host_group_detail | 获取主机分组详情 | 0.006 | ✅ 通过 |
| assets | HostGroupTest | test_get_host_group_tree | 获取分组树形结构 | 0.009 | ✅ 通过 |
| assets | HostGroupTest | test_list_host_groups | 主机分组列表 | 0.006 | ✅ 通过 |
| assets | HostGroupTest | test_update_host_group | 编辑主机分组 | 0.009 | ✅ 通过 |
| assets | HostTest | test_cancel_webssh_chunk_upload | 取消分片上传应删除远端临时分片文件。 | 0.020 | ✅ 通过 |
| assets | HostTest | test_create_host | 新增主机 | 0.013 | ✅ 通过 |
| assets | HostTest | test_create_webssh_directory | 可创建远端目录。 | 0.012 | ✅ 通过 |
| assets | HostTest | test_create_webssh_empty_file | 可创建远端空文件。 | 0.015 | ✅ 通过 |
| assets | HostTest | test_download_webssh_file_streaming | 下载主机文件应使用流式响应，避免大文件内存占用。 | 0.015 | ✅ 通过 |
| assets | HostTest | test_download_webssh_file_with_range | 下载主机文件支持 Range 断点续传。 | 0.015 | ✅ 通过 |
| assets | HostTest | test_get_host_detail | 获取主机详情 | 0.011 | ✅ 通过 |
| assets | HostTest | test_get_host_detail_ignores_sr0_in_disks | 主机详情应隐藏 /dev/sr0 磁盘，且使用率仅按有效磁盘计算。 | 0.012 | ✅ 通过 |
| assets | HostTest | test_get_webssh_active_count | 可查看某台主机当前在线 WebSSH 连接数 | 0.009 | ✅ 通过 |
| assets | HostTest | test_get_webssh_active_sessions | 可查看某台主机在线会话的用户名和开始时间 | 0.011 | ✅ 通过 |
| assets | HostTest | test_get_webssh_chunk_upload_status | 查询分片上传状态应返回当前已上传字节和下一分片下标。 | 0.013 | ✅ 通过 |
| assets | HostTest | test_get_webssh_chunk_upload_status_not_exists | 上传状态查询在临时分片不存在时应返回 exists=False。 | 0.010 | ✅ 通过 |
| assets | HostTest | test_issue_webssh_file_download_ticket | 可签发独立传输服务下载票据。 | 0.011 | ✅ 通过 |
| assets | HostTest | test_issue_webssh_file_upload_ticket | 可签发独立传输服务上传票据。 | 0.011 | ✅ 通过 |
| assets | HostTest | test_list_hosts | 主机列表返回分页格式 | 0.012 | ✅ 通过 |
| assets | HostTest | test_list_hosts_with_host_id_filter | 按主机 ID 过滤应精确返回单台主机 | 0.012 | ✅ 通过 |
| assets | HostTest | test_list_webssh_files | 可获取主机文件列表（目录优先排序）。 | 0.011 | ✅ 通过 |
| assets | HostTest | test_list_webssh_sessions | 可按主机查询 Web SSH 会话审计记录 | 0.014 | ✅ 通过 |
| assets | HostTest | test_rename_webssh_file | 可重命名主机文件。 | 0.014 | ✅ 通过 |
| assets | HostTest | test_update_host | 编辑主机后返回完整主机信息 | 0.016 | ✅ 通过 |
| assets | HostTest | test_upload_webssh_file_by_chunks | 分片上传完成后应合并为最终文件。 | 0.026 | ✅ 通过 |
| audit | AuditCleanupTaskTest | test_cleanup_login_audit_logs_falls_back_to_default_on_invalid_config | 验证：清理 登录 审计 日志 回退到 默认 在 非法 配置 | 0.009 | ✅ 通过 |
| audit | AuditCleanupTaskTest | test_cleanup_login_audit_logs_respects_retention_window | 验证：清理 登录 审计 日志 遵循 保留 窗口 | 0.008 | ✅ 通过 |
| audit | AuditCleanupTaskTest | test_cleanup_operation_audit_logs_respects_retention_window | 验证：清理 操作 审计 日志 遵循 保留 窗口 | 0.008 | ✅ 通过 |
| audit | OperationAuditLogTest | test_admin_role_has_operation_audit_menu_permissions | 验证：管理员 角色 有 操作 审计 菜单 权限 | 0.020 | ✅ 通过 |
| audit | OperationAuditLogTest | test_get_requests_are_not_logged | 验证：获取 requests 被 not logged | 0.027 | ✅ 通过 |
| audit | OperationAuditLogTest | test_list_operation_logs_returns_paginated | 验证：列表 操作 日志 返回 分页 | 0.024 | ✅ 通过 |
| audit | OperationAuditLogTest | test_operation_audit_menu_has_three_entries | 验证：操作 审计 菜单 有 三个 条目 | 0.021 | ✅ 通过 |
| audit | OperationAuditLogTest | test_operation_log_created_by_middleware | 验证：操作 日志 创建 按 中间件 | 0.022 | ✅ 通过 |
| audit | WebSSHAuditDownloadTest | test_download_filtered_webssh_session_logs | 验证：下载 已过滤 WebSSH 会话 日志 | 0.019 | ✅ 通过 |
| audit | WebSSHAuditDownloadTest | test_download_selected_webssh_session_logs_by_ids | 验证：下载 选中 WebSSH 会话 日志 按 ID 列表 | 0.019 | ✅ 通过 |
| audit | WebSSHAuditDownloadTest | test_download_webssh_session_log | 验证：下载 WebSSH 会话 日志 | 0.019 | ✅ 通过 |
| audit | WebSSHAuditDownloadTest | test_download_webssh_session_log_uses_sanitized_output | 验证：下载 WebSSH 会话 日志 使用 脱敏 输出 | 0.025 | ✅ 通过 |
| audit | WebSSHAuditDownloadTest | test_download_webssh_session_logs_as_zip_when_more_than_two | 验证：下载 WebSSH 会话 日志 为 压缩包 当 更多 than 两个 | 0.029 | ✅ 通过 |
| audit | WebSSHAuditDownloadTest | test_list_webssh_sessions_filter_by_output_keyword | 验证：列表 WebSSH 会话 过滤 按 输出 关键字 | 0.016 | ✅ 通过 |
| automation | AutomationJobFilterTest | test_job_list_supports_status_and_output_keyword_filter | 验证：任务列表 支持 状态 并且 输出 关键字 过滤 | 0.019 | ✅ 通过 |
| automation | AutomationJobLogConsumerTest | test_connect_rejects_when_job_not_found | 验证：连接 拒绝 当 任务 未找到 | 0.006 | ✅ 通过 |
| automation | AutomationJobLogConsumerTest | test_connect_rejects_when_permission_missing | 验证：连接 拒绝 当 权限 缺失 | 0.003 | ✅ 通过 |
| automation | AutomationJobLogConsumerTest | test_connect_rejects_when_token_missing | 验证：连接 拒绝 当 令牌 缺失 | 0.059 | ✅ 通过 |
| automation | AutomationJobLogConsumerTest | test_connect_sends_snapshot_status_and_completed_for_finished_job | 验证：连接 发送 快照 状态 并且 完成 用于 已完成 任务 | 0.018 | ✅ 通过 |
| automation | AutomationRunDispatchTest | test_run_now_dispatches_to_celery | 验证：立即执行 派发 到 Celery | 0.033 | ✅ 通过 |
| automation | AutomationRunDispatchTest | test_run_now_with_empty_scope_defaults_to_all_hosts | 验证：立即执行 带 empty 范围 defaults 到 全部 hosts | 0.019 | ✅ 通过 |
| automation | AutomationRunDispatchTest | test_run_template_dispatches_to_celery | 验证：执行 模板 派发 到 Celery | 0.017 | ✅ 通过 |
| automation | AutomationTaskScopeTest | test_task_list_contains_execution_scope_summary_and_tree | 验证：任务 列表 包含 执行 范围 摘要 并且 树 | 0.096 | ✅ 通过 |
| automation | AutomationTaskScopeTest | test_task_list_empty_scope_is_all_hosts | 验证：任务 列表 empty 范围 is 全部 hosts | 0.053 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_create_playbook_rejects_invalid_yaml_structure | 验证：创建 Playbook 拒绝 非法 YAML structure | 0.008 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_download_playbook_returns_attachment | 验证：下载 Playbook 返回 附件 | 0.005 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_upload_playbook_rejects_empty_file | 验证：上传 Playbook 拒绝 empty 文件 | 0.006 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_upload_playbook_rejects_invalid_extension | 验证：上传 Playbook 拒绝 非法 扩展名 | 0.007 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_upload_playbook_rejects_invalid_yaml_syntax | 验证：上传 Playbook 拒绝 非法 YAML syntax | 0.008 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_upload_playbook_yaml_updates_content | 验证：上传 Playbook YAML updates 内容 | 0.009 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_validate_playbook_content_rejects_invalid_yaml | 验证：校验 Playbook 内容 拒绝 非法 YAML | 0.005 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_validate_playbook_content_success | 验证：校验 Playbook 内容 成功 | 0.005 | ✅ 通过 |
| menu | MenuManageTest | test_create_menu | 验证：创建 菜单 | 0.017 | ✅ 通过 |
| menu | MenuManageTest | test_patch_menu | 验证：更新 菜单 | 0.009 | ✅ 通过 |
| menu | MenuManageTest | test_retrieve_menu | 验证：查询 菜单 | 0.005 | ✅ 通过 |
| role | RoleManageTest | test_batch_delete_cascades_user_role | 删除角色时级联删除用户-角色关联 | 0.011 | ✅ 通过 |
| role | RoleManageTest | test_batch_delete_empty_ids | 批量删除不传 ids 应报错 | 0.003 | ✅ 通过 |
| role | RoleManageTest | test_batch_delete_roles | 批量删除角色 | 0.009 | ✅ 通过 |
| role | RoleManageTest | test_create_role | 新增角色 | 0.005 | ✅ 通过 |
| role | RoleManageTest | test_get_current_user_role_list | 获取当前用户角色列表 | 0.005 | ✅ 通过 |
| role | RoleManageTest | test_get_role_detail | 获取角色详情 | 0.004 | ✅ 通过 |
| role | RoleManageTest | test_list_roles | 角色列表返回分页格式 | 0.005 | ✅ 通过 |
| role | RoleManageTest | test_update_role | 编辑角色 | 0.007 | ✅ 通过 |
| scheduler | ScheduledTaskApiTest | test_list_supports_search | 验证：列表 支持 搜索 | 0.070 | ✅ 通过 |
| scheduler | ScheduledTaskApiTest | test_run_now_returns_error_when_worker_offline | 验证：立即执行 返回 错误 当 Worker 离线 | 0.070 | ✅ 通过 |
| scheduler | ScheduledTaskApiTest | test_run_now_submits_task_when_worker_online | 验证：立即执行 提交 任务 当 Worker 在线 | 0.064 | ✅ 通过 |
| scheduler | ScheduledTaskApiTest | test_update_accepts_description_field | 验证：更新 accepts description field | 0.073 | ✅ 通过 |
| scheduler | ScheduledTaskApiTest | test_update_ignores_menu_field | 验证：更新 ignores 菜单 field | 0.072 | ✅ 通过 |
| scheduler | ScheduledTaskApiTest | test_update_with_legacy_remark_field_does_not_change_description | 验证：更新 带 legacy remark field does not change description | 0.060 | ✅ 通过 |
| scheduler | SchedulerManagerTest | test_cleanup_task_logs_applies_retention_and_max_rows | 验证：清理 任务 日志 applies 保留 并且 max rows | 0.020 | ✅ 通过 |
| scheduler | SchedulerManagerTest | test_ensure_default_tasks_creates_split_tasks_and_removes_legacy | 验证：ensure 默认 tasks creates split tasks 并且 移除 legacy | 0.040 | ✅ 通过 |
| scheduler | SchedulerManagerTest | test_run_scheduled_task_writes_success_log | 验证：执行 scheduled 任务 writes 成功 日志 | 0.047 | ✅ 通过 |
| user | LoginViewTest | test_login_disabled_user | 禁用用户不能登录 | 0.784 | ✅ 通过 |
| user | LoginViewTest | test_login_response_format | 验证响应格式有 code/msg/data 三个字段 | 1.286 | ✅ 通过 |
| user | LoginViewTest | test_login_success | 正常登录返回 token | 1.209 | ✅ 通过 |
| user | LoginViewTest | test_login_user_not_exist | 用户不存在返回 code=300 | 0.620 | ✅ 通过 |
| user | LoginViewTest | test_login_wrong_password | 密码错误返回 code=300 | 1.243 | ✅ 通过 |
| user | UserCenterTest | test_login_plaintext_password_auto_migrates_to_hash | 历史明文密码用户登录成功后自动迁移为哈希 | 1.857 | ✅ 通过 |
| user | UserCenterTest | test_update_password_success | 修改密码成功 | 2.440 | ✅ 通过 |
| user | UserCenterTest | test_update_password_wrong_old | 旧密码错误修改失败 | 1.230 | ✅ 通过 |
| user | UserCenterTest | test_update_user_info | 更新个人信息 | 0.618 | ✅ 通过 |
| user | UserManageTest | test_assign_roles | 分配用户角色 | 0.625 | ✅ 通过 |
| user | UserManageTest | test_batch_delete_empty_ids | 批量删除不传 ids 应报错 | 0.609 | ✅ 通过 |
| user | UserManageTest | test_batch_delete_users | 批量删除用户 | 0.621 | ✅ 通过 |
| user | UserManageTest | test_change_user_status | 修改用户状态为禁用 | 0.613 | ✅ 通过 |
| user | UserManageTest | test_check_username_exists | 检查用户名存在 | 0.612 | ✅ 通过 |
| user | UserManageTest | test_check_username_not_exists | 检查用户名不存在 | 0.604 | ✅ 通过 |
| user | UserManageTest | test_create_user | 新增用户 | 1.206 | ✅ 通过 |
| user | UserManageTest | test_create_user_duplicate_username | 重复用户名应失败 | 1.218 | ✅ 通过 |
| user | UserManageTest | test_get_current_user | 获取当前用户信息 | 0.605 | ✅ 通过 |
| user | UserManageTest | test_get_user_detail | 获取用户详情 | 0.613 | ✅ 通过 |
| user | UserManageTest | test_get_user_roles | 获取用户角色列表 | 0.622 | ✅ 通过 |
| user | UserManageTest | test_list_users_returns_paginated | 用户列表返回分页格式 | 0.631 | ✅ 通过 |
| user | UserManageTest | test_list_users_without_token_returns_301 | 无 token 访问返回 301 | 0.620 | ✅ 通过 |
| user | UserManageTest | test_reset_password | 重置用户密码为 123456 | 2.431 | ✅ 通过 |
| user | UserManageTest | test_update_user | 编辑用户 | 0.610 | ✅ 通过 |
