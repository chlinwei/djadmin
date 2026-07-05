import os
import subprocess

from django.core.management.base import BaseCommand


class Command(BaseCommand):
    help = 'Run Celery worker process.'

    def add_arguments(self, parser):
        parser.add_argument('--loglevel', default='info', help='Celery log level')
        parser.add_argument('--concurrency', default='2', help='Worker concurrency')

    def handle(self, *args, **options):
        env = os.environ.copy()
        command = [
            'celery',
            '-A',
            'djadmin',
            'worker',
            '-l',
            str(options['loglevel']),
            '--concurrency',
            str(options['concurrency']),
        ]
        raise SystemExit(subprocess.call(command, env=env))
