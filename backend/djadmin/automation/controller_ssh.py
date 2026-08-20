from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor, as_completed
import time

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from django.db import transaction

from assets.credential_crypto import decrypt_secret, encrypt_secret
from assets.grpc_transfer.client import AgentChannelClient, AgentGrpcTransferError

from .models import AutomationControllerSSHKey


def _is_transient_agent_channel_error(error: AgentGrpcTransferError) -> bool:
    """Only retry transport-level gaps caused by a backend restart/reconnect."""
    error_text = str(error).lower()
    return any(keyword in error_text for keyword in (
        '未建立 gRPC 通道连接',
        '等待 agent 响应超时',
        'agent 连接已断开',
        'session',
        'connection',
        'timeout',
    ))


def get_or_create_controller_ssh_key() -> tuple[str, str]:
    """返回平台控制节点的私钥和公钥；私钥仅以加密形式持久化。"""
    with transaction.atomic():
        controller_key = AutomationControllerSSHKey.objects.select_for_update().first()
        if controller_key is None:
            private_key = Ed25519PrivateKey.generate()
            private_key_text = private_key.private_bytes(
                encoding=serialization.Encoding.PEM,
                format=serialization.PrivateFormat.OpenSSH,
                encryption_algorithm=serialization.NoEncryption(),
            ).decode('utf-8')
            public_key_text = private_key.public_key().public_bytes(
                encoding=serialization.Encoding.OpenSSH,
                format=serialization.PublicFormat.OpenSSH,
            ).decode('utf-8') + ' djadmin-automation'
            controller_key = AutomationControllerSSHKey.objects.create(
                public_key=public_key_text,
                private_key=encrypt_secret(private_key_text),
            )

    return decrypt_secret(controller_key.private_key), controller_key.public_key


def sync_controller_public_key(agent_id: str, public_key: str, timeout_seconds: int = 30) -> None:
    """通过已认证的 agent 通道安装平台公钥，禁止 backend 直写远端文件。"""
    max_attempts = 5
    for attempt in range(1, max_attempts + 1):
        try:
            result = AgentChannelClient(agent_id).execute_automation(
                job_id=f'sync-automation-ssh-key-{agent_id}',
                params={'public_key': public_key},
                timeout_seconds=timeout_seconds,
                task_type='custom',
                action='sync_automation_ssh_key',
            )
            if str(result.get('status') or '').lower() != 'success':
                raise RuntimeError(str(result.get('error_message') or 'agent rejected controller SSH key'))
            return
        except AgentGrpcTransferError as error:
            if attempt == max_attempts or not _is_transient_agent_channel_error(error):
                raise
            # API 重启后 gRPC 注册表会暂时为空，等待 agent 主动重连再发送公钥。
            time.sleep(2)


def sync_controller_public_key_to_agents(agent_ids: list[str], public_key: str, concurrency: int) -> dict[str, str]:
    """并发同步公钥，返回每个失败 agent 的错误信息。"""
    unique_agent_ids = sorted({str(agent_id).strip() for agent_id in agent_ids if str(agent_id).strip()})
    if not unique_agent_ids:
        return {}

    failures: dict[str, str] = {}
    worker_count = max(1, min(int(concurrency), len(unique_agent_ids)))
    with ThreadPoolExecutor(max_workers=worker_count, thread_name_prefix='automation-key-sync') as executor:
        futures = {
            executor.submit(sync_controller_public_key, agent_id, public_key): agent_id
            for agent_id in unique_agent_ids
        }
        for future in as_completed(futures):
            agent_id = futures[future]
            try:
                future.result()
            except Exception as exc:
                failures[agent_id] = str(exc)
    return failures