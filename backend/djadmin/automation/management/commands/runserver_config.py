from django.conf import settings
from django.core.management import call_command
from django.core.management.base import BaseCommand


class Command(BaseCommand):
    help = 'Run development server with host/port from settings.py (SERVER_HOST/SERVER_PORT).'

    def add_arguments(self, parser):
        parser.add_argument('--noreload', action='store_true', default=False, help='Tells Django to NOT use the auto-reloader.')
        parser.add_argument('--nothreading', action='store_true', default=False, help='Tells Django to NOT use threading.')

    def handle(self, *args, **options):
        host = getattr(settings, 'SERVER_HOST', '0.0.0.0')
        port = getattr(settings, 'SERVER_PORT', 8000)
        addrport = f'{host}:{port}'
        self.stdout.write(self.style.SUCCESS(f'Using configured server address: {addrport}'))
        call_command(
            'runserver',
            addrport,
            use_reloader=not options.get('noreload', False),
            use_threading=not options.get('nothreading', False),
        )
