import os
import subprocess

from django.core.management.base import BaseCommand


class Command(BaseCommand):
    help = 'Run Celery scheduler stack (worker + beat) with one command.'

    def handle(self, *args, **options):
        env = os.environ.copy()
        worker_command = ['celery', '-A', 'djadmin', 'worker', '-l', 'info', '--concurrency', '2']
        beat_command = ['celery', '-A', 'djadmin', 'beat', '-l', 'info']

        worker_proc = subprocess.Popen(worker_command, env=env)
        self.stdout.write(self.style.SUCCESS(f'Celery worker started (pid={worker_proc.pid})'))
        self.stdout.write(self.style.SUCCESS('Starting Celery beat...'))

        beat_exit_code = 0
        try:
            beat_exit_code = subprocess.call(beat_command, env=env)
        finally:
            if worker_proc.poll() is None:
                worker_proc.terminate()
                try:
                    worker_proc.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    worker_proc.kill()

        raise SystemExit(beat_exit_code)
