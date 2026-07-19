"""
URL configuration for djadmin project.

The `urlpatterns` list routes URLs to views. For more information please see:
    https://docs.djangoproject.com/en/5.1/topics/http/urls/
Examples:
Function views
    1. Add an import:  from my_app import views
    2. Add a URL to urlpatterns:  path('', views.home, name='home')
Class-based views
    1. Add an import:  from other_app.views import Home
    2. Add a URL to urlpatterns:  path('', Home.as_view(), name='home')
Including another URLconf
    1. Import the include() function: from django.urls import include, path
    2. Add a URL to urlpatterns:  path('blog/', include('blog.urls'))
"""
from django.urls import path,include,re_path
from djadmin import settings
from django.views.static import serve
from assets import views as assets_views
from sys_config import views as sys_config_views

urlpatterns = [
    path('api/agent/configs/by-key/<path:key>', sys_config_views.agent_config_by_key),
    path('api/agent/jobs/create', assets_views.agent_create_job),
    path('api/agent/jobs/create-batch', assets_views.agent_create_jobs_batch),
    path('api/agent/jobs/retry', assets_views.agent_retry_job),
    path('api/agent/jobs/cancel', assets_views.agent_cancel_job),
    path('api/agent/jobs/query', assets_views.agent_query_jobs),
    path('api/agent/jobs/query-chain', assets_views.agent_query_job_chain),
    path('api/agent/jobs/events', assets_views.agent_query_job_events),
    path('sys/',include('user.urls')),
    re_path('media/(?P<path>.*)', serve, {'document_root': settings.MEDIA_ROOT},
name='media'),
    path('sys/',include('role.urls')),
    path('sys/',include('menu.urls')),
    path('sys/scheduler/', include('scheduler.urls')),
    path('sys/audit/', include('audit.urls')),
    path('sys/', include('sys_config.urls')),
    path('sys/monitor/', include('monitor.urls')),
    path('sys/automation/', include('automation.urls')),
    path('assets/',include('assets.urls')),
]
