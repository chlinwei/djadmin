from django.urls import include, path
from rest_framework.routers import DefaultRouter

from .views import LoginAuditLogManage, OperationAuditLogManage, WebSSHSessionLogAuditManage

router = DefaultRouter()
router.register(r'webssh-sessions', WebSSHSessionLogAuditManage, basename='audit-webssh-sessions')
router.register(r'login-logs', LoginAuditLogManage, basename='audit-login-logs')
router.register(r'operation-logs', OperationAuditLogManage, basename='audit-operation-logs')

urlpatterns = [
    path('', include(router.urls)),
]
