from django.core.management.base import BaseCommand
from django.utils import timezone
from scheduler.models import ScheduledTaskLog


class Command(BaseCommand):
    help = 'Cleanup scheduler logs by retention days and/or max rows per task'

    def add_arguments(self, parser):
        parser.add_argument(
            '--days',
            type=int,
            default=30,
            help='Delete logs older than N days. Set 0 to disable age-based cleanup.',
        )
        parser.add_argument(
            '--max-per-task',
            type=int,
            default=2000,
            help='Keep latest N rows per task. Set 0 to disable count-based cleanup.',
        )

    def handle(self, *args, **options):
        days = options['days']
        max_per_task = options['max_per_task']

        deleted_total = 0

        if days > 0:
            cutoff = timezone.now() - timezone.timedelta(days=days)
            deleted, _ = ScheduledTaskLog.objects.filter(run_time__lt=cutoff).delete()
            deleted_total += deleted
            self.stdout.write(self.style.SUCCESS(f'Age cleanup deleted rows: {deleted}'))

        if max_per_task > 0:
            task_ids = (
                ScheduledTaskLog.objects.values_list('task_id', flat=True)
                .distinct()
            )
            for task_id in task_ids:
                keep_ids = list(
                    ScheduledTaskLog.objects.filter(task_id=task_id)
                    .order_by('-run_time', '-id')
                    .values_list('id', flat=True)[:max_per_task]
                )
                if keep_ids:
                    deleted, _ = (
                        ScheduledTaskLog.objects.filter(task_id=task_id)
                        .exclude(id__in=keep_ids)
                        .delete()
                    )
                    deleted_total += deleted

            self.stdout.write(self.style.SUCCESS('Count cleanup finished.'))

        self.stdout.write(self.style.SUCCESS(f'Total deleted rows: {deleted_total}'))
