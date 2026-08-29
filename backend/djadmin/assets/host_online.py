from __future__ import annotations

from typing import Any

from .grpc_transfer.registry import REGISTRY
from .models import Host


def get_connected_agent_ids() -> set[str]:
    """获取当前所有 gRPC 已建立活跃 Session 的 Agent ID 集合。"""
    return set(REGISTRY.connected_agent_ids())


def is_agent_online(host_or_agent_id: Any) -> bool:
    """Agent 是否物理在线的唯一判定：必须绑定非空 agent_id 且在当前 gRPC 会话注册表中。"""
    if host_or_agent_id is None:
        return False
    agent_id = str(getattr(host_or_agent_id, 'agent_id', '') or host_or_agent_id or '').strip()
    if not agent_id:
        return False
    return REGISTRY.is_connected(agent_id)


def sync_host_online_status_to_db() -> set[str]:
    """将内存 gRPC 注册表的最新在线状态同步刷新到 Host 表，并返回在线 agent_id 集合。"""
    connected_ids = set(REGISTRY.connected_agent_ids())
    Host.objects.filter(agent_online=True).exclude(agent_id__in=connected_ids).update(agent_online=False)
    if connected_ids:
        Host.objects.filter(agent_id__in=connected_ids, agent_online=False).update(agent_online=True)
    return connected_ids
