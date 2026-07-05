from celery import current_app


def has_active_celery_worker(timeout=1.0):
    """Check whether at least one Celery worker is online."""
    try:
        inspector = current_app.control.inspect(timeout=timeout)
        ping_result = inspector.ping() or {}
        return bool(ping_result)
    except Exception:
        return False
