import os
import subprocess

from django.core.management.base import BaseCommand


class Command(BaseCommand):
    help = 'Run Celery beat process.'

    def add_arguments(self, parser):
        parser.add_argument('--loglevel', default='info', help='Celery log level')

    def handle(self, *args, **options):
        env = os.environ.copy()
        command = ['celery', '-A', 'djadmin', 'beat', '-l', str(options['loglevel'])]
        raise SystemExit(subprocess.call(command, env=env))
