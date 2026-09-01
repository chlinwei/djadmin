# Backend module map

The current backend contains ten first-party Django apps, 70 concrete first-party models and two implicit many-to-many tables. This document defines initial Go ownership without renaming the existing schema.

| Go domain | Route prefix | Existing tables |
|---|---|---|
| user | `/sys` | `sys_user`, `sys_user_role`, `sys_agent_token`, implicit `sys_user_alert_media` |
| role | `/sys` | `sys_role` |
| menu | `/sys` | `sys_menu`, `sys_role_menu` |
| sysconfig | `/sys` | `sys_config` |
| scheduler | `/sys/scheduler` | `scheduler_scheduledtask`, `scheduler_scheduledtasklog` |
| audit | `/sys/audit` | `audit_login_log`, `audit_operation_log`; WebSSH audit reads assets tables |
| automation | `/sys/automation` | `automation_playbook_template`, `automation_task`, `automation_controller_ssh_key`, `automation_inventory`, `automation_execution_job`, `automation_execution_host_log`, `automation_workflow_template`, `automation_workflow_run` |
| inspection | `/sys/inspection` | `inspection_group`, `inspection_check`, `inspection_task`, `inspection_execution`, `inspection_target_execution`, `inspection_result` |
| monitor | `/monitor` | `monitor_target`, `monitor_target_install_history`, `monitor_alert_history`, `monitor_alert_media`, `monitor_alert_route`, `monitor_alert_notification_event`, `monitor_alert_notification_delivery`, `monitor_user_alert_media_binding`, implicit `monitor_alert_route_media`, `monitor_software_package`, `monitor_opensearch_cluster`, `monitor_log_retention_tier`, `monitor_log_processing_rule`, `monitor_log_collection_filter_rule`, `monitor_log_collection_target` |
| assets | `/assets`, `/api/agent` | `assets_credential`, `assets_hostgroup`, `assets_cloudaccount`, `assets_host`, `assets_hostcredential`, `assets_hosthardware`, `assets_hostsystem`, `assets_hostruntime`, `assets_hostdisk`, `assets_application`, `assets_business_system`, `assets_project`, `assets_business_environment`, `assets_application_version`, `assets_application_deployment_template`, `assets_cluster_profile`, `assets_application_service`, `assets_application_deployment`, `assets_application_service_deployment`, `assets_application_port`, `assets_application_path`, `assets_application_config_file`, `assets_application_log_definition`, `assets_application_service_log_setting`, `assets_application_control_action`, `assets_docker_control_config`, `assets_docker_compose_control_config`, `assets_agent_job`, `assets_agent_job_event`, `assets_webssh_session_log`, `assets_webssh_temp_credential` |

## Dependency order

1. `user`, `role`, `menu`, `sysconfig`: authentication, authorization and shared configuration.
2. `assets`: core inventory and agent identities used by every operational domain.
3. `scheduler` and `audit`: shared execution scheduling and traceability.
4. `automation`: inventories, playbooks, jobs and workflows.
5. `inspection`: depends on assets, applications and agent execution.
6. `monitor`: depends on assets, automation-style installation and alert recipients.

## Semantics that must survive

- Inventory and inspection host scope are fixed host-id snapshots, not dynamic host-group queries.
- User-role and role-menu assignment replace the complete relationship set transactionally.
- Host deletion and host-group deletion enforce references currently represented in JSON snapshots.
- Job, workflow, inspection and target records retain immutable execution snapshots.
- Audit retention and scheduler switches remain system configuration values.