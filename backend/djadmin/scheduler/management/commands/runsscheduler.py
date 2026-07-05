from django.core.management import call_command
from django.core.management.base import BaseCommand


class Command(BaseCommand):
    help = 'Alias of runscheduler to keep backward compatibility with common typo.'

    def handle(self, *args, **options):
        call_command('runscheduler')
