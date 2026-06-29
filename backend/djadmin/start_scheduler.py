#!/usr/bin/env python
"""
Direct scheduler startup script - bypasses Django management command discovery
"""
import os
import sys
import django
import time

# Add the project directory to Python path
sys.path.insert(0, os.path.dirname(__file__))

# Setup Django settings
os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

# Import and start scheduler
from scheduler_manager import start

if __name__ == '__main__':
    print("=" * 60)
    print("Starting APScheduler Daemon")
    print("=" * 60)
    
    try:
        scheduler = start()
        print("\n✓ Scheduler started successfully!")
        print("Press Ctrl+C to stop the scheduler...\n")
        
        # Keep the scheduler running
        while True:
            time.sleep(60)
    except KeyboardInterrupt:
        print("\n\nShutting down scheduler...")
        if scheduler:
            scheduler.shutdown()
        print("Scheduler stopped.")
        sys.exit(0)
    except Exception as e:
        print(f"\n✗ Error starting scheduler: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)
