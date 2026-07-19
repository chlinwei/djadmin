from __future__ import annotations

import logging
import threading

from .executor import execute_automation_job

logger = logging.getLogger(__name__)


def run_job_in_background(job_id: int) -> None:
    """Run ansible job in a local background thread (non-Celery)."""

    normalized_job_id = int(job_id)

    def _worker() -> None:
        try:
            execute_automation_job(normalized_job_id)
        except Exception as exc:
            try:
                from .models import AutomationExecutionJob

                job = AutomationExecutionJob.objects.filter(id=normalized_job_id).first()
                if job is not None and str(job.status).lower() in {"pending", "running"}:
                    summary = job.result_summary if isinstance(job.result_summary, dict) else {}
                    summary["message"] = f"Local background runner failed: {exc}"
                    job.status = AutomationExecutionJob.Status.FAILED
                    job.result_summary = summary
                    job.save(update_fields=["status", "result_summary", "update_time"])
            except Exception:
                # Never let error-handling path crash the runner thread.
                pass
            logger.error(
                "Background ansible job execution failed: job_id=%s err=%s",
                normalized_job_id,
                exc,
                exc_info=True,
            )

    threading.Thread(
        target=_worker,
        name=f"automation-job-{normalized_job_id}",
        daemon=True,
    ).start()
