from datetime import datetime, timedelta, timezone as dt_timezone

import jwt
from django.conf import settings


DOWNLOAD_TICKET_TYPE = 'download'
UPLOAD_TICKET_TYPE = 'upload'


def _signing_key():
    return getattr(settings, 'TRANSFER_TICKET_SECRET', settings.SECRET_KEY)


def issue_download_ticket(*, user_id, host_id, remote_path):
    expire_seconds = int(getattr(settings, 'TRANSFER_TICKET_EXPIRE_SECONDS', 7200))
    now = datetime.now(dt_timezone.utc)
    payload = {
        'typ': DOWNLOAD_TICKET_TYPE,
        'uid': int(user_id) if user_id is not None else 0,
        'hid': int(host_id),
        'path': str(remote_path),
        'iat': int(now.timestamp()),
        'exp': int((now + timedelta(seconds=expire_seconds)).timestamp()),
    }
    token = jwt.encode(payload, _signing_key(), algorithm='HS256')
    if isinstance(token, bytes):
        return token.decode('utf-8')
    return token


def parse_download_ticket(token):
    payload = jwt.decode(token, _signing_key(), algorithms=['HS256'])
    if payload.get('typ') != DOWNLOAD_TICKET_TYPE:
        raise ValueError('invalid ticket type')
    return payload


def issue_upload_ticket(*, user_id, host_id, target_path, file_name):
    expire_seconds = int(getattr(settings, 'TRANSFER_TICKET_EXPIRE_SECONDS', 7200))
    now = datetime.now(dt_timezone.utc)
    payload = {
        'typ': UPLOAD_TICKET_TYPE,
        'uid': int(user_id) if user_id is not None else 0,
        'hid': int(host_id),
        'path': str(target_path),
        'name': str(file_name),
        'iat': int(now.timestamp()),
        'exp': int((now + timedelta(seconds=expire_seconds)).timestamp()),
    }
    token = jwt.encode(payload, _signing_key(), algorithm='HS256')
    if isinstance(token, bytes):
        return token.decode('utf-8')
    return token


def parse_upload_ticket(token):
    payload = jwt.decode(token, _signing_key(), algorithms=['HS256'])
    if payload.get('typ') != UPLOAD_TICKET_TYPE:
        raise ValueError('invalid ticket type')
    return payload
