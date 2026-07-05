def schedule_update():
    """Compatibility shim for legacy imports."""
    return None


def start():
    """Compatibility shim for legacy imports."""
    from scheduler.tasks import dispatch_due_tasks
    dispatch_due_tasks.delay()