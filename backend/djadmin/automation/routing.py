from django.urls import re_path

from .consumers import AutomationJobLogConsumer, AutomationWorkflowRunConsumer

websocket_urlpatterns = [
    re_path(r'^ws/automation/jobs/(?P<job_id>\d+)/logs/$', AutomationJobLogConsumer.as_asgi()),
    re_path(r'^ws/automation/workflow-runs/(?P<run_id>\d+)/stream/$', AutomationWorkflowRunConsumer.as_asgi()),
]
