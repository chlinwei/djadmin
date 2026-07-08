from __future__ import annotations


WORKFLOW_RUNTIME_ACTIVE_STATUSES = {'waiting', 'pending', 'queued', 'running'}
WORKFLOW_RUNTIME_FAILURE_STATUSES = {'failed', 'cancelled'}
WORKFLOW_RUNTIME_FINAL_STATUSES = {'success', 'failed'}


def get_workflow_runtime_status(node_results, workflow_edges, fallback_status: str = 'pending') -> str:
    normalized_results = [item for item in (node_results or []) if isinstance(item, dict)]
    statuses = [str(item.get('status') or '').lower() for item in normalized_results]

    if any(status in WORKFLOW_RUNTIME_ACTIVE_STATUSES for status in statuses):
        return 'running'

    error_handler_sources = set()
    for edge in workflow_edges or []:
        if not isinstance(edge, dict):
            continue
        source_key = str(edge.get('source_key') or '').strip()
        condition = str(edge.get('condition') or 'success').strip().lower() or 'success'
        if source_key and condition in {'failure', 'always'}:
            error_handler_sources.add(source_key)

    has_unhandled_failure = False
    for item in normalized_results:
        status = str(item.get('status') or '').lower()
        if status not in WORKFLOW_RUNTIME_FAILURE_STATUSES:
            continue
        node_key = str(item.get('node_key') or '').strip()
        if node_key not in error_handler_sources:
            has_unhandled_failure = True
            break

    if statuses:
        return 'failed' if has_unhandled_failure else 'success'
    return str(fallback_status or 'pending').lower()