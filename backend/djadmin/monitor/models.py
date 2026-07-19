from django.db import models

from djadmin.basemodel import BaseModel


class MonitorTarget(BaseModel):
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

	host = models.ForeignKey('assets.Host', on_delete=models.CASCADE, related_name='monitor_targets')
	exporter_type = models.CharField(max_length=32, choices=ExporterType.choices, default=ExporterType.NODE_EXPORTER)
	exporter_port = models.PositiveIntegerField(default=9100)
	managed_enabled = models.BooleanField(default=True)
	install_status = models.CharField(max_length=16, choices=InstallStatus.choices, default=InstallStatus.UNKNOWN)
	install_message = models.TextField(blank=True, default='')
	last_scrape_status = models.CharField(max_length=16, choices=ScrapeStatus.choices, default=ScrapeStatus.UNKNOWN)
	last_scrape_at = models.DateTimeField(null=True, blank=True)
	labels = models.JSONField(default=dict, blank=True)

	class Meta:
		db_table = 'monitor_target'
		ordering = ['-id']
		unique_together = ('host', 'exporter_type')

	def __str__(self):
		host_name = str(getattr(self.host, 'instance_name', '') or '')
		host_ip = str(getattr(self.host, 'ip', '') or '')
		host_pk = getattr(self, 'host_id', None)
		host_label = host_name or host_ip or f'host-{host_pk}'
		return f'{self.exporter_type}:{host_label}:{self.exporter_port}'
