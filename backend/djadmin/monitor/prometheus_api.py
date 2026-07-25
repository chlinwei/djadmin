from __future__ import annotations

import json
import os
import shutil
import subprocess
import tempfile
from datetime import datetime
from typing import Any
from urllib import error as urllib_error
from urllib import parse as urllib_parse
from urllib import request as urllib_request

from sys_config.models import SysConfig


PROMETHEUS_BASE_URL_KEY = 'monitor.prometheus.base_url'
PROMETHEUS_BASE_URL_LEGACY_KEY = 'sys.monitor.prometheus.base_url'
DEFAULT_PROMETHEUS_BASE_URL = 'http://10.25.66.150:9999'
PROMETHEUS_HTTP_SD_TOKEN_KEY = 'monitor.prometheus.http_sd_token'
PROMETHEUS_HTTP_SD_TOKEN_LEGACY_KEY = 'sys.monitor.prometheus.http_sd_token'
DEFAULT_PROMETHEUS_HTTP_SD_TOKEN = 'REPLACE_ME'
PROMETHEUS_ALERT_RULES_FILE_PATH_KEY = 'monitor.prometheus.alert_rules_file_path'
PROMETHEUS_ALERT_RULES_FILE_PATH_LEGACY_KEY = 'sys.monitor.prometheus.alert_rules_file_path'
DEFAULT_PROMETHEUS_ALERT_RULES_FILE_PATH = '/data/apps/prometheus/config/rules/djadmin-alert-rules.yml'
PROMETHEUS_RELOAD_TIMEOUT_SECONDS_KEY = 'sys.monitor.prometheus.reload_timeout_seconds'
DEFAULT_PROMETHEUS_RELOAD_TIMEOUT_SECONDS = '8'
PROMETHEUS_DEPLOY_SKIP_PROMTOOL_KEY = 'sys.monitor.prometheus.deploy_skip_promtool'
DEFAULT_PROMETHEUS_DEPLOY_SKIP_PROMTOOL = 'false'


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
    cfg.value = str(legacy_cfg.value or defaults.get('value') or '')
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


def get_prometheus_http_sd_token() -> str:
    defaults = {
        'value': DEFAULT_PROMETHEUS_HTTP_SD_TOKEN,
        'default_value': DEFAULT_PROMETHEUS_HTTP_SD_TOKEN,
        'value_type': 'string',
        'name': 'Prometheus HTTP SD Token',
        'description': 'Prometheus 访问 HTTP SD 接口使用的 token',
        'is_readonly': False,
    }
    cfg = _get_or_create_config_with_legacy_migration(
        key=PROMETHEUS_HTTP_SD_TOKEN_KEY,
        legacy_key=PROMETHEUS_HTTP_SD_TOKEN_LEGACY_KEY,
        defaults=defaults,
    )
    return str(cfg.value or DEFAULT_PROMETHEUS_HTTP_SD_TOKEN).strip()


def get_prometheus_alert_rules_file_path() -> str:
    defaults = {
        'value': DEFAULT_PROMETHEUS_ALERT_RULES_FILE_PATH,
        'default_value': DEFAULT_PROMETHEUS_ALERT_RULES_FILE_PATH,
        'value_type': 'string',
        'name': 'Prometheus 告警规则文件路径',
        'description': '一键部署时写入的 Prometheus rule_files 文件绝对路径。',
        'is_readonly': False,
    }
    cfg = _get_or_create_config_with_legacy_migration(
        key=PROMETHEUS_ALERT_RULES_FILE_PATH_KEY,
        legacy_key=PROMETHEUS_ALERT_RULES_FILE_PATH_LEGACY_KEY,
        defaults=defaults,
    )
    return str(cfg.value or DEFAULT_PROMETHEUS_ALERT_RULES_FILE_PATH).strip()


def get_prometheus_reload_timeout_seconds() -> int:
    cfg, _ = SysConfig.objects.get_or_create(
        key=PROMETHEUS_RELOAD_TIMEOUT_SECONDS_KEY,
        defaults={
            'value': DEFAULT_PROMETHEUS_RELOAD_TIMEOUT_SECONDS,
            'default_value': DEFAULT_PROMETHEUS_RELOAD_TIMEOUT_SECONDS,
            'value_type': 'int',
            'name': 'Prometheus reload 超时（秒）',
            'description': '一键部署触发 /-/reload 的 HTTP 超时时间。',
            'is_readonly': False,
        },
    )
    try:
        value = int(str(cfg.value or DEFAULT_PROMETHEUS_RELOAD_TIMEOUT_SECONDS).strip())
    except ValueError:
        value = int(DEFAULT_PROMETHEUS_RELOAD_TIMEOUT_SECONDS)
    if value <= 0:
        return int(DEFAULT_PROMETHEUS_RELOAD_TIMEOUT_SECONDS)
    return value


def get_prometheus_deploy_skip_promtool() -> bool:
    cfg, _ = SysConfig.objects.get_or_create(
        key=PROMETHEUS_DEPLOY_SKIP_PROMTOOL_KEY,
        defaults={
            'value': DEFAULT_PROMETHEUS_DEPLOY_SKIP_PROMTOOL,
            'default_value': DEFAULT_PROMETHEUS_DEPLOY_SKIP_PROMTOOL,
            'value_type': 'bool',
            'name': '一键部署跳过 promtool 校验',
            'description': 'true 时部署不执行 promtool check rules。',
            'is_readonly': False,
        },
    )
    return str(cfg.value or '').strip().lower() in {'true', '1', 'yes'}


def deploy_alert_rules_yaml(content: str) -> dict[str, Any]:
    file_path = get_prometheus_alert_rules_file_path()
    if file_path == '':
        return {'ok': False, 'error': 'Prometheus 规则文件路径为空'}

    target_dir = os.path.dirname(file_path)
    if target_dir == '':
        return {'ok': False, 'error': f'无效规则文件路径：{file_path}'}

    os.makedirs(target_dir, exist_ok=True)

    backup_file_path = ''
    if os.path.exists(file_path):
        backup_file_path = f"{file_path}.bak.{datetime.now().strftime('%Y%m%d%H%M%S')}"
        shutil.copy2(file_path, backup_file_path)

    fd, temp_path = tempfile.mkstemp(prefix='djadmin-alert-rules-', suffix='.yml', dir=target_dir)
    try:
        with os.fdopen(fd, 'w', encoding='utf-8') as temp_file:
            temp_file.write(content)
        os.replace(temp_path, file_path)
    except Exception as exc:
        try:
            os.unlink(temp_path)
        except OSError:
            pass
        return {
            'ok': False,
            'error': f'写入规则文件失败：{exc}',
            'file_path': file_path,
            'backup_file_path': backup_file_path,
        }

    if not get_prometheus_deploy_skip_promtool():
        try:
            proc = subprocess.run(
                ['promtool', 'check', 'rules', file_path],
                capture_output=True,
                text=True,
                timeout=20,
                check=False,
            )
        except FileNotFoundError:
            if backup_file_path != '':
                shutil.copy2(backup_file_path, file_path)
            return {
                'ok': False,
                'error': '未找到 promtool，请安装后重试或将 sys.monitor.prometheus.deploy_skip_promtool 设为 true',
                'file_path': file_path,
                'backup_file_path': backup_file_path,
            }
        except Exception as exc:
            if backup_file_path != '':
                shutil.copy2(backup_file_path, file_path)
            return {
                'ok': False,
                'error': f'promtool 执行失败：{exc}',
                'file_path': file_path,
                'backup_file_path': backup_file_path,
            }

        if proc.returncode != 0:
            if backup_file_path != '':
                shutil.copy2(backup_file_path, file_path)
            error_text = (proc.stderr or proc.stdout or '').strip()
            return {
                'ok': False,
                'error': f'promtool 校验失败：{error_text}',
                'file_path': file_path,
                'backup_file_path': backup_file_path,
            }

    reload_url = f"{get_prometheus_base_url()}/-/reload"
    req = urllib_request.Request(url=reload_url, method='POST')
    timeout_seconds = get_prometheus_reload_timeout_seconds()
    try:
        with urllib_request.urlopen(req, timeout=timeout_seconds):
            pass
    except urllib_error.URLError as exc:
        if backup_file_path != '':
            shutil.copy2(backup_file_path, file_path)
        return {
            'ok': False,
            'error': f'触发 Prometheus reload 失败：{exc}',
            'file_path': file_path,
            'backup_file_path': backup_file_path,
            'reload_url': reload_url,
        }

    return {
        'ok': True,
        'file_path': file_path,
        'backup_file_path': backup_file_path,
        'reload_url': reload_url,
    }


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
