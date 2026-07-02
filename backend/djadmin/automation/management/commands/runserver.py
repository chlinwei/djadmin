from django.conf import settings

from daphne.management.commands.runserver import Command as DaphneRunserverCommand


class Command(DaphneRunserverCommand):
    """Keep `manage.py runserver` unchanged, but use settings defaults when addrport is omitted."""

    def handle(self, *args, **options):
        if not options.get('addrport'):
            host = getattr(settings, 'SERVER_HOST', '0.0.0.0')
            port = getattr(settings, 'SERVER_PORT', 8000)
            options['addrport'] = f'{host}:{port}'
        return super().handle(*args, **options)
