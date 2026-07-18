import base64
from functools import lru_cache
from hashlib import sha256

from cryptography.fernet import Fernet, InvalidToken
from django.conf import settings


ENCRYPTED_PREFIX = 'enc:v1:'


def is_encrypted_secret(value):
    return isinstance(value, str) and value.startswith(ENCRYPTED_PREFIX)


@lru_cache(maxsize=1)
def _get_fernet():
    configured_key = str(getattr(settings, 'ASSETS_CREDENTIAL_ENCRYPTION_KEY', '') or '').strip()
    if configured_key:
        key = configured_key.encode('utf-8')
    else:
        # Fallback keeps compatibility without adding required env vars.
        key = base64.urlsafe_b64encode(sha256(str(settings.SECRET_KEY).encode('utf-8')).digest())
    return Fernet(key)


def encrypt_secret(value):
    if value is None:
        return None
    text = str(value)
    if text == '' or is_encrypted_secret(text):
        return text
    token = _get_fernet().encrypt(text.encode('utf-8')).decode('utf-8')
    return f'{ENCRYPTED_PREFIX}{token}'


def decrypt_secret(value):
    if value is None:
        return None
    text = str(value)
    if text == '' or not is_encrypted_secret(text):
        return text

    token = text[len(ENCRYPTED_PREFIX):]
    try:
        return _get_fernet().decrypt(token.encode('utf-8')).decode('utf-8')
    except (InvalidToken, ValueError, TypeError) as exc:
        raise ValueError('凭证密文解密失败，请检查 ASSETS_CREDENTIAL_ENCRYPTION_KEY 配置') from exc