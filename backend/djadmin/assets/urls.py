from django.urls import path,include
from rest_framework.routers import DefaultRouter
from .views import *
router = DefaultRouter()
router.register(r'credentials',CredentialManage,basename="credentials")
router.register(r'applications',ApplicationManage,basename="applications")
router.register(r'host-groups',HostGroupManage,basename="host-groups")
router.register(r'hosts',HostManage,basename="hosts")

urlpatterns = [
    path('', include(router.urls)),
    # dj-agent HTTP 报告端点
    path('agent/heartbeat', agent_heartbeat, name='agent-heartbeat'),
    path('agent/report-host-snapshot', agent_report_host_snapshot, name='agent-report-host-snapshot'),
    path('agent/report-job-result', agent_report_job_result, name='agent-report-job-result'),
    path('agent/report-job-event', agent_report_job_event, name='agent-report-job-event'),
    path('agent/report-terminal-event', agent_report_terminal_event, name='agent-report-terminal-event'),
]

