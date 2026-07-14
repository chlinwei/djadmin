import os
from django.core.management.base import BaseCommand


class Command(BaseCommand):
    help = 'Run dedicated transfer service via Daphne.'

    def add_arguments(self, parser):
        parser.add_argument('--host', default='0.0.0.0', help='Transfer service bind host')
        parser.add_argument('--port', default='9101', help='Transfer service bind port')

    def handle(self, *args, **options):
        env = os.environ.copy()
        env['DJANGO_SETTINGS_MODULE'] = 'djadmin.transfer_settings'
        os.execvpe(
            'daphne',
            [
                'daphne',
                '-b',
                str(options['host']),
                '-p',
                str(options['port']),
                'djadmin.asgi:application',
            ],
            env,
        )
