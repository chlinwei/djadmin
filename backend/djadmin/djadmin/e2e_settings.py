"""E2E settings: isolated MySQL database for Playwright runs.

Uses a dedicated schema `djadmin_e2e` to avoid polluting the default database.
"""

from .settings import *  # type: ignore[wildcard-import]

DATABASES = {
    'default': {
        'ENGINE': 'django.db.backends.mysql',
        'NAME': os.getenv('DJADMIN_E2E_DB_NAME', 'djadmin_e2e'),
        'USER': os.getenv('DJADMIN_E2E_DB_USER', 'root'),
        'PASSWORD': os.getenv('DJADMIN_E2E_DB_PASSWORD', '1qazXSW@'),
        'HOST': os.getenv('DJADMIN_E2E_DB_HOST', '10.25.66.150'),
        'PORT': os.getenv('DJADMIN_E2E_DB_PORT', '3400'),
    }
}

# Speed up hashing in local E2E setup.
PASSWORD_HASHERS = [
    'django.contrib.auth.hashers.MD5PasswordHasher',
]
