#!/usr/bin/env python
"""
Direct scheduler startup script.
"""
import os
import sys
import subprocess

# Add the project directory to Python path
sys.path.insert(0, os.path.dirname(__file__))

if __name__ == '__main__':
    print("=" * 60)
    print("Starting Celery Scheduler Stack (Worker + Beat)")
    print("=" * 60)
    env = os.environ.copy()
    env.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
    command = [sys.executable, 'manage.py', 'runscheduler']
    sys.exit(subprocess.call(command, env=env))
