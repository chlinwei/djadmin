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

	# 安装/卸载任务失败后自动重试的次数上限：达到该次数仍失败则终止自动重试，改为需要人工点击"重试"。
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
	# 安装/卸载完全复用监控软件仓库（SoftwarePackage）上绑定的 install_task/uninstall_task 执行，
	# 本机不再保留脚本副本；这里只记录最近一次下发的 AutomationExecutionJob id，
	# 供前端渲染“查看日志”跳转链接（日志本身以 automation 模块的执行记录为准，不在本模型重复存储）。
	last_install_job_id = models.PositiveIntegerField(
		null=True, blank=True,
		help_text='最近一次安装/卸载对应的 AutomationExecutionJob id，用于跳转查看执行日志',
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
	# 安装/卸载改为完全复用“自动化任务”功能（PlaybookTemplate + AutomationTask），不再由 dj-agent
	# 按 manage_script 驱动：安装/卸载各自对应一个 AutomationTask（其 playbook_template 内容可在
	# “模板 -> Playbook模板”页面查看/编辑），任务本身的 become_enabled/become_method/become_user
	# 决定是否以及如何 sudo 提权。on_delete=SET_NULL：任务被删除时软件包记录本身不应被级联删除，
	# 只是暂时失去可执行的安装/卸载入口，需要用户重新绑定。
	install_task = models.ForeignKey(
		'automation.AutomationTask', on_delete=models.SET_NULL, null=True, blank=True,
		related_name='+', help_text='安装该软件包使用的自动化任务（Playbook），内容在“模板->Playbook模板”中维护',
	)
	uninstall_task = models.ForeignKey(
		'automation.AutomationTask', on_delete=models.SET_NULL, null=True, blank=True,
		related_name='+', help_text='卸载该软件包使用的自动化任务（Playbook）',
	)
	# systemd unit 文件内容：安装 playbook 通过 extra_vars 拿到这段内容，写入
	# /usr/lib/systemd/system/<name>.service（文件名固定按软件包 name 拼接，见 service_unit_name）。
	service_file_content = models.TextField(
		blank=True, default='',
		help_text='systemd unit 文件内容，安装时写入 /usr/lib/systemd/system/<name>.service',
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
