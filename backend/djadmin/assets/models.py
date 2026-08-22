from django.db import models
from django.core.validators import MaxValueValidator, MinValueValidator
from djadmin.basemodel import BaseModel

# Create your models here.


from django.utils import timezone

class Credential(BaseModel):

    class AuthType(models.IntegerChoices):
        PASSWORD = 1, "Password"
        SSH_KEY = 2, "SSH Key"

    name = models.CharField(max_length=200, blank=True, null=True)
    username = models.CharField(max_length=128, null=False, default="root")

    password = models.CharField(max_length=512, blank=True, null=True)
    private_key = models.TextField(blank=True, null=True)
    port = models.PositiveIntegerField(default=22)

    auth_type = models.IntegerField(
        choices=AuthType.choices,
        default=AuthType.PASSWORD
    )





class Application(BaseModel):
    class Category(models.TextChoices):
        WEB_CONTAINER = 'web_container', 'Web 容器'
        DATABASE = 'database', '数据库'
        MIDDLEWARE = 'middleware', '中间件'
        BUSINESS = 'business', '业务应用'
        OTHER = 'other', '其他'

    name = models.CharField(max_length=128, unique=True, verbose_name='应用名称')
    code = models.CharField(max_length=64, unique=True, verbose_name='应用编码')
    category = models.CharField(max_length=32, choices=Category.choices, default=Category.OTHER, verbose_name='应用类别')
    vendor = models.CharField(max_length=128, blank=True, default='', verbose_name='厂商')
    description = models.TextField(blank=True, default='', verbose_name='描述')
    enabled = models.BooleanField(default=True, verbose_name='是否启用')

    class Meta:
        ordering = ['-id']
        db_table = 'assets_application'


class ApplicationVersion(BaseModel):
    application = models.ForeignKey(Application, on_delete=models.CASCADE, related_name='versions')
    version = models.CharField(max_length=128, verbose_name='版本号')
    release_date = models.DateField(null=True, blank=True, verbose_name='发布日期')
    end_of_support = models.DateField(null=True, blank=True, verbose_name='停止支持日期')
    enabled = models.BooleanField(default=True, verbose_name='允许新部署')

    class Meta:
        db_table = 'assets_application_version'
        ordering = ['application_id', '-id']
        constraints = [
            models.UniqueConstraint(fields=['application', 'version'], name='unique_application_version'),
        ]

    def __str__(self):
        return f'{self.application.name} {self.version}'


class ApplicationDeploymentTemplate(BaseModel):
    class ControlType(models.TextChoices):
        SYSTEMD = 'systemd', 'Systemd'
        COMMAND = 'command', '命令行'
        EXTERNAL_HA = 'external_ha', '外部 HA'
        DOCKER = 'docker', 'Docker 容器'
        DOCKER_COMPOSE = 'docker_compose', 'Docker Compose'

    class SystemdScope(models.TextChoices):
        SYSTEM = 'system', '系统服务'
        USER = 'user', '用户服务'

    application = models.ForeignKey(Application, on_delete=models.CASCADE, related_name='deployment_templates')
    name = models.CharField(max_length=128, verbose_name='模板名称')
    control_type = models.CharField(max_length=32, choices=ControlType.choices)
    run_user = models.CharField(max_length=100, verbose_name='运行用户')
    run_group = models.CharField(max_length=100, blank=True, default='', verbose_name='运行组')
    app_home = models.CharField(max_length=512, blank=True, default='', verbose_name='App Home')
    work_directory = models.CharField(max_length=512, blank=True, default='', verbose_name='工作目录')
    service_name = models.CharField(max_length=255, blank=True, default='', verbose_name='Systemd 服务名')
    systemd_scope = models.CharField(
        max_length=16,
        choices=SystemdScope.choices,
        default=SystemdScope.SYSTEM,
        verbose_name='Systemd 作用域',
    )
    ha_system_name = models.CharField(max_length=128, blank=True, default='', verbose_name='HA 系统名称')
    ha_cluster_name = models.CharField(max_length=128, blank=True, default='', verbose_name='集群名称')
    ha_resource_name = models.CharField(max_length=128, blank=True, default='', verbose_name='资源名称')
    enabled = models.BooleanField(default=True, verbose_name='允许新部署')

    class Meta:
        db_table = 'assets_application_deployment_template'
        ordering = ['application_id', '-id']
        constraints = [
            models.UniqueConstraint(fields=['application', 'name'], name='unique_application_deployment_template'),
        ]


class ApplicationDeployment(BaseModel):
    class Environment(models.TextChoices):
        PRODUCTION = 'production', '生产'
        TESTING = 'testing', '测试'
        DEVELOPMENT = 'development', '开发'
        OTHER = 'other', '其他'

    class HealthStatus(models.TextChoices):
        UNKNOWN = 'unknown', '未检查'
        CHECKING = 'checking', '检查中'
        HEALTHY = 'healthy', '正常'
        UNHEALTHY = 'unhealthy', '异常'
        ERROR = 'error', '检查失败'

    application_version = models.ForeignKey(ApplicationVersion, on_delete=models.PROTECT, related_name='deployments')
    deployment_template = models.ForeignKey(ApplicationDeploymentTemplate, on_delete=models.PROTECT, related_name='deployments')
    host = models.ForeignKey('Host', on_delete=models.CASCADE, related_name='application_deployments')
    instance_name = models.CharField(max_length=128, verbose_name='实例名称')
    environment = models.CharField(max_length=16, choices=Environment.choices, default=Environment.PRODUCTION)
    enabled = models.BooleanField(default=True)
    health_status = models.CharField(max_length=16, choices=HealthStatus.choices, default=HealthStatus.UNKNOWN)
    baseline_pass_rate = models.FloatField(null=True, blank=True)
    last_check_time = models.DateTimeField(null=True, blank=True)

    class Meta:
        db_table = 'assets_application_deployment'
        ordering = ['-id']
        constraints = [
            models.UniqueConstraint(fields=['host', 'instance_name'], name='unique_host_application_instance'),
        ]


class ApplicationPort(BaseModel):
    class Protocol(models.TextChoices):
        TCP = 'tcp', 'TCP'
        UDP = 'udp', 'UDP'

    deployment_template = models.ForeignKey(ApplicationDeploymentTemplate, on_delete=models.CASCADE, related_name='ports')
    name = models.CharField(max_length=64)
    protocol = models.CharField(max_length=8, choices=Protocol.choices, default=Protocol.TCP)
    bind_address = models.CharField(max_length=255, blank=True, default='0.0.0.0')
    port = models.PositiveIntegerField(validators=[MinValueValidator(1), MaxValueValidator(65535)])
    required = models.BooleanField(default=True)
    external_access = models.BooleanField(default=False)
    check_enabled = models.BooleanField(default=True)

    class Meta:
        db_table = 'assets_application_port'
        ordering = ['deployment_template_id', 'protocol', 'port']
        constraints = [
            models.UniqueConstraint(fields=['deployment_template', 'protocol', 'port'], name='unique_template_protocol_port'),
        ]


class ApplicationPath(BaseModel):
    class PathType(models.TextChoices):
        HOME = 'home', 'Home 目录'
        BIN = 'bin', 'Bin 目录'
        CONFIG = 'config', '配置目录'
        DATA = 'data', '数据目录'
        LOG = 'log', '日志目录'
        PID = 'pid', 'PID 文件'
        EXECUTABLE = 'executable', '可执行文件'
        OTHER = 'other', '其他'

    deployment_template = models.ForeignKey(ApplicationDeploymentTemplate, on_delete=models.CASCADE, related_name='paths')
    name = models.CharField(max_length=64)
    path_type = models.CharField(max_length=16, choices=PathType.choices)
    path = models.CharField(max_length=512)
    required = models.BooleanField(default=True)
    expected_owner = models.CharField(max_length=100, blank=True, default='')
    expected_group = models.CharField(max_length=100, blank=True, default='')
    expected_mode = models.CharField(max_length=8, blank=True, default='')
    check_enabled = models.BooleanField(default=True)

    class Meta:
        db_table = 'assets_application_path'
        ordering = ['deployment_template_id', 'path_type', 'id']
        constraints = [
            models.UniqueConstraint(fields=['deployment_template', 'name'], name='unique_template_path_name'),
        ]


class ApplicationConfigFile(BaseModel):
    class FileFormat(models.TextChoices):
        XML = 'xml', 'XML'
        YAML = 'yaml', 'YAML'
        JSON = 'json', 'JSON'
        INI = 'ini', 'INI'
        PROPERTIES = 'properties', 'Properties'
        TEXT = 'text', '文本'

    deployment_template = models.ForeignKey(ApplicationDeploymentTemplate, on_delete=models.CASCADE, related_name='config_files')
    name = models.CharField(max_length=128)
    path = models.CharField(max_length=512)
    file_format = models.CharField(max_length=16, choices=FileFormat.choices, default=FileFormat.TEXT)
    required = models.BooleanField(default=True)
    baseline_enabled = models.BooleanField(default=True)

    class Meta:
        db_table = 'assets_application_config_file'
        ordering = ['deployment_template_id', 'id']
        constraints = [
            models.UniqueConstraint(fields=['deployment_template', 'path'], name='unique_template_config_path'),
        ]


class ApplicationLogDefinition(BaseModel):
    deployment_template = models.ForeignKey(ApplicationDeploymentTemplate, on_delete=models.CASCADE, related_name='logs')
    name = models.CharField(max_length=128)
    path_pattern = models.CharField(max_length=512)
    encoding = models.CharField(max_length=32, default='utf-8')
    collection_enabled = models.BooleanField(default=False)
    retention_days = models.PositiveIntegerField(default=30)

    class Meta:
        db_table = 'assets_application_log_definition'
        ordering = ['deployment_template_id', 'id']
        constraints = [
            models.UniqueConstraint(fields=['deployment_template', 'name'], name='unique_template_log_name'),
        ]


class ApplicationControlAction(BaseModel):
    class Action(models.TextChoices):
        START = 'start', '启动'
        STOP = 'stop', '停止'
        STATUS = 'status', '状态'
        RESTART = 'restart', '重启'
        RELOAD = 'reload', '重载'

    deployment_template = models.ForeignKey(ApplicationDeploymentTemplate, on_delete=models.CASCADE, related_name='control_actions')
    action = models.CharField(max_length=16, choices=Action.choices)
    command = models.TextField()
    timeout_seconds = models.PositiveIntegerField(default=60)
    success_exit_codes = models.JSONField(default=list, blank=True)

    class Meta:
        db_table = 'assets_application_control_action'
        ordering = ['deployment_template_id', 'id']
        constraints = [
            models.UniqueConstraint(fields=['deployment_template', 'action'], name='unique_template_control_action'),
        ]


class DockerControlConfig(BaseModel):
    deployment_template = models.OneToOneField(ApplicationDeploymentTemplate, on_delete=models.CASCADE, related_name='docker_config')
    container_name = models.CharField(max_length=255)
    docker_host = models.CharField(max_length=255, blank=True, default='unix:///var/run/docker.sock')
    expected_image = models.CharField(max_length=255, blank=True, default='')
    expected_image_tag = models.CharField(max_length=128, blank=True, default='')

    class Meta:
        db_table = 'assets_docker_control_config'


class DockerComposeControlConfig(BaseModel):
    deployment_template = models.OneToOneField(ApplicationDeploymentTemplate, on_delete=models.CASCADE, related_name='compose_config')
    project_name = models.CharField(max_length=255)
    service_name = models.CharField(max_length=255)
    compose_file_path = models.CharField(max_length=512)
    working_directory = models.CharField(max_length=512)
    env_file = models.CharField(max_length=512, blank=True, default='')
    expected_image = models.CharField(max_length=255, blank=True, default='')
    expected_image_tag = models.CharField(max_length=128, blank=True, default='')

    class Meta:
        db_table = 'assets_docker_compose_control_config'


class ApplicationBaselineExecution(BaseModel):
    class Status(models.TextChoices):
        QUEUED = 'queued', '等待中'
        RUNNING = 'running', '检查中'
        COMPLETED = 'completed', '已完成'
        FAILED = 'failed', '执行失败'

    deployment = models.ForeignKey(ApplicationDeployment, on_delete=models.CASCADE, related_name='baseline_executions')
    agent_job = models.OneToOneField('AgentJob', on_delete=models.SET_NULL, null=True, blank=True, related_name='application_baseline_execution')
    status = models.CharField(max_length=16, choices=Status.choices, default=Status.QUEUED)
    passed = models.BooleanField(null=True, blank=True)
    total_count = models.PositiveIntegerField(default=0)
    passed_count = models.PositiveIntegerField(default=0)
    summary = models.JSONField(default=dict, blank=True)
    error_message = models.TextField(blank=True, default='')
    requested_user_id = models.IntegerField(null=True, blank=True)
    requested_username = models.CharField(max_length=100, blank=True, default='')
    start_time = models.DateTimeField(null=True, blank=True)
    end_time = models.DateTimeField(null=True, blank=True)

    class Meta:
        db_table = 'assets_application_baseline_execution'
        ordering = ['-id']


class ApplicationBaselineResult(BaseModel):
    class Status(models.TextChoices):
        PASS = 'pass', '通过'
        FAIL = 'fail', '失败'
        ERROR = 'error', '错误'
        SKIPPED = 'skipped', '跳过'

    execution = models.ForeignKey(ApplicationBaselineExecution, on_delete=models.CASCADE, related_name='results')
    check_key = models.CharField(max_length=255)
    check_type = models.CharField(max_length=64)
    name = models.CharField(max_length=255)
    status = models.CharField(max_length=16, choices=Status.choices)
    expected_value = models.JSONField(null=True, blank=True)
    actual_value = models.JSONField(null=True, blank=True)
    message = models.TextField(blank=True, default='')

    class Meta:
        db_table = 'assets_application_baseline_result'
        ordering = ['id']
        indexes = [
            models.Index(fields=['execution', 'status']),
            models.Index(fields=['check_type', 'status']),
        ]



class HostGroup(BaseModel):
    name = models.CharField(max_length=128, unique=True)

    # 可选：支持分组嵌套（以后用）
    parent = models.ForeignKey(
        "self",
        on_delete=models.SET_NULL,
        null=True,
        blank=True,
        related_name="children"
    )

    def __str__(self):
        return self.name


class CloudAccount(BaseModel):

    CLOUD_TYPE_CHOICES = (
        ("vsphere", "vSphere"),
        ("aliyun", "阿里云"),
        ("aws", "AWS"),
        ("manual", "手动"),
    )

    name = models.CharField(max_length=128)

    cloud_type = models.CharField(max_length=32, choices=CLOUD_TYPE_CHOICES)

    # 通用字段
    endpoint = models.CharField(max_length=255, blank=True, null=True)

    # vSphere
    username = models.CharField(max_length=128, blank=True, null=True)
    password = models.CharField(max_length=128, blank=True, null=True)

    # 阿里云
    access_key = models.CharField(max_length=128, blank=True, null=True)
    secret_key = models.CharField(max_length=128, blank=True, null=True)

    
class Host(BaseModel):
    instance_name = models.CharField(max_length=128, blank=True, null=True)
    agent_id = models.CharField(max_length=128, blank=True, null=True, unique=True, verbose_name='Agent ID')
    ip = models.GenericIPAddressField(null=True)
    instance_id = models.CharField(max_length=128, blank=True, null=True)

    cloud_account = models.ForeignKey(
        "CloudAccount",
        on_delete=models.SET_NULL,
        null=True,
        blank=True
    )

    group = models.ForeignKey(
        "HostGroup",
        on_delete=models.SET_NULL,
        null=True,
        blank=True
    )
    status = models.CharField(max_length=32, default="running")
    is_deleted_in_cloud = models.BooleanField(default=False)

    # 采集状态（用于在列表中突出无法连接的主机）
    class CollectStatus(models.TextChoices):
        UNKNOWN = "unknown", "未采集"
        SUCCESS = "success", "成功"
        FAILED = "failed", "失败"

    collect_status = models.CharField(
        max_length=16, default=CollectStatus.UNKNOWN, verbose_name="采集状态"
    )
    collect_message = models.TextField(blank=True, default="", verbose_name="采集失败原因")
    collect_time = models.DateTimeField(null=True, blank=True, verbose_name="最后采集时间")
    
    # dj-agent 在线状态
    agent_online = models.BooleanField(default=False, verbose_name="Agent在线状态")
    agent_online_time = models.DateTimeField(null=True, blank=True, verbose_name="Agent最后心跳时间")
    webssh_default_username = models.CharField(max_length=100, default='root', blank=True)
    webssh_login_users = models.CharField(max_length=512, default='root', blank=True)

    def __str__(self):
        display_name = self.instance_name or f"Host-{self.id}"
        return f"{display_name} ({self.ip})"
    


from django.db import models
from django.db.models import Q

class HostCredential(BaseModel):
    host = models.ForeignKey("Host", on_delete=models.CASCADE)
    credential = models.ForeignKey("Credential", on_delete=models.CASCADE)
    is_default = models.BooleanField(default=False)

    class Meta:
        unique_together = ("host", "credential")

        constraints = [
            models.UniqueConstraint(
                fields=["host"],
                condition=Q(is_default=True),
                name="unique_default_credential_per_host"
            )
        ]



class HostHardware(BaseModel):
    host = models.OneToOneField(
        "Host",
        on_delete=models.CASCADE,
        related_name="hardware"
    )

    cpu_cores = models.IntegerField(null=True, blank=True)
    cpu_model = models.CharField(max_length=255, blank=True, null=True)

    memory_gb = models.FloatField(null=True, blank=True)

    disk_total_gb = models.FloatField(null=True, blank=True)

    architecture = models.CharField(max_length=64, blank=True, null=True)
    collected_at = models.DateTimeField(null=True, blank=True, verbose_name='最后采集时间')

    def __str__(self):
        host_label = self.host.instance_name or f"Host-{self.host_id}"
        return f"Hardware of {host_label}"
    

class HostSystem(BaseModel):
    host = models.OneToOneField(
        "Host",
        on_delete=models.CASCADE,
        related_name="system"
    )

    os_type = models.CharField(max_length=64, blank=True, null=True)
    os_version = models.CharField(max_length=128, blank=True, null=True)

    kernel_version = models.CharField(max_length=128, blank=True, null=True)

    hostname = models.CharField(max_length=128, blank=True, null=True)

    agent_version = models.CharField(max_length=64, blank=True, null=True)
    timezone_name = models.CharField(max_length=64, blank=True, null=True)
    utc_offset = models.CharField(max_length=16, blank=True, null=True)
    collector_source = models.CharField(max_length=32, blank=True, null=True)
    collected_at = models.DateTimeField(null=True, blank=True, verbose_name='最后采集时间')


    def __str__(self):
        host_label = self.host.instance_name or f"Host-{self.host_id}"
        return f"System of {host_label}"


class HostRuntime(BaseModel):
    """dj-agent 最近一次采集的主机瞬时状态，不保存历史趋势。"""

    host = models.OneToOneField(
        "Host",
        on_delete=models.CASCADE,
        related_name="runtime",
    )
    cpu_usage_percent = models.FloatField(null=True, blank=True)
    cpu_times = models.JSONField(default=dict, blank=True)
    memory_usage_percent = models.FloatField(null=True, blank=True)
    memory = models.JSONField(default=dict, blank=True)
    disk_io = models.JSONField(default=list, blank=True)
    os_uptime_seconds = models.BigIntegerField(null=True, blank=True)
    os_boot_time = models.DateTimeField(null=True, blank=True)
    metrics_sample_window_ms = models.PositiveIntegerField(null=True, blank=True)
    static_fingerprint = models.CharField(max_length=64, blank=True, default='')
    collected_at = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        host_label = self.host.instance_name or f"Host-{self.host_id}"
        return f"Runtime of {host_label}"


class AgentJob(BaseModel):
    class JobStatus(models.TextChoices):
        QUEUED = 'queued', 'Queued'
        RUNNING = 'running', 'Running'
        SUCCESS = 'success', 'Success'
        FAILED = 'failed', 'Failed'
        CANCELED = 'canceled', 'Canceled'
        TIMEOUT = 'timeout', 'Timeout'

    job_id = models.CharField(max_length=128, unique=True)
    client_request_id = models.CharField(max_length=128, null=True, blank=True, unique=True)
    agent_id = models.CharField(max_length=128)
    host = models.ForeignKey('Host', on_delete=models.SET_NULL, null=True, blank=True, related_name='agent_jobs')
    job_type = models.CharField(max_length=32)
    action = models.CharField(max_length=64)
    params = models.JSONField(default=dict, blank=True)
    timeout_seconds = models.PositiveIntegerField(default=30)
    status = models.CharField(max_length=16, choices=JobStatus.choices, default=JobStatus.QUEUED)
    picked_at = models.DateTimeField(null=True, blank=True)
    finished_at = models.DateTimeField(null=True, blank=True)
    result_data = models.JSONField(default=dict, blank=True)
    error_message = models.TextField(blank=True, default='')
    
    # 任务执行结果字段 - 用于 RabbitMQ/HTTP 上报
    exit_code = models.IntegerField(default=-1, verbose_name="退出码")
    stdout = models.TextField(blank=True, default='', verbose_name="标准输出")
    stderr = models.TextField(blank=True, default='', verbose_name="标准错误")

    class Meta:
        db_table = 'assets_agent_job'
        indexes = [
            models.Index(fields=['agent_id', 'status']),
            models.Index(fields=['status', 'create_time']),
        ]

    def __str__(self):
        return f"{self.job_id} ({self.status})"


class AgentJobEvent(BaseModel):
    tag = models.CharField(max_length=255)
    job_id = models.CharField(max_length=128, blank=True, default='')
    agent_id = models.CharField(max_length=128, blank=True, default='')
    event_type = models.CharField(max_length=64, blank=True, default='')
    payload = models.JSONField(default=dict, blank=True)

    class Meta:
        db_table = 'assets_agent_job_event'
        indexes = [
            models.Index(fields=['job_id', 'create_time']),
            models.Index(fields=['agent_id', 'create_time']),
            models.Index(fields=['tag', 'create_time']),
        ]

    def __str__(self):
        return f"{self.tag} ({self.job_id})"


class HostDisk(models.Model):
    host = models.ForeignKey(
        "Host",
        on_delete=models.CASCADE,
        related_name="disks"
    )

    device = models.CharField(max_length=64)
    mount_point = models.CharField(max_length=128, blank=True, null=True)

    size_gb = models.FloatField(null=True, blank=True)
    used_gb = models.FloatField(null=True, blank=True)

    filesystem = models.CharField(max_length=64, blank=True, null=True)

    def __str__(self):
        host_label = self.host.instance_name or f"Host-{self.host_id}"
        return f"{self.device} ({host_label})"


class WebSSHSessionLog(models.Model):
    class Status(models.TextChoices):
        CONNECTED = 'connected', '已连接'
        CLOSED = 'closed', '已关闭'
        FAILED = 'failed', '连接失败'

    host = models.ForeignKey('Host', on_delete=models.CASCADE, related_name='webssh_sessions')
    user_id = models.IntegerField(null=True, blank=True)
    username = models.CharField(max_length=100, blank=True, default='')
    requested_username = models.CharField(max_length=100, blank=True, default='')
    effective_username = models.CharField(max_length=100, blank=True, default='')
    switch_user_status = models.CharField(max_length=16, blank=True, default='none')
    switch_user_error = models.TextField(blank=True, default='')
    client_ip = models.CharField(max_length=64, blank=True, default='')
    user_agent = models.CharField(max_length=255, blank=True, default='')

    status = models.CharField(max_length=16, choices=Status.choices, default=Status.CONNECTED)
    start_time = models.DateTimeField(auto_now_add=True)
    end_time = models.DateTimeField(null=True, blank=True)
    duration_seconds = models.IntegerField(null=True, blank=True)
    close_code = models.IntegerField(null=True, blank=True)
    error_message = models.TextField(blank=True, default='')

    input_bytes = models.IntegerField(default=0)
    command_count = models.IntegerField(default=0)
    input_content = models.TextField(blank=True, default='')
    output_content = models.TextField(blank=True, default='')
    recorded_content_bytes = models.IntegerField(default=0)
    is_content_truncated = models.BooleanField(default=False)

    class Meta:
        db_table = 'assets_webssh_session_log'
        ordering = ['-start_time']

    def __str__(self):
        return f"{self.id} {self.username} {self.host.id}"


class WebSSHTempCredential(models.Model):
    """临时 WebSSH 凭证追踪表：记录手动输入的临时凭证与 WebSSH 会话的绑定关系。
    
    生命周期：
    1. 前端手动输入凭证 → 创建 Credential + 此记录（session_pk=null）
    2. SSH 连接建立后 → 写入 session_pk
    3. WebSSH 会话关闭时 → 删除此记录及关联凭证
    4. 兜底清理：Celery 任务定期清理超过 2 小时且会话已结束的孤立记录
    """
    credential = models.OneToOneField(
        Credential,
        on_delete=models.CASCADE,
        related_name='temp_credential_info',
    )
    # SSH 连接建立后绑定，用于判断会话是否仍在运行
    session_pk = models.IntegerField(null=True, blank=True, db_index=True)
    created_at = models.DateTimeField(auto_now_add=True)

    class Meta:
        db_table = 'assets_webssh_temp_credential'


