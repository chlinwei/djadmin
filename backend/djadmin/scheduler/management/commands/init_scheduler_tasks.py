from django.core.management.base import BaseCommand
from scheduler.models import ScheduledTask
from django.utils import timezone


class Command(BaseCommand):
    help = 'Initialize default scheduled tasks'

    def handle(self, *args, **options):
        defaults = {
            'name': '主机信息采集',
            'description': '定时采集所有主机信息',
            'enabled': True,
            'interval_minutes': 15,
        }
        task, created = ScheduledTask.objects.get_or_create(
            code='collect_all_hosts_info',
            defaults=defaults,
        )
        if created:
            self.stdout.write(self.style.SUCCESS(f'Created task: {task.name}'))
        else:
            self.stdout.write(self.style.WARNING(f'Task already exists: {task.name}'))
