from django.urls import re_path

from .consumers import HostWebSSHConsumer

websocket_urlpatterns = [
    re_path(r'^ws/assets/hosts/(?P<host_id>\d+)/webssh/$', HostWebSSHConsumer.as_asgi()),
]
