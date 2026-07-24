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


def serve_media(request, path, document_root=None, show_indexes=False):
    """媒体文件下载视图。

    Django 内置 serve() 会用 mimetypes.guess_type() 猜测编码，对 .tar.gz/.gz 等文件名
    会返回 encoding='gzip'，serve() 因此会自动设置 Content-Encoding: gzip 响应头。
    但文件本身已经是原始压缩包字节（未被二次传输编码），这个头是误报——会导致遵守
    Content-Encoding 的 HTTP 客户端（Ansible get_url、requests/urllib3、curl --compressed）
    对响应体自动做一次 gunzip 解压，破坏下载内容（表现为 checksum 校验失败）。
    媒体文件始终应作为原始字节下载，这里强制去掉该响应头。
    """
    response = serve(request, path, document_root=document_root, show_indexes=show_indexes)
    if response.has_header('Content-Encoding'):
        del response['Content-Encoding']
    return response


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
    re_path('media/(?P<path>.*)', serve_media, {'document_root': settings.MEDIA_ROOT},
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
