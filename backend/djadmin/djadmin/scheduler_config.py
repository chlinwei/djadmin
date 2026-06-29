from apscheduler.schedulers.background import BackgroundScheduler
from django_apscheduler.jobstores import DjangoJobStore
from django.core.management import call_command

# Create a scheduler instance
scheduler = BackgroundScheduler()
scheduler.add_jobstore(DjangoJobStore(), 'default')

# Schedule the update command to run every 5 minutes
def schedule_update():
    scheduler.add_job(
        call_command, 
        args=('update_cache',), 
        trigger='interval', 
        minutes=5,
        id='update_cache_job',
        replace_existing=True
    )

# Start the scheduler
def start():
    scheduler.start()