from django.core.management.base import BaseCommand
from djadmin.scheduler_config import start, schedule_update

class Command(BaseCommand):
    help = 'Runs the APScheduler to execute scheduled tasks'

    def handle(self, *args, **options):
        # Schedule the update task
        schedule_update()
        
        # Start the scheduler
        start()
        
        self.stdout.write(
            self.style.SUCCESS('Successfully started the APScheduler')
        )