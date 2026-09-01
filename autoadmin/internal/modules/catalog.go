package modules

type Module struct {
	Name        string
	RoutePrefix string
	Tables      []string
}

// Catalog preserves the current Django ownership boundaries during the rewrite.
var Catalog = []Module{
	{Name: "user", RoutePrefix: "/sys", Tables: []string{"sys_user", "sys_user_role", "sys_agent_token", "sys_user_alert_media"}},
	{Name: "role", RoutePrefix: "/sys", Tables: []string{"sys_role"}},
	{Name: "menu", RoutePrefix: "/sys", Tables: []string{"sys_menu", "sys_role_menu"}},
	{Name: "sys_config", RoutePrefix: "/sys", Tables: []string{"sys_config"}},
	{Name: "scheduler", RoutePrefix: "/sys/scheduler", Tables: []string{"scheduler_scheduledtask", "scheduler_scheduledtasklog"}},
	{Name: "audit", RoutePrefix: "/sys/audit", Tables: []string{"audit_login_log", "audit_operation_log"}},
	{Name: "automation", RoutePrefix: "/sys/automation", Tables: []string{
		"automation_playbook_template", "automation_task", "automation_controller_ssh_key", "automation_inventory",
		"automation_execution_job", "automation_execution_host_log",
	}},
	{Name: "inspection", RoutePrefix: "/sys/inspection", Tables: []string{
		"inspection_group", "inspection_check", "inspection_task", "inspection_execution", "inspection_target_execution", "inspection_result",
	}},
	{Name: "monitor", RoutePrefix: "/monitor", Tables: []string{
		"monitor_target", "monitor_target_install_history", "monitor_alert_history", "monitor_alert_media", "monitor_alert_route",
		"monitor_alert_notification_event", "monitor_alert_notification_delivery", "monitor_user_alert_media_binding", "monitor_alert_route_media",
		"monitor_software_package", "monitor_opensearch_cluster", "monitor_log_retention_tier", "monitor_log_processing_rule",
		"monitor_log_collection_filter_rule", "monitor_log_collection_target",
	}},
	{Name: "assets", RoutePrefix: "/assets", Tables: []string{
		"assets_credential", "assets_hostgroup", "assets_cloudaccount", "assets_host", "assets_hostcredential", "assets_hosthardware",
		"assets_hostsystem", "assets_hostruntime", "assets_hostdisk", "assets_application", "assets_business_system", "assets_project",
		"assets_business_environment", "assets_application_version", "assets_application_deployment_template", "assets_cluster_profile",
		"assets_application_service", "assets_application_deployment", "assets_application_service_deployment", "assets_application_port",
		"assets_application_path", "assets_application_config_file", "assets_application_log_definition", "assets_application_service_log_setting",
		"assets_application_control_action", "assets_docker_control_config", "assets_docker_compose_control_config", "assets_agent_job",
		"assets_agent_job_event", "assets_webssh_session_log", "assets_webssh_temp_credential",
	}},
}
