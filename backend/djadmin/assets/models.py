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
        db_table = 'assets_application'
        ordering = ['-id']


class BusinessSystem(BaseModel):
    name = models.CharField(max_length=128, verbose_name='业务系统名称')
    code = models.CharField(max_length=64, verbose_name='业务系统编码')
    # 反向名沿用 business_systems，使 Project 侧的查询/序列化写法在 M2M 改 FK 后保持不变。
    project = models.ForeignKey(
        'Project',
        on_delete=models.PROTECT,
        null=True,
        blank=True,
        related_name='business_systems',
        verbose_name='所属项目',
    )
    owner = models.CharField(max_length=128, blank=True, default='', verbose_name='负责人')
    enabled = models.BooleanField(default=True, verbose_name='是否启用')

    class Meta:
        db_table = 'assets_business_system'
        ordering = ['name', 'id']
        constraints = [
            models.UniqueConstraint(fields=['project', 'name'], name='unique_project_business_system_name'),
            models.UniqueConstraint(fields=['project', 'code'], name='unique_project_business_system_code'),
        ]

    def __str__(self):
        return self.name


class Project(BaseModel):
    """长期业务集合：一个项目包含多个业务系统，业务系统只属于一个项目（反向关系 business_systems）。

    项目编码会拼进日志 data stream 名，因此必须能由服务唯一推导。
    """

    name = models.CharField(max_length=128, unique=True, verbose_name='项目名称')
    code = models.CharField(max_length=64, unique=True, verbose_name='项目编码')
    owner = models.CharField(max_length=128, blank=True, default='', verbose_name='负责人')
    enabled = models.BooleanField(default=True, verbose_name='是否启用')

    class Meta:
        db_table = 'assets_project'
        ordering = ['name', 'id']

    def __str__(self):
        return self.name


class BusinessEnvironment(BaseModel):
    """全局环境字典，供不同业务系统和逻辑服务复用。"""

    name = models.CharField(max_length=64, verbose_name='环境名称')
    code = models.CharField(max_length=32, verbose_name='环境编码')
    # 用于服务树中固定“生产在前、开发在后”这类展示顺序，避免按名称排序导致次序不稳定。
    order = models.PositiveIntegerField(default=0, verbose_name='展示顺序')
    owner = models.CharField(max_length=128, blank=True, default='', verbose_name='负责人')
    enabled = models.BooleanField(default=True, verbose_name='是否启用')

    class Meta:
        db_table = 'assets_business_environment'
        ordering = ['order', 'name', 'id']
        constraints = [
            models.UniqueConstraint(fields=['code'], name='unique_business_environment_code'),
            models.UniqueConstraint(fields=['name'], name='unique_business_environment_name'),
        ]

    def __str__(self):
        return self.name


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
    macro_definitions = models.JSONField(default=list, blank=True, verbose_name='宏定义')

    class Meta:
        db_table = 'assets_application_deployment_template'
        ordering = ['application_id', '-id']
        constraints = [
            models.UniqueConstraint(fields=['application', 'name'], name='unique_application_deployment_template'),
        ]


class ClusterProfile(BaseModel):
    class ProfileType(models.TextChoices):
        BUILTIN = 'builtin', '内置'
        CUSTOM = 'custom', '自定义'

    class ClusterType(models.TextChoices):
        MYSQL = 'mysql', 'MySQL 集群'
        REDIS = 'redis', 'Redis 集群'
        NACOS = 'nacos', 'Nacos 集群'
        ELASTICSEARCH = 'elasticsearch', 'Elasticsearch 集群'
        HA = 'ha', 'HA 集群'
        CUSTOM = 'custom', '自定义集群'

    name = models.CharField(max_length=128, unique=True, verbose_name='集群模型名称')
    code = models.CharField(max_length=64, unique=True, verbose_name='集群模型编码')
    application = models.ForeignKey(
        Application,
        on_delete=models.PROTECT,
        related_name='cluster_profiles',
        null=True,
        blank=True,
        verbose_name='应用',
    )
    profile_type = models.CharField(max_length=16, choices=ProfileType.choices, default=ProfileType.CUSTOM)
    cluster_type = models.CharField(max_length=24, choices=ClusterType.choices, default=ClusterType.CUSTOM)
    enabled = models.BooleanField(default=True, verbose_name='是否启用')

    class Meta:
        db_table = 'assets_cluster_profile'
        ordering = ['name', 'id']


class ApplicationService(BaseModel):
    class TopologyType(models.TextChoices):
        STANDALONE = 'standalone', '单机'
        CLUSTER = 'cluster', '集群'
        LOAD_BALANCER = 'load_balancer', '负载均衡'

    business_system = models.ForeignKey(
        BusinessSystem,
        on_delete=models.PROTECT,
        related_name='services',
        verbose_name='所属业务系统',
    )
    environment = models.ForeignKey(
        BusinessEnvironment,
        on_delete=models.PROTECT,
        related_name='services',
        null=True,
        blank=True,
        verbose_name='所属环境',
    )
    application = models.ForeignKey(Application, on_delete=models.PROTECT, related_name='services')
    deployments = models.ManyToManyField(
        'ApplicationDeployment',
        through='ApplicationServiceDeployment',
        related_name='application_services',
        blank=True,
        verbose_name='部署实例',
    )
    application_version = models.ForeignKey(
        ApplicationVersion,
        on_delete=models.PROTECT,
        related_name='services',
        verbose_name='应用版本',
    )
    deployment_template = models.ForeignKey(
        ApplicationDeploymentTemplate,
        on_delete=models.PROTECT,
        related_name='services',
        verbose_name='部署模板',
    )
    macro_values = models.JSONField(default=dict, blank=True, verbose_name='宏值')
    cluster_profile = models.ForeignKey(
        ClusterProfile,
        on_delete=models.PROTECT,
        related_name='services',
        null=True,
        blank=True,
        verbose_name='集群模型',
    )
    name = models.CharField(max_length=128, verbose_name='逻辑服务名称')
    code = models.CharField(max_length=64, unique=True, verbose_name='逻辑服务编码')
    topology_type = models.CharField(max_length=16, choices=TopologyType.choices, default=TopologyType.STANDALONE)
    access_address = models.CharField(max_length=255, blank=True, default='', verbose_name='服务入口地址')
    enabled = models.BooleanField(default=True, verbose_name='是否启用')
    # 服务级日志采集开关：与日志定义的 collection_enabled 共同为 ON 时，服务下所有部署实例均采集，
    # 实例层不设开关（新增实例自动继承），见架构文档 §6。
    log_collection_enabled = models.BooleanField(default=False, verbose_name='开启日志采集')
    log_retention_tier = models.ForeignKey(
        'monitor.LogRetentionTier', on_delete=models.PROTECT, null=True, blank=True,
        related_name='services', verbose_name='日志保留档位',
    )

    class Meta:
        db_table = 'assets_application_service'
        ordering = ['business_system_id', 'environment_id', 'name']
        constraints = [
            models.UniqueConstraint(
                fields=['business_system', 'environment', 'name'],
                name='unique_business_environment_service',
            ),
        ]


class ApplicationDeployment(BaseModel):

    class RuntimeStatus(models.TextChoices):
        UNKNOWN = 'unknown', '未知'
        RUNNING = 'running', '运行中'
        STOPPED = 'stopped', '已停止'
        ERROR = 'error', '状态检查失败'

    class HaRole(models.TextChoices):
        UNKNOWN = 'unknown', '未知'
        PRIMARY = 'primary', '主'
        STANDBY = 'standby', '备'

    host = models.ForeignKey('Host', on_delete=models.CASCADE, related_name='application_deployments')
    instance_name = models.CharField(max_length=128, verbose_name='实例名称')
    enabled = models.BooleanField(default=True)
    runtime_status = models.CharField(max_length=16, choices=RuntimeStatus.choices, default=RuntimeStatus.UNKNOWN)
    runtime_status_output = models.TextField(blank=True, default='')
    last_status_check_time = models.DateTimeField(null=True, blank=True)
    ha_role = models.CharField(max_length=16, choices=HaRole.choices, default=HaRole.UNKNOWN)

    # 版本与模板由所属逻辑服务统一定义，实例侧不再冗余存储，避免两处配置漂移。
    @property
    def service(self):
        return self.application_services.first()  # type: ignore[attr-defined]

    @property
    def application_version(self):
        service = self.service
        return service.application_version if service else None

    @property
    def deployment_template(self):
        service = self.service
        return service.deployment_template if service else None

    class Meta:
        db_table = 'assets_application_deployment'
        ordering = ['-id']
        constraints = [
            models.UniqueConstraint(fields=['host', 'instance_name'], name='unique_host_application_instance'),
        ]


class ApplicationServiceDeployment(BaseModel):
    service = models.ForeignKey(ApplicationService, on_delete=models.CASCADE, related_name='deployment_links')
    deployment = models.ForeignKey(ApplicationDeployment, on_delete=models.CASCADE, related_name='service_links')
    enabled = models.BooleanField(default=True, verbose_name='是否启用')

    class Meta:
        db_table = 'assets_application_service_deployment'
        constraints = [
            models.UniqueConstraint(fields=['service', 'deployment'], name='unique_application_service_deployment'),
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
    collection_enabled = models.BooleanField(default=False)
    # 保留期改由逻辑服务的 log_retention_tier 在索引级经 ISM 执行，日志定义级不再配置 retention_days
    # （定义级配置了也无法执行，保留会造成误导），见架构文档 §7。
    processing_rule = models.ForeignKey(
        'monitor.LogProcessingRule', on_delete=models.PROTECT, null=True, blank=True,
        related_name='log_definitions', verbose_name='日志处理规则',
    )
    extra_fields = models.JSONField(default=dict, blank=True, verbose_name='附加标签')

    class Meta:
        db_table = 'assets_application_log_definition'
        ordering = ['deployment_template_id', 'id']
        constraints = [
            models.UniqueConstraint(fields=['deployment_template', 'name'], name='unique_template_log_name'),
        ]


class ApplicationServiceLogSetting(BaseModel):
    """逻辑服务对模板日志定义的覆盖。

    档位是索引名的一段，不同档位即不同 data stream，ISM 才能分别过期；
    而“同一模板被不同业务复用、但某条日志要单独长留”是业务差异，只能落在服务级。
    """

    service = models.ForeignKey(
        'assets.ApplicationService', on_delete=models.CASCADE, related_name='log_settings',
    )
    log_definition = models.ForeignKey(
        ApplicationLogDefinition, on_delete=models.CASCADE, related_name='service_overrides',
    )
    # 两个字段留空都表示继承：档位继承逻辑服务，采集开关继承模板日志定义。
    retention_tier = models.ForeignKey(
        'monitor.LogRetentionTier', on_delete=models.PROTECT, null=True, blank=True,
        related_name='service_log_settings', verbose_name='日志保留档位',
    )
    collection_enabled = models.BooleanField(null=True, blank=True, verbose_name='采集开关')
    collection_filter_rule = models.ForeignKey(
        'monitor.LogCollectionFilterRule', on_delete=models.PROTECT, null=True, blank=True,
        related_name='service_log_settings', verbose_name='采集过滤规则',
    )
    processing_rule = models.ForeignKey(
        'monitor.LogProcessingRule', on_delete=models.PROTECT, null=True, blank=True,
        related_name='service_log_settings', verbose_name='日志处理规则',
    )

    class Meta:
        db_table = 'assets_application_service_log_setting'
        ordering = ['service_id', 'log_definition_id']
        constraints = [
            models.UniqueConstraint(fields=['service', 'log_definition'], name='unique_service_log_setting'),
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


def default_tomcat_manager_roles():
    """保留给历史迁移 0048 加载旧字段默认值。"""
    return [
        'manager',
        'manager-gui',
        'admin',
        'admin-gui',
        'manager-script',
        'manager-jmx',
        'manager-status',
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
    environment = models.ForeignKey(
        BusinessEnvironment,
        on_delete=models.PROTECT,
        related_name='hosts',
        null=True,
        blank=True,
        verbose_name='所属环境',
    )

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
    # 安装包选择必须使用 /etc/os-release 的稳定机器字段，不能从展示名称猜发行版。
    os_id = models.CharField(max_length=64, blank=True, null=True)
    os_id_like = models.CharField(max_length=128, blank=True, null=True)
    os_version_id = models.CharField(max_length=64, blank=True, null=True)

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
    
    # Agent 任务通过 gRPC 返回的执行结果
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


