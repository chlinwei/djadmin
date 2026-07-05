from .settings import *  # noqa

ROOT_URLCONF = 'transfer.urls'

# transfer 服务通过 ticket 做鉴权，不依赖业务 JWT 中间件
MIDDLEWARE = [m for m in MIDDLEWARE if m != 'user.middleware.JwtAuthenticationMiddleware']
