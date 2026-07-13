from django.core.management.base import BaseCommand
from scheduler.models import ScheduledTask
from django.utils import timezone


class Command(BaseCommand):
    help = 'Initialize default scheduled tasks'

    def handle(self, *args, **options):
        task_defs = [
            {
                'code': 'collect_all_hosts_info',
                'name': '主机信息采集',
                'description': '定时采集所有主机信息',
                'enabled': True,
                'cron_expression': '*/15 * * * *',
            },
            {
                'code': 'cleanup_webssh_session_logs',
                'name': 'WebSSH 会话日志清理',
                'description': '按保留天数清理过期 WebSSH 会话审计日志',
                'enabled': True,
                'cron_expression': '0 0 * * *',
            },
        ]

        for item in task_defs:
            defaults = {
                'name': item['name'],
                'description': item['description'],
                'enabled': item['enabled'],
                'cron_expression': item['cron_expression'],
                'interval_minutes': None,
            }
            task, created = ScheduledTask.objects.get_or_create(
                code=item['code'],
                defaults=defaults,
            )
            if created:
                self.stdout.write(self.style.SUCCESS(f'Created task: {task.name}'))
            else:
                self.stdout.write(self.style.WARNING(f'Task already exists: {task.name}'))
