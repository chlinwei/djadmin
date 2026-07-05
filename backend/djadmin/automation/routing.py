from django.urls import re_path

from .consumers import AutomationJobLogConsumer

websocket_urlpatterns = [
    re_path(r'^ws/automation/jobs/(?P<job_id>\d+)/logs/$', AutomationJobLogConsumer.as_asgi()),
]
