from __future__ import annotations


WORKFLOW_RUNTIME_ACTIVE_STATUSES = {'waiting', 'pending', 'queued', 'running'}
WORKFLOW_RUNTIME_FAILURE_STATUSES = {'failed', 'cancelled'}
WORKFLOW_RUNTIME_FINAL_STATUSES = {'success', 'failed'}


def get_workflow_runtime_status(node_results, workflow_edges, fallback_status: str = 'pending') -> str:
    normalized_results = [item for item in (node_results or []) if isinstance(item, dict)]
    statuses = [str(item.get('status') or '').lower() for item in normalized_results]

    if any(status in WORKFLOW_RUNTIME_ACTIVE_STATUSES for status in statuses):
        return 'running'

    # 线性 success-only 语义：只要任一节点 failed/cancelled，run 即 failed。
    has_failure = any(
        str(item.get('status') or '').lower() in WORKFLOW_RUNTIME_FAILURE_STATUSES
        for item in normalized_results
    )

    if statuses:
        return 'failed' if has_failure else 'success'
    return str(fallback_status or 'pending').lower()