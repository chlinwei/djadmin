import time
from django.core.management.base import BaseCommand
from djadmin.scheduler_manager import start


class Command(BaseCommand):
    help = 'Run the APScheduler background scheduler for periodic host collection.'

    def handle(self, *args, **options):
        scheduler = start()
        self.stdout.write(self.style.SUCCESS('APScheduler started.'))
        try:
            while True:
                time.sleep(60)
        except KeyboardInterrupt:
            scheduler.shutdown()
            self.stdout.write(self.style.WARNING('APScheduler stopped.'))
