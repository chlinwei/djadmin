from __future__ import annotations

import json
from typing import Any
from urllib import error as urllib_error
from urllib import parse as urllib_parse
from urllib import request as urllib_request

from django.contrib.auth.hashers import make_password

from sys_config.models import SysConfig


PROMETHEUS_BASE_URL_KEY = 'monitor.prometheus.base_url'
PROMETHEUS_BASE_URL_LEGACY_KEY = 'sys.monitor.prometheus.base_url'
DEFAULT_PROMETHEUS_BASE_URL = 'http://10.25.66.150:9999'


def _get_or_create_config_with_legacy_migration(
    *,
    key: str,
    legacy_key: str | None,
    defaults: dict[str, Any],
) -> SysConfig:
    cfg = SysConfig.objects.filter(key=key).order_by('id').first()
    if cfg is not None:
        if legacy_key:
            SysConfig.objects.filter(key=legacy_key).delete()
        return cfg

    legacy_cfg = None
    if legacy_key:
        legacy_cfg = SysConfig.objects.filter(key=legacy_key).order_by('id').first()

    cfg, _ = SysConfig.objects.get_or_create(key=key, defaults=defaults)
    if legacy_cfg is None:
        return cfg

    # 用户已明确要求去掉 sys 前缀：新键创建后自动吸收旧键值，避免手工迁移。
    legacy_value = str(legacy_cfg.value or defaults.get('value') or '')
    # secret 类型的旧键值是明文（还没有哈希机制时写进去的），迁移到新键时必须补一次哈希，
    # 否则会把明文 token 直接写进本应该只存哈希的字段。
    if defaults.get('value_type') == 'secret':
        cfg.value = make_password(legacy_value)
    else:
        cfg.value = legacy_value
    if cfg.default_value in (None, ''):
        cfg.default_value = str(legacy_cfg.default_value or defaults.get('default_value') or '')
    cfg.value_type = str(legacy_cfg.value_type or cfg.value_type)
    cfg.is_readonly = bool(legacy_cfg.is_readonly)
    cfg.save(update_fields=['value', 'default_value', 'value_type', 'is_readonly', 'update_time'])
    legacy_cfg.delete()
    return cfg


def get_prometheus_base_url() -> str:
    defaults = {
        'value': DEFAULT_PROMETHEUS_BASE_URL,
        'default_value': DEFAULT_PROMETHEUS_BASE_URL,
        'value_type': 'string',
        'name': 'Prometheus 基础地址',
        'description': '监控中心请求 Prometheus HTTP API 的基础地址',
        'is_readonly': False,
    }
    cfg = _get_or_create_config_with_legacy_migration(
        key=PROMETHEUS_BASE_URL_KEY,
        legacy_key=PROMETHEUS_BASE_URL_LEGACY_KEY,
        defaults=defaults,
    )
    return str(cfg.value or DEFAULT_PROMETHEUS_BASE_URL).rstrip('/')


def _build_url(path: str, params: dict[str, Any] | None = None) -> str:
    base_url = get_prometheus_base_url()
    normalized_path = path if path.startswith('/') else f'/{path}'
    if not params:
        return f'{base_url}{normalized_path}'
    query = urllib_parse.urlencode(params)
    return f'{base_url}{normalized_path}?{query}'


def api_get(path: str, params: dict[str, Any] | None = None, timeout_seconds: int = 8) -> dict[str, Any]:
    url = _build_url(path, params)
    req = urllib_request.Request(url=url, method='GET')
    req.add_header('Accept', 'application/json')
    try:
        with urllib_request.urlopen(req, timeout=timeout_seconds) as resp:
            payload_text = resp.read().decode('utf-8', errors='replace')
    except urllib_error.URLError as exc:
        return {
            'ok': False,
            'status': 'error',
            'error': f'prometheus request failed: {exc}',
            'data': {},
        }

    try:
        payload = json.loads(payload_text)
    except json.JSONDecodeError:
        return {
            'ok': False,
            'status': 'error',
            'error': f'prometheus invalid json response: {payload_text[:500]}',
            'data': {},
        }

    is_success = str(payload.get('status') or '').lower() == 'success'
    return {
        'ok': is_success,
        'status': str(payload.get('status') or ''),
        'error': str(payload.get('error') or ''),
        'errorType': str(payload.get('errorType') or ''),
        'warnings': payload.get('warnings') or [],
        'data': payload.get('data') if isinstance(payload.get('data'), dict) else payload.get('data') or {},
    }


def query_instant(promql: str, timeout_seconds: int = 8) -> dict[str, Any]:
    return api_get('/api/v1/query', params={'query': promql}, timeout_seconds=timeout_seconds)
