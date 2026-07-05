"""
ASGI config for djadmin project.

It exposes the ASGI callable as a module-level variable named ``application``.

For more information on this file, see
https://docs.djangoproject.com/en/5.1/howto/deployment/asgi/
"""

import os

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')

from channels.routing import ProtocolTypeRouter, URLRouter
from django.core.asgi import get_asgi_application

django_asgi_app = get_asgi_application()
from assets.routing import websocket_urlpatterns as assets_websocket_urlpatterns
from automation.routing import websocket_urlpatterns as automation_websocket_urlpatterns

websocket_urlpatterns = [
	*assets_websocket_urlpatterns,
	*automation_websocket_urlpatterns,
]

application = ProtocolTypeRouter({
	'http': django_asgi_app,
	'websocket': URLRouter(websocket_urlpatterns),
})
