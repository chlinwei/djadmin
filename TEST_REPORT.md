# 测试报告

- **生成时间**: 2026-07-10 12:17:40
- **整体结果**: ✅ 全部通过
- **总耗时**: 29.580 秒

## 汇总

| 指标 | 数量 |
| --- | --- |
| 总用例数 | 171 |
| ✅ 通过 | 171 |
| ❌ 失败 | 0 |
| 🔥 错误 | 0 |
| ⏭️ 跳过 | 0 |
| 通过率 | 100.0% |

## 模块汇总

| 模块 | 用例数 | 耗时(秒) |
| --- | --- | --- |
| assets | 54 | 0.705 |
| audit | 14 | 0.323 |
| automation | 56 | 1.218 |
| menu | 3 | 0.019 |
| role | 8 | 0.052 |
| scheduler | 12 | 0.753 |
| user | 24 | 23.709 |

## 用例明细

| 模块 | 测试类 | 用例 | 说明 | 耗时(秒) | 结果 |
| --- | --- | --- | --- | --- | --- |
| assets | ApplicationTest | test_batch_delete_applications | 批量删除应用 | 0.007 | ✅ 通过 |
| assets | ApplicationTest | test_create_application | 新增应用 | 0.006 | ✅ 通过 |
| assets | ApplicationTest | test_list_applications | 应用列表返回分页格式 | 0.006 | ✅ 通过 |
| assets | CredentialTest | test_batch_delete_credentials | 批量删除凭证 | 0.009 | ✅ 通过 |
| assets | CredentialTest | test_create_credential | 新增凭证 | 0.006 | ✅ 通过 |
| assets | CredentialTest | test_get_credential_detail | 获取凭证详情格式正确 | 0.005 | ✅ 通过 |
| assets | CredentialTest | test_list_credentials_paginated | 凭证列表返回分页格式 | 0.006 | ✅ 通过 |
| assets | CredentialTest | test_list_without_token | 无 token 返回 301 | 0.002 | ✅ 通过 |
| assets | CredentialTest | test_search_credentials | 搜索凭证 | 0.006 | ✅ 通过 |
| assets | CredentialTest | test_update_credential | 编辑凭证 | 0.009 | ✅ 通过 |
| assets | CredentialTest | test_update_credential_connection_config_should_force_close_related_webssh_sessions | 修改凭证连接配置后，应断开使用该默认凭证的主机 WebSSH 会话。 | 0.022 | ✅ 通过 |
| assets | HostCollectTest | test_collect_auth_failure_reaches_lock_threshold | 连续认证失败达到阈值后，应触发自动采集保护期。 | 0.022 | ✅ 通过 |
| assets | HostCollectTest | test_collect_host_with_invalid_connection | 采集主机时凭证错误应该返回 failed 状态 | 0.020 | ✅ 通过 |
| assets | HostCollectTest | test_collect_host_without_credential | 采集主机时没有配置凭证应该返回 failed 状态 | 0.012 | ✅ 通过 |
| assets | HostCollectTest | test_collect_persists_failed_status_on_host | 采集失败（无凭证）应把 collect_status 持久化为 failed，并写入原因和时间 | 0.006 | ✅ 通过 |
| assets | HostCollectTest | test_scheduled_batch_collect_updates_status | 定时任务批量采集 collect_all_hosts_info 应更新每台主机的 collect_status | 0.007 | ✅ 通过 |
| assets | HostCollectTest | test_scheduled_collect_skips_locked_host | 定时任务应跳过处于认证保护期的主机，避免持续触发账号锁定。 | 0.005 | ✅ 通过 |
| assets | HostGroupTest | test_create_host_group | 新增主机分组 | 0.009 | ✅ 通过 |
| assets | HostGroupTest | test_create_nested_group | 创建嵌套子分组 | 0.013 | ✅ 通过 |
| assets | HostGroupTest | test_delete_host_group | 删除主机分组 | 0.012 | ✅ 通过 |
| assets | HostGroupTest | test_get_host_group_detail | 获取主机分组详情 | 0.008 | ✅ 通过 |
| assets | HostGroupTest | test_get_host_group_tree | 获取分组树形结构 | 0.012 | ✅ 通过 |
| assets | HostGroupTest | test_list_host_groups | 主机分组列表 | 0.008 | ✅ 通过 |
| assets | HostGroupTest | test_update_host_group | 编辑主机分组 | 0.012 | ✅ 通过 |
| assets | HostTest | test_cancel_webssh_chunk_upload | 取消分片上传应删除远端临时分片文件。 | 0.017 | ✅ 通过 |
| assets | HostTest | test_create_host | 新增主机 | 0.016 | ✅ 通过 |
| assets | HostTest | test_create_webssh_directory | 可创建远端目录。 | 0.013 | ✅ 通过 |
| assets | HostTest | test_create_webssh_empty_file | 可创建远端空文件。 | 0.014 | ✅ 通过 |
| assets | HostTest | test_delete_host_should_force_close_active_webssh_sessions | 删除主机时应主动关闭该主机在线 WebSSH 会话。 | 0.019 | ✅ 通过 |
| assets | HostTest | test_download_webssh_file_streaming | 下载主机文件应使用流式响应，避免大文件内存占用。 | 0.013 | ✅ 通过 |
| assets | HostTest | test_download_webssh_file_with_range | 下载主机文件支持 Range 断点续传。 | 0.015 | ✅ 通过 |
| assets | HostTest | test_get_host_detail | 获取主机详情 | 0.011 | ✅ 通过 |
| assets | HostTest | test_get_host_detail_ignores_sr0_in_disks | 主机详情应隐藏 /dev/sr0 磁盘，且使用率仅按有效磁盘计算。 | 0.013 | ✅ 通过 |
| assets | HostTest | test_get_webssh_active_count | 可查看某台主机当前在线 WebSSH 连接数 | 0.010 | ✅ 通过 |
| assets | HostTest | test_get_webssh_active_sessions | 可查看某台主机在线会话的用户名和开始时间 | 0.010 | ✅ 通过 |
| assets | HostTest | test_get_webssh_chunk_upload_status | 查询分片上传状态应返回当前已上传字节和下一分片下标。 | 0.012 | ✅ 通过 |
| assets | HostTest | test_get_webssh_chunk_upload_status_not_exists | 上传状态查询在临时分片不存在时应返回 exists=False。 | 0.012 | ✅ 通过 |
| assets | HostTest | test_issue_webssh_file_download_ticket | 可签发独立传输服务下载票据。 | 0.011 | ✅ 通过 |
| assets | HostTest | test_issue_webssh_file_upload_ticket | 可签发独立传输服务上传票据。 | 0.012 | ✅ 通过 |
| assets | HostTest | test_list_hosts | 主机列表返回分页格式 | 0.014 | ✅ 通过 |
| assets | HostTest | test_list_hosts_with_host_id_filter | 按主机 ID 过滤应精确返回单台主机 | 0.073 | ✅ 通过 |
| assets | HostTest | test_list_webssh_files | 可获取主机文件列表（目录优先排序）。 | 0.013 | ✅ 通过 |
| assets | HostTest | test_list_webssh_sessions | 可按主机查询 Web SSH 会话审计记录 | 0.015 | ✅ 通过 |
| assets | HostTest | test_rename_webssh_file | 可重命名主机文件。 | 0.014 | ✅ 通过 |
| assets | HostTest | test_update_host | 编辑主机后返回完整主机信息 | 0.024 | ✅ 通过 |
| assets | HostTest | test_update_host_default_credential_should_force_close_active_webssh_sessions | 切换主机默认凭证后，应主动断开该主机在线 WebSSH 会话。 | 0.033 | ✅ 通过 |
| assets | HostTest | test_update_host_ip_should_force_close_active_webssh_sessions | 修改主机 IP 后，应主动断开该主机在线 WebSSH 会话。 | 0.027 | ✅ 通过 |
| assets | HostTest | test_update_host_port_should_force_close_active_webssh_sessions | 修改主机 SSH 端口后，应主动断开该主机在线 WebSSH 会话。 | 0.028 | ✅ 通过 |
| assets | HostTest | test_upload_webssh_file_by_chunks | 分片上传完成后应合并为最终文件。 | 0.026 | ✅ 通过 |
| assets | HostWebSSHConsumerTest | test_collect_linux_info_excludes_optical_device_sr0 | 磁盘采集应忽略 /dev/sr0，避免无意义光驱统计进入结果。 | 0.001 | ✅ 通过 |
| assets | HostWebSSHConsumerTest | test_collect_persists_success_status_on_host | 采集成功应把 collect_status 持久化为 success 并清空失败原因 | 0.012 | ✅ 通过 |
| assets | HostWebSSHConsumerTest | test_extract_token_expire_at_from_payload | 验证：extract 令牌 expire at 来自 payload | 0.000 | ✅ 通过 |
| assets | HostWebSSHConsumerTest | test_extract_token_expire_at_invalid_value | 验证：extract 令牌 expire at 非法 value | 0.000 | ✅ 通过 |
| assets | HostWebSSHConsumerTest | test_receive_should_close_when_token_expired | 验证：receive should close 当 令牌 expired | 0.003 | ✅ 通过 |
| audit | AuditCleanupTaskTest | test_cleanup_login_audit_logs_falls_back_to_default_on_invalid_config | 验证：清理 登录 审计 日志 回退到 默认 在 非法 配置 | 0.007 | ✅ 通过 |
| audit | AuditCleanupTaskTest | test_cleanup_login_audit_logs_respects_retention_window | 验证：清理 登录 审计 日志 遵循 保留 窗口 | 0.007 | ✅ 通过 |
| audit | AuditCleanupTaskTest | test_cleanup_operation_audit_logs_respects_retention_window | 验证：清理 操作 审计 日志 遵循 保留 窗口 | 0.007 | ✅ 通过 |
| audit | OperationAuditLogTest | test_admin_role_has_operation_audit_menu_permissions | 验证：管理员 角色 有 操作 审计 菜单 权限 | 0.016 | ✅ 通过 |
| audit | OperationAuditLogTest | test_get_requests_are_not_logged | 验证：获取 requests 被 not logged | 0.020 | ✅ 通过 |
| audit | OperationAuditLogTest | test_list_operation_logs_returns_paginated | 验证：列表 操作 日志 返回 分页 | 0.022 | ✅ 通过 |
| audit | OperationAuditLogTest | test_operation_audit_menu_has_three_entries | 验证：操作 审计 菜单 有 三个 条目 | 0.014 | ✅ 通过 |
| audit | OperationAuditLogTest | test_operation_log_created_by_middleware | 验证：操作 日志 创建 按 中间件 | 0.019 | ✅ 通过 |
| audit | WebSSHAuditDownloadTest | test_download_filtered_webssh_session_logs | 验证：下载 已过滤 WebSSH 会话 日志 | 0.123 | ✅ 通过 |
| audit | WebSSHAuditDownloadTest | test_download_selected_webssh_session_logs_by_ids | 验证：下载 选中 WebSSH 会话 日志 按 ID 列表 | 0.016 | ✅ 通过 |
| audit | WebSSHAuditDownloadTest | test_download_webssh_session_log | 验证：下载 WebSSH 会话 日志 | 0.013 | ✅ 通过 |
| audit | WebSSHAuditDownloadTest | test_download_webssh_session_log_uses_sanitized_output | 验证：下载 WebSSH 会话 日志 使用 脱敏 输出 | 0.018 | ✅ 通过 |
| audit | WebSSHAuditDownloadTest | test_download_webssh_session_logs_as_zip_when_more_than_two | 验证：下载 WebSSH 会话 日志 为 压缩包 当 更多 than 两个 | 0.023 | ✅ 通过 |
| audit | WebSSHAuditDownloadTest | test_list_webssh_sessions_filter_by_output_keyword | 验证：列表 WebSSH 会话 过滤 按 输出 关键字 | 0.014 | ✅ 通过 |
| automation | AutomationJobFilterTest | test_job_list_supports_status_and_output_keyword_filter | 验证：任务列表 支持 状态 并且 输出 关键字 过滤 | 0.016 | ✅ 通过 |
| automation | AutomationJobLogConsumerTest | test_connect_rejects_when_job_not_found | 验证：连接 拒绝 当 任务 未找到 | 0.006 | ✅ 通过 |
| automation | AutomationJobLogConsumerTest | test_connect_rejects_when_permission_missing | 验证：连接 拒绝 当 权限 缺失 | 0.007 | ✅ 通过 |
| automation | AutomationJobLogConsumerTest | test_connect_rejects_when_token_missing | 验证：连接 拒绝 当 令牌 缺失 | 0.003 | ✅ 通过 |
| automation | AutomationJobLogConsumerTest | test_connect_sends_snapshot_status_and_completed_for_finished_job | 验证：连接 发送 快照 状态 并且 完成 用于 已完成 任务 | 0.008 | ✅ 通过 |
| automation | AutomationRunDispatchTest | test_create_task_without_inventory_succeeds | 创建 Task 时不指定 Inventory，应成功创建 | 0.028 | ✅ 通过 |
| automation | AutomationRunDispatchTest | test_patch_task_remove_inventory_then_run_fails | 编辑 Task 移除 Inventory 后，执行应失败 | 0.074 | ✅ 通过 |
| automation | AutomationRunDispatchTest | test_run_now_dispatches_to_celery | 验证：立即执行 派发 到 Celery | 0.026 | ✅ 通过 |
| automation | AutomationRunDispatchTest | test_run_now_fails_without_inventory | 验证：立即执行 fails 不带 inventory | 0.009 | ✅ 通过 |
| automation | AutomationRunDispatchTest | test_run_now_with_empty_scope_defaults_to_all_hosts | 验证：立即执行 带 empty 范围 defaults 到 全部 hosts | 0.020 | ✅ 通过 |
| automation | AutomationRunDispatchTest | test_run_template_dispatches_to_celery | 验证：执行 模板 派发 到 Celery | 0.016 | ✅ 通过 |
| automation | AutomationTaskScopeTest | test_task_list_contains_execution_scope_summary_and_tree | 验证：任务 列表 包含 执行 范围 摘要 并且 树 | 0.081 | ✅ 通过 |
| automation | AutomationTaskScopeTest | test_task_list_empty_scope_is_all_hosts | 验证：任务 列表 empty 范围 is 全部 hosts | 0.044 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_create_workflow_and_precheck_launch_without_global_inventory | 验证：创建 workflow 并且 precheck launch 不带 global inventory | 0.030 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_create_workflow_draft_without_nodes | 验证：创建 workflow draft 不带 nodes | 0.009 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_create_workflow_duplicate_name_rejected | 重复名称的 workflow 应被拒绝 | 0.014 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_create_workflow_with_nodes_and_edges | 创建包含节点和边的 workflow，字段正确保存 | 0.010 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_create_workflow_without_inventory_succeeds | 创建 Workflow 时不指定 default_inventory，应成功创建 | 0.011 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_delete_workflow_referenced_by_other_workflow | 测试删除被其他 Workflow 引用的 Workflow - 应拒绝删除 | 0.023 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_delete_workflow_with_runs | 测试删除包含运行记录的 Workflow - workflow 删除后，runs 保留但 workflow_id 设为 NULL | 0.055 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_delete_workflow_without_runs | 测试删除不含运行记录的 Workflow | 0.016 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_launch_blocked_when_task_node_is_disabled | launch 应在节点任务被禁用时返回业务错误，不创建 WorkflowRun。 | 0.027 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_launch_fails_without_workflow_inventory | Workflow 未设置 default_inventory 时，launch 应返回错误 | 0.015 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_launch_workflow_dispatches_task_jobs | 验证：launch workflow 派发 任务 jobs | 0.042 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_launch_workflow_uses_workflow_default_inventory_scope | 验证：launch workflow 使用 workflow 默认 inventory 范围 | 0.042 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_list_workflow_runs | 运行记录列表接口返回分页数据 | 0.041 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_list_workflow_runs_filter_by_status | 运行记录列表支持按 status 过滤 | 0.017 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_list_workflows | 列表接口返回分页结果，已创建的 workflow 出现在结果中 | 0.016 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_partial_update_workflow_enabled | PATCH 部分更新 enabled 字段 | 0.016 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_patch_workflow_remove_inventory_then_launch_fails | 编辑 Workflow 移除 default_inventory 后，launch 应失败 | 0.028 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_patch_workflow_remove_inventory_then_precheck_fails | 编辑 Workflow 移除 default_inventory 后，precheck-launch 应失败 | 0.026 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_patch_workflow_to_empty_nodes_succeeds | 编辑 Workflow 将 nodes 改为空列表，应成功 | 0.023 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_precheck_launch_blocked_when_task_inventory_is_disabled | precheck-launch 应在节点任务绑定的 Inventory 被禁用时返回 ok=False。 | 0.024 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_precheck_launch_blocked_when_task_node_is_disabled | precheck-launch 应在节点任务被禁用时返回 ok=False, status=node_task_invalid。 | 0.023 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_precheck_launch_fails_without_workflow_inventory | Workflow 未设置 default_inventory 时，precheck-launch 应返回错误 | 0.013 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_precheck_launch_not_blocked_by_disabled_task_in_other_workflow | 禁用某个任务只影响引用该任务的 Workflow，不影响未引用该任务的 Workflow。 | 0.021 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_precheck_launch_succeeds_after_re_enabling_task | 任务禁用后再重新启用，precheck-launch 应恢复为 ok=True。 | 0.034 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_precheck_launch_with_multiple_roots_and_global_inventory | 验证：precheck launch 带 multiple roots 并且 global inventory | 0.022 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_retrieve_workflow | 详情接口返回单条 workflow 数据 | 0.016 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_retrieve_workflow_run | 运行记录详情接口返回单条数据 | 0.017 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_run_record_name_preserved_after_workflow_deleted | 测试删除 Workflow 后，运行记录的名称仍然可以从快照读取 | 0.021 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_run_records_searchable_by_name_after_workflow_deleted | 测试删除 Workflow 后，仍可通过名称搜索到运行记录 | 0.024 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_update_workflow_name_and_description | PUT 全量更新名称和描述 | 0.018 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_update_workflow_nodes | PUT 更新节点列表，node_count 同步变更 | 0.019 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_workflow_can_be_edited_with_circular_reference | 验证：test that workflows can be edited 到 contain circular references (validation deferred 到 runtime). | 0.031 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_workflow_cycle_detected_at_runtime | 验证：test that workflow cycles 被 detected 当 executing workflow nodes. | 0.055 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_workflow_run_name_from_snapshot | 运行记录的 workflow_name 始终取自 workflow_name_snapshot | 0.019 | ✅ 通过 |
| automation | AutomationWorkflowTest | test_workflow_with_valid_nested_workflow_node | 验证：test that valid workflow nesting (without cycles) works 在 edit 并且 at runtime. | 0.019 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_create_playbook_rejects_invalid_yaml_structure | 验证：创建 Playbook 拒绝 非法 YAML structure | 0.006 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_download_playbook_returns_attachment | 验证：下载 Playbook 返回 附件 | 0.004 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_upload_playbook_rejects_empty_file | 验证：上传 Playbook 拒绝 empty 文件 | 0.005 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_upload_playbook_rejects_invalid_extension | 验证：上传 Playbook 拒绝 非法 扩展名 | 0.005 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_upload_playbook_rejects_invalid_yaml_syntax | 验证：上传 Playbook 拒绝 非法 YAML syntax | 0.008 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_upload_playbook_yaml_updates_content | 验证：上传 Playbook YAML updates 内容 | 0.008 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_validate_playbook_content_rejects_invalid_yaml | 验证：校验 Playbook 内容 拒绝 非法 YAML | 0.005 | ✅ 通过 |
| automation | PlaybookTemplateFileTest | test_validate_playbook_content_success | 验证：校验 Playbook 内容 成功 | 0.004 | ✅ 通过 |
| menu | MenuManageTest | test_create_menu | 验证：创建 菜单 | 0.006 | ✅ 通过 |
| menu | MenuManageTest | test_patch_menu | 验证：更新 菜单 | 0.008 | ✅ 通过 |
| menu | MenuManageTest | test_retrieve_menu | 验证：查询 菜单 | 0.004 | ✅ 通过 |
| role | RoleManageTest | test_batch_delete_cascades_user_role | 删除角色时级联删除用户-角色关联 | 0.012 | ✅ 通过 |
| role | RoleManageTest | test_batch_delete_empty_ids | 批量删除不传 ids 应报错 | 0.003 | ✅ 通过 |
| role | RoleManageTest | test_batch_delete_roles | 批量删除角色 | 0.008 | ✅ 通过 |
| role | RoleManageTest | test_create_role | 新增角色 | 0.005 | ✅ 通过 |
| role | RoleManageTest | test_get_current_user_role_list | 获取当前用户角色列表 | 0.004 | ✅ 通过 |
| role | RoleManageTest | test_get_role_detail | 获取角色详情 | 0.004 | ✅ 通过 |
| role | RoleManageTest | test_list_roles | 角色列表返回分页格式 | 0.009 | ✅ 通过 |
| role | RoleManageTest | test_update_role | 编辑角色 | 0.007 | ✅ 通过 |
| scheduler | ScheduledTaskApiTest | test_list_supports_search | 验证：列表 支持 搜索 | 0.073 | ✅ 通过 |
| scheduler | ScheduledTaskApiTest | test_run_now_returns_error_when_worker_offline | 验证：立即执行 返回 错误 当 Worker 离线 | 0.066 | ✅ 通过 |
| scheduler | ScheduledTaskApiTest | test_run_now_submits_task_when_worker_online | 验证：立即执行 提交 任务 当 Worker 在线 | 0.066 | ✅ 通过 |
| scheduler | ScheduledTaskApiTest | test_update_accepts_description_field | 验证：更新 accepts description field | 0.080 | ✅ 通过 |
| scheduler | ScheduledTaskApiTest | test_update_ignores_menu_field | 验证：更新 ignores 菜单 field | 0.086 | ✅ 通过 |
| scheduler | ScheduledTaskApiTest | test_update_with_legacy_remark_field_does_not_change_description | 验证：更新 带 legacy remark field does not change description | 0.081 | ✅ 通过 |
| scheduler | SchedulerManagerTest | test_cleanup_task_logs_applies_retention_and_max_rows | 验证：清理 任务 日志 applies 保留 并且 max rows | 0.023 | ✅ 通过 |
| scheduler | SchedulerManagerTest | test_ensure_default_tasks_creates_split_tasks_and_removes_legacy | 验证：ensure 默认 tasks creates split tasks 并且 移除 legacy | 0.046 | ✅ 通过 |
| scheduler | SchedulerManagerTest | test_run_scheduled_task_writes_success_log | 验证：执行 scheduled 任务 writes 成功 日志 | 0.055 | ✅ 通过 |
| scheduler | WorkflowRunCleanupTaskTest | test_cleanup_removes_finished_runs_older_than_retention | 验证：清理 移除 已完成 runs older than 保留 | 0.055 | ✅ 通过 |
| scheduler | WorkflowRunCleanupTaskTest | test_run_now_api_submits_workflow_cleanup_task | run_now 接口能正常提交 cleanup_workflow_run_logs 任务。 | 0.068 | ✅ 通过 |
| scheduler | WorkflowRunCleanupTaskTest | test_run_scheduled_task_executes_workflow_cleanup | run_scheduled_task 能正常调度 cleanup_workflow_run_logs 且写成功日志。 | 0.055 | ✅ 通过 |
| user | LoginViewTest | test_login_disabled_user | 禁用用户不能登录 | 0.677 | ✅ 通过 |
| user | LoginViewTest | test_login_response_format | 验证响应格式有 code/msg/data 三个字段 | 1.328 | ✅ 通过 |
| user | LoginViewTest | test_login_success | 正常登录返回 token | 1.243 | ✅ 通过 |
| user | LoginViewTest | test_login_user_not_exist | 用户不存在返回 code=300 | 0.657 | ✅ 通过 |
| user | LoginViewTest | test_login_wrong_password | 密码错误返回 code=300 | 1.253 | ✅ 通过 |
| user | UserCenterTest | test_login_plaintext_password_auto_migrates_to_hash | 历史明文密码用户登录成功后自动迁移为哈希 | 1.974 | ✅ 通过 |
| user | UserCenterTest | test_update_password_success | 修改密码成功 | 2.481 | ✅ 通过 |
| user | UserCenterTest | test_update_password_wrong_old | 旧密码错误修改失败 | 1.220 | ✅ 通过 |
| user | UserCenterTest | test_update_user_info | 更新个人信息 | 0.611 | ✅ 通过 |
| user | UserManageTest | test_assign_roles | 分配用户角色 | 0.659 | ✅ 通过 |
| user | UserManageTest | test_batch_delete_empty_ids | 批量删除不传 ids 应报错 | 0.611 | ✅ 通过 |
| user | UserManageTest | test_batch_delete_users | 批量删除用户 | 0.618 | ✅ 通过 |
| user | UserManageTest | test_change_user_status | 修改用户状态为禁用 | 0.606 | ✅ 通过 |
| user | UserManageTest | test_check_username_exists | 检查用户名存在 | 0.602 | ✅ 通过 |
| user | UserManageTest | test_check_username_not_exists | 检查用户名不存在 | 0.593 | ✅ 通过 |
| user | UserManageTest | test_create_user | 新增用户 | 1.198 | ✅ 通过 |
| user | UserManageTest | test_create_user_duplicate_username | 重复用户名应失败 | 1.213 | ✅ 通过 |
| user | UserManageTest | test_get_current_user | 获取当前用户信息 | 0.674 | ✅ 通过 |
| user | UserManageTest | test_get_user_detail | 获取用户详情 | 0.619 | ✅ 通过 |
| user | UserManageTest | test_get_user_roles | 获取用户角色列表 | 0.619 | ✅ 通过 |
| user | UserManageTest | test_list_users_returns_paginated | 用户列表返回分页格式 | 0.610 | ✅ 通过 |
| user | UserManageTest | test_list_users_without_token_returns_301 | 无 token 访问返回 301 | 0.614 | ✅ 通过 |
| user | UserManageTest | test_reset_password | 重置用户密码为 123456 | 2.422 | ✅ 通过 |
| user | UserManageTest | test_update_user | 编辑用户 | 0.607 | ✅ 通过 |
