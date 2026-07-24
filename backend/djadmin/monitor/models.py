from django.core.files.storage import FileSystemStorage
from django.db import models

from djadmin.basemodel import BaseModel


class OverwriteStorage(FileSystemStorage):
	"""软件包按固定官方命名存储：重传同名文件时直接覆盖，避免 Django 追加随机后缀，
	否则 agent 端按 uname 拼接的下载 URL 会命中失败。"""

	def get_available_name(self, name, max_length=None):
		if self.exists(name):
			self.delete(name)
		return name


def software_package_upload_to(instance, filename):
	# 使用 node_exporter 官方 tarball 命名规则，确保 agent 端拼出的下载 URL 能命中 media 文件
	safe_name = str(instance.name or 'package').strip()
	safe_version = str(instance.version or '0').strip().lstrip('v')
	return f'monitor_packages/{safe_name}-{safe_version}.{instance.os}-{instance.arch}.tar.gz'


# node_exporter 默认预置版本：与 dj_agent 内置安装脚本的 defaultNodeExporterVersion 保持一致，
# 保证“默认预置记录/自动更新”默认拉取的版本与 agent 回退逻辑一致。
DEFAULT_NODE_EXPORTER_VERSION = '1.8.2'


def build_node_exporter_official_url(version, os_name, arch):
	"""按官方 node_exporter release 命名规则拼接下载地址，供“自动更新”从 GitHub 官方源拉取。"""
	safe_version = str(version or '').strip().lstrip('v')
	tarball = f'node_exporter-{safe_version}.{os_name}-{arch}.tar.gz'
	return f'https://github.com/prometheus/node_exporter/releases/download/v{safe_version}/{tarball}'


class MonitorTarget(BaseModel):
	# 保留 NODE_EXPORTER 常量供代码内引用，但字段本身不再限定 choices——
	# exporter_type 现在存的是“监控软件仓库”中的软件包 name，用于支持一台主机纳管多个不同 exporter。
	class ExporterType(models.TextChoices):
		NODE_EXPORTER = 'node_exporter', 'Node Exporter'

	class InstallStatus(models.TextChoices):
		UNKNOWN = 'unknown', 'Unknown'
		PENDING = 'pending', 'Pending'
		SUCCESS = 'success', 'Success'
		FAILED = 'failed', 'Failed'

	class ScrapeStatus(models.TextChoices):
		UNKNOWN = 'unknown', 'Unknown'
		UP = 'up', 'Up'
		DOWN = 'down', 'Down'

	# 兼容历史字段：当前已不启用“自动重试”逻辑，失败后由用户手动重试。
	MAX_AUTO_RETRY = 3

	host = models.ForeignKey('assets.Host', on_delete=models.CASCADE, related_name='monitor_targets')
	# 不再使用 choices 限制：名称来自监控软件仓库（SoftwarePackage.name），支持任意 exporter 类型
	exporter_type = models.CharField(max_length=64, default=ExporterType.NODE_EXPORTER)
	managed_enabled = models.BooleanField(default=True)
	install_status = models.CharField(max_length=16, choices=InstallStatus.choices, default=InstallStatus.UNKNOWN)
	install_message = models.TextField(blank=True, default='')
	# 当前安装/卸载操作周期内已自动尝试的次数（成功或人工触发重试时会被重置为 0）
	retry_count = models.PositiveIntegerField(default=0)
	last_scrape_status = models.CharField(max_length=16, choices=ScrapeStatus.choices, default=ScrapeStatus.UNKNOWN)
	last_scrape_at = models.DateTimeField(null=True, blank=True)
	labels = models.JSONField(default=dict, blank=True)
	# 安装/卸载完全复用监控软件仓库（SoftwarePackage）上绑定的 install_playbook_template/
	# uninstall_playbook_template 执行，本机不再保留脚本副本；这里只记录最近一次下发的
	# AutomationExecutionJob id，供前端渲染“查看日志”跳转链接（日志本身以 automation 模块的
	# 执行记录为准，不在本模型重复存储）。
	last_install_job_id = models.PositiveIntegerField(
		null=True, blank=True,
		help_text='最近一次安装/卸载对应的 AutomationExecutionJob id，用于跳转查看执行日志',
	)
	# 标记最近一次下发是否为“人工点击重试”触发：用于失败文案区分与历史审计。
	last_dispatch_manual = models.BooleanField(
		default=False,
		help_text='最近一次安装/卸载下发是否为人工点击“重试”触发',
	)

	class Meta:
		db_table = 'monitor_target'
		ordering = ['-id']
		unique_together = ('host', 'exporter_type')

	def __str__(self):
		host_name = str(getattr(self.host, 'instance_name', '') or '')
		host_ip = str(getattr(self.host, 'ip', '') or '')
		host_pk = getattr(self, 'host_id', None)
		host_label = host_name or host_ip or f'host-{host_pk}'
		return f'{self.exporter_type}:{host_label}'


class MonitorTargetInstallHistory(BaseModel):
	"""监控目标安装/卸载历史快照。

	该表是 monitor 侧的长期可追溯历史，不依赖 automation 执行记录长期保留。
	即使 AutomationExecutionJob 被定时清理，仍可通过本表查看关键执行信息。
	"""

	class Action(models.TextChoices):
		INSTALL = 'install', 'Install'
		UNINSTALL = 'uninstall', 'Uninstall'

	class TriggerType(models.TextChoices):
		AUTO = 'auto', 'Auto'
		MANUAL = 'manual', 'Manual'

	class Status(models.TextChoices):
		PENDING = 'pending', 'Pending'
		RUNNING = 'running', 'Running'
		SUCCESS = 'success', 'Success'
		FAILED = 'failed', 'Failed'
		CANCELLED = 'cancelled', 'Cancelled'

	target = models.ForeignKey('monitor.MonitorTarget', on_delete=models.CASCADE, related_name='install_histories')
	host = models.ForeignKey('assets.Host', on_delete=models.SET_NULL, null=True, blank=True, related_name='+')

	# 下面三项是“外部任务关联快照”，用于从本地历史跳到自动化任务中心；
	# 本地可读性不依赖这些字段必须可解析。
	automation_job = models.ForeignKey('automation.AutomationExecutionJob', on_delete=models.SET_NULL, null=True, blank=True, related_name='+')
	automation_job_id_snapshot = models.PositiveIntegerField(null=True, blank=True, default=None)
	automation_job_uuid_snapshot = models.CharField(max_length=64, blank=True, default='')

	action = models.CharField(max_length=16, choices=Action.choices)
	trigger_type = models.CharField(max_length=16, choices=TriggerType.choices, default=TriggerType.AUTO)
	status = models.CharField(max_length=16, choices=Status.choices, default=Status.PENDING)

	host_id_snapshot = models.IntegerField(null=True, blank=True, default=None)
	host_name_snapshot = models.CharField(max_length=128, blank=True, default='')
	host_ip_snapshot = models.CharField(max_length=64, blank=True, default='')
	exporter_type_snapshot = models.CharField(max_length=64, blank=True, default='')

	summary_message = models.TextField(blank=True, default='')
	stdout_snapshot = models.TextField(blank=True, default='')
	stderr_snapshot = models.TextField(blank=True, default='')
	error_message_snapshot = models.TextField(blank=True, default='')
	result_summary_snapshot = models.JSONField(default=dict, blank=True)

	requested_user_id_snapshot = models.IntegerField(null=True, blank=True, default=None)
	requested_username_snapshot = models.CharField(max_length=100, blank=True, default='')

	start_time = models.DateTimeField(null=True, blank=True)
	end_time = models.DateTimeField(null=True, blank=True)
	duration_seconds = models.FloatField(null=True, blank=True)

	class Meta:
		db_table = 'monitor_target_install_history'
		ordering = ['-id']
		indexes = [
			models.Index(fields=['target', '-id'], name='mon_hist_target_desc_ix'),
			models.Index(fields=['status', '-id'], name='mon_hist_status_desc_ix'),
			models.Index(fields=['automation_job_id_snapshot'], name='monitor_hist_auto_job_id_idx'),
		]

	def __str__(self):
		target_id = getattr(self, 'target_id', None)
		return f'target={target_id} {self.action} [{self.status}]'


class SoftwarePackage(BaseModel):
	"""本地软件仓库：托管待下发到 agent 的二进制包（当前用于 node_exporter），文件落 media/monitor_packages/。"""

	class OSType(models.TextChoices):
		LINUX = 'linux', 'Linux'

	class ArchType(models.TextChoices):
		AMD64 = 'amd64', 'x86_64/amd64'
		ARM64 = 'arm64', 'aarch64/arm64'

	name = models.CharField(max_length=64, default='node_exporter')
	version = models.CharField(max_length=32)
	os = models.CharField(max_length=16, choices=OSType.choices, default=OSType.LINUX)
	arch = models.CharField(max_length=16, choices=ArchType.choices, default=ArchType.AMD64)
	# blank=True：允许先预置“未同步”占位记录（无文件），后续通过上传或自动更新补全
	file = models.FileField(upload_to=software_package_upload_to, storage=OverwriteStorage(), max_length=255, blank=True, default='')
	sha256 = models.CharField(max_length=64, blank=True, default='')
	size_bytes = models.BigIntegerField(default=0)
	enabled = models.BooleanField(default=True)
	# 安装/卸载直接绑定“Playbook 模板”（automation.PlaybookTemplate，分类需为 software_package），
	# 不再经由 AutomationTask 中转：dj-agent 下发时会把主机范围收窄到当前操作的单台主机
	# （见 assets.views.dispatch_exporter_install_job），AutomationTask 自身的主机/清单选择字段
	# 对这个场景完全不生效，强制用户先建一个任务纯属多余的一层封装，因此直接选模板即可。
	# 安装/卸载过程本身固定以 root 执行（需要创建系统用户、写 /usr/lib/systemd/system 等，
	# 必须 root 权限），不提供可配置项；这与下面 service_run_as_user/group（装好之后守护进程的
	# 运行身份）是完全独立的两回事。
	# on_delete=SET_NULL：模板被删除时软件包记录本身不应被级联删除，
	# 只是暂时失去可执行的安装/卸载入口，需要用户重新绑定。
	install_playbook_template = models.ForeignKey(
		'automation.PlaybookTemplate', on_delete=models.SET_NULL, null=True, blank=True,
		related_name='+', help_text='安装该软件包使用的 Playbook 模板（需为“软件包安装/卸载专用”分类）',
	)
	uninstall_playbook_template = models.ForeignKey(
		'automation.PlaybookTemplate', on_delete=models.SET_NULL, null=True, blank=True,
		related_name='+', help_text='卸载该软件包使用的 Playbook 模板（需为“软件包安装/卸载专用”分类）',
	)
	# 安装/卸载 Playbook 执行时的工作目录，默认 /tmp（与 AutomationTask.work_directory 的默认值保持一致）。
	work_directory = models.CharField(max_length=255, blank=True, default='/tmp', help_text='安装/卸载 Playbook 执行时的工作目录，默认为 /tmp')
	# systemd unit 文件内容：安装 playbook 通过 extra_vars 拿到这段内容，写入
	# /usr/lib/systemd/system/<name>.service（文件名固定按软件包 name 拼接，见 service_unit_name）。
	service_file_content = models.TextField(
		blank=True, default='',
		help_text='systemd unit 文件内容，安装时写入 /usr/lib/systemd/system/<name>.service',
	)
	# 安装完成后 systemd 服务常驻运行时使用的系统用户/组（与 AutomationTask.run_as_user/run_as_group
	# 是两回事：那两个字段控制“安装过程本身”以什么身份执行，这两个字段控制“装好之后的守护进程”以什么身份
	# 常驻运行）。默认值 dj-agent：与 dj-agent 自身的运行账号保持一致，是本仓库约定的标准运行账号；
	# service_run_as_user 为必填项（不允许留空，避免服务意外以 root 常驻）。
	service_run_as_user = models.CharField(
		max_length=100, default='dj-agent',
		help_text='安装后 systemd 服务运行时使用的系统用户（必填，安装 playbook 据此创建该系统用户），默认 dj-agent',
	)
	service_run_as_group = models.CharField(
		max_length=100, blank=True, default='dj-agent',
		help_text='服务运行时的系统组，默认 dj-agent，留空则使用 service_run_as_user 的主组',
	)

	class Meta:
		db_table = 'monitor_software_package'
		ordering = ['-id']
		unique_together = ('name', 'version', 'os', 'arch')

	def __str__(self):
		return f'{self.name}-{self.version}.{self.os}-{self.arch}'

	@property
	def service_unit_name(self):
		"""固定按软件包 name 拼接 systemd unit 名（如 node_exporter.service），安装/启停均按此约定。"""
		return f'{self.name}.service'
