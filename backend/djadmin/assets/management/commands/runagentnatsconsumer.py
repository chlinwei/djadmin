import asyncio
import json
import signal
from typing import Any

from asgiref.sync import sync_to_async
from django.conf import settings
from django.core.management.base import BaseCommand
from django.utils import timezone

from assets.models import AgentJob, AgentJobEvent, Host, HostHardware, HostSystem


class Command(BaseCommand):
    help = "Consume agent uplink messages from NATS and persist to AgentJob/AgentJobEvent"

    def add_arguments(self, parser):
        parser.add_argument(
            "--nats-url",
            default=str(getattr(settings, "NATS_URL", "nats://127.0.0.1:4222") or "nats://127.0.0.1:4222"),
            help="NATS server URL (default from NATS_URL setting)",
        )

    def handle(self, *args, **options):
        nats_url = str(options.get("nats_url") or "nats://127.0.0.1:4222").strip()
        asyncio.run(self._run(nats_url))

    async def _run(self, nats_url: str):
        from nats.aio.client import Client as NATS

        nc = NATS()
        await nc.connect(servers=[nats_url], connect_timeout=2)

        stop_event = asyncio.Event()
        loop = asyncio.get_running_loop()
        for sig in (signal.SIGINT, signal.SIGTERM):
            try:
                loop.add_signal_handler(sig, stop_event.set)
            except NotImplementedError:
                pass

        async def on_result(msg):
            await self._handle_result_message(msg.subject, msg.data)

        async def on_event(msg):
            await self._handle_event_message(msg.subject, msg.data)

        async def on_heartbeat(msg):
            await self._handle_heartbeat_message(msg.subject, msg.data)

        async def on_host_report(msg):
            await self._handle_host_report_message(msg.subject, msg.data)

        await nc.subscribe("ret.job.>", cb=on_result)
        await nc.subscribe("evt.job.>", cb=on_event)
        await nc.subscribe("hb.agent.>", cb=on_heartbeat)
        await nc.subscribe("rpt.host.>", cb=on_host_report)
        await nc.flush(timeout=2)

        self.stdout.write(self.style.SUCCESS(f"NATS consumer started: {nats_url}"))
        self.stdout.write("Subscribed: ret.job.>, evt.job.>, hb.agent.>, rpt.host.>")

        await stop_event.wait()
        self.stdout.write("Stopping NATS consumer...")
        await nc.drain()
        await nc.close()

    async def _handle_result_message(self, subject: str, raw_data: bytes):
        payload = self._safe_json_loads(raw_data)
        await sync_to_async(self._persist_result, thread_sensitive=True)(subject, payload)

    async def _handle_event_message(self, subject: str, raw_data: bytes):
        payload = self._safe_json_loads(raw_data)
        await sync_to_async(self._persist_event, thread_sensitive=True)(subject, payload)

    async def _handle_heartbeat_message(self, subject: str, raw_data: bytes):
        payload = self._safe_json_loads(raw_data)
        await sync_to_async(self._persist_heartbeat, thread_sensitive=True)(subject, payload)

    async def _handle_host_report_message(self, subject: str, raw_data: bytes):
        payload = self._safe_json_loads(raw_data)
        await sync_to_async(self._persist_host_report, thread_sensitive=True)(subject, payload)

    @staticmethod
    def _safe_json_loads(raw_data: bytes) -> dict[str, Any]:
        try:
            decoded = json.loads(raw_data.decode("utf-8"))
            if isinstance(decoded, dict):
                return decoded
        except Exception:
            return {}
        return {}

    def _persist_result(self, subject: str, payload: dict[str, Any]):
        job_id = str(payload.get("job_id") or "").strip()
        if job_id == "":
            return

        job = AgentJob.objects.filter(job_id=job_id).first()
        if job is None:
            return

        if job.status == AgentJob.JobStatus.CANCELED:
            return

        now = timezone.now()
        previous_status = str(job.status or '').strip().lower()
        status_raw = str(payload.get("status") or "").strip().lower()
        action = str(payload.get("action") or "").strip()
        error_message = str(payload.get("error_message") or payload.get("error") or "").strip()
        stdout_text = str(payload.get("stdout") or "")
        stderr_text = str(payload.get("stderr") or "")
        result_data = payload.get("result_data")
        if not isinstance(result_data, dict):
            result_data = payload.get("data") if isinstance(payload.get("data"), dict) else {}
        if stdout_text:
            result_data["stdout"] = stdout_text
        if stderr_text:
            result_data["stderr"] = stderr_text

        if action:
            job.action = action
        job.result_data = result_data
        job.error_message = error_message

        if status_raw == AgentJob.JobStatus.SUCCESS:
            job.status = AgentJob.JobStatus.SUCCESS
            job.finished_at = now
        elif status_raw == AgentJob.JobStatus.TIMEOUT:
            job.status = AgentJob.JobStatus.TIMEOUT
            job.finished_at = now
        elif status_raw == AgentJob.JobStatus.CANCELED:
            job.status = AgentJob.JobStatus.CANCELED
            job.finished_at = now
        elif status_raw == AgentJob.JobStatus.RUNNING:
            job.status = AgentJob.JobStatus.RUNNING
            if job.picked_at is None:
                job.picked_at = now
        else:
            job.status = AgentJob.JobStatus.FAILED
            job.finished_at = now

        update_fields = ["action", "status", "result_data", "error_message", "update_time"]
        if job.finished_at is not None:
            update_fields.append("finished_at")
        if job.picked_at is not None:
            update_fields.append("picked_at")
        job.save(update_fields=update_fields)

        self._sync_automation_job_from_agent(
            job=job,
            previous_status=previous_status,
            stdout_text=stdout_text,
            stderr_text=stderr_text,
        )

        self._create_job_event(
            job=job,
            event_type="ret",
            payload={
                "id": job.agent_id,
                "jid": job.job_id,
                "retcode": 0 if job.status == AgentJob.JobStatus.SUCCESS else 1,
                "fun": job.action,
                "return": job.result_data,
                "status": job.status,
                "error": job.error_message,
                "source_subject": subject,
            },
        )

        host_id = int(getattr(job, "host_id", 0) or 0)
        if host_id <= 0:
            return

        host = Host.objects.filter(id=host_id).only("id").first()
        if host is None:
            return

        if job.status != AgentJob.JobStatus.SUCCESS:
            host.collect_status = Host.CollectStatus.FAILED
            host.collect_message = job.error_message or "agent collect failed"
            host.collect_time = now
            host.save(update_fields=["collect_status", "collect_message", "collect_time"])
            return

        # Only host-info style result updates host system/hardware snapshots.
        if str(job.action or "").strip() != "get_host_info":
            return

        data = result_data if isinstance(result_data, dict) else {}
        system_obj, _ = HostSystem.objects.get_or_create(host=host)
        system_obj.os_type = str(data.get("os") or system_obj.os_type or "").strip() or None
        system_obj.hostname = str(data.get("hostname") or system_obj.hostname or "").strip() or None
        system_obj.agent_version = str(data.get("agent_version") or system_obj.agent_version or "").strip() or None
        system_obj.collector_source = "agent"
        system_obj.collected_at = now
        system_obj.update_time = now
        system_obj.save()

        hardware_obj, _ = HostHardware.objects.get_or_create(host=host)
        cpu_count = data.get("cpu_count")
        arch = str(data.get("arch") or "").strip()
        go_version = str(data.get("go_version") or "").strip()
        if isinstance(cpu_count, int):
            hardware_obj.cpu_cores = cpu_count
        if arch:
            hardware_obj.architecture = arch
        if go_version:
            hardware_obj.cpu_model = go_version
        hardware_obj.collected_at = now
        hardware_obj.update_time = now
        hardware_obj.save()

        host.collect_status = Host.CollectStatus.SUCCESS
        host.collect_message = ""
        host.collect_time = now
        host.save(update_fields=["collect_status", "collect_message", "collect_time"])

    def _persist_event(self, subject: str, payload: dict[str, Any]):
        job_id = str(payload.get("job_id") or "").strip()
        agent_id = str(payload.get("agent_id") or "").strip()

        subject_parts = subject.split(".")
        event_type = str(payload.get("event_type") or "").strip()
        if event_type == "" and len(subject_parts) >= 4:
            event_type = subject_parts[3]

        AgentJobEvent.objects.create(
            tag=subject,
            job_id=job_id,
            agent_id=agent_id,
            event_type=event_type,
            payload=payload,
        )

    def _persist_heartbeat(self, subject: str, payload: dict[str, Any]):
        agent_id = str(payload.get("agent_id") or "").strip()
        now = timezone.now()

        host_id = Host.objects.filter(instance_name=agent_id).values_list("id", flat=True).first() if agent_id else None
        if host_id is not None:
            # 心跳用于判活：只刷新最近在线时间，不覆盖采集失败状态与错误信息。
            Host.objects.filter(id=host_id).update(collect_time=now)

            system_obj, _ = HostSystem.objects.get_or_create(host_id=host_id)
            system_obj.collector_source = "agent"
            system_obj.update_time = now
            system_obj.save(update_fields=["collector_source", "update_time"])

        AgentJobEvent.objects.create(
            tag=subject,
            job_id="",
            agent_id=agent_id,
            event_type="heartbeat",
            payload=payload,
        )

    def _persist_host_report(self, subject: str, payload: dict[str, Any]):
        agent_id = str(payload.get("agent_id") or "").strip()
        if agent_id == "":
            return

        now = timezone.now()
        status_raw = str(payload.get("status") or "").strip().lower()
        error_message = str(payload.get("error_message") or payload.get("error") or "").strip()
        result_data = payload.get("result_data")
        if not isinstance(result_data, dict):
            result_data = payload.get("data") if isinstance(payload.get("data"), dict) else {}

        host_id = Host.objects.filter(instance_name=agent_id).values_list("id", flat=True).first()
        if host_id is None:
            AgentJobEvent.objects.create(
                tag=subject,
                job_id="",
                agent_id=agent_id,
                event_type="host_report_orphan",
                payload=payload,
            )
            return

        if status_raw != AgentJob.JobStatus.SUCCESS:
            Host.objects.filter(id=host_id).update(
                collect_status=Host.CollectStatus.FAILED,
                collect_message=error_message or "agent active report failed",
                collect_time=now,
            )
            AgentJobEvent.objects.create(
                tag=subject,
                job_id="",
                agent_id=agent_id,
                event_type="host_report_failed",
                payload=payload,
            )
            return

        system_obj, _ = HostSystem.objects.get_or_create(host_id=host_id)
        system_obj.os_type = str(result_data.get("os") or system_obj.os_type or "").strip() or None
        system_obj.hostname = str(result_data.get("hostname") or system_obj.hostname or "").strip() or None
        system_obj.agent_version = str(result_data.get("agent_version") or system_obj.agent_version or "").strip() or None
        system_obj.collector_source = "agent"
        system_obj.collected_at = now
        system_obj.update_time = now
        system_obj.save()

        hardware_obj, _ = HostHardware.objects.get_or_create(host_id=host_id)
        cpu_count = result_data.get("cpu_count")
        arch = str(result_data.get("arch") or "").strip()
        go_version = str(result_data.get("go_version") or "").strip()
        if isinstance(cpu_count, int):
            hardware_obj.cpu_cores = cpu_count
        if arch:
            hardware_obj.architecture = arch
        if go_version:
            hardware_obj.cpu_model = go_version
        hardware_obj.collected_at = now
        hardware_obj.update_time = now
        hardware_obj.save()

        Host.objects.filter(id=host_id).update(
            collect_status=Host.CollectStatus.SUCCESS,
            collect_message="",
            collect_time=now,
        )

        AgentJobEvent.objects.create(
            tag=subject,
            job_id="",
            agent_id=agent_id,
            event_type="host_report",
            payload=payload,
        )

    @staticmethod
    def _create_job_event(job: AgentJob, event_type: str, payload: dict[str, Any]):
        job_id = str(getattr(job, "job_id", "") or "").strip()
        agent_id = str(getattr(job, "agent_id", "") or "").strip()
        if event_type == "new":
            tag = f"salt/job/{job_id}/new"
        elif event_type == "ret":
            safe_agent_id = agent_id or "unknown"
            tag = f"salt/job/{job_id}/ret/{safe_agent_id}"
        else:
            tag = f"salt/job/{job_id}/{event_type}"

        AgentJobEvent.objects.create(
            tag=tag,
            job_id=job_id,
            agent_id=agent_id,
            event_type=event_type,
            payload=payload if isinstance(payload, dict) else {},
        )

    def _sync_automation_job_from_agent(
        self,
        job: AgentJob,
        previous_status: str,
        stdout_text: str,
        stderr_text: str,
    ):
        params = job.params if isinstance(job.params, dict) else {}
        automation_job_id_raw = params.get('automation_execution_job_id')
        if not str(automation_job_id_raw or '').isdigit():
            return

        automation_job_id = int(str(automation_job_id_raw))

        try:
            from automation.models import AnsibleExecutionJob
        except Exception:
            return

        automation_job = AnsibleExecutionJob.objects.filter(id=automation_job_id).first()
        if automation_job is None:
            return

        related_jobs = list(
            AgentJob.objects.filter(params__automation_execution_job_id=automation_job_id).only(
                'id', 'status', 'error_message', 'params', 'result_data',
            )
        )
        if len(related_jobs) == 0:
            return

        total = len(related_jobs)
        queued = 0
        running = 0
        success = 0
        failed = 0
        canceled = 0
        timeout = 0
        for item in related_jobs:
            current_status = str(item.status or '').strip().lower()
            if current_status == AgentJob.JobStatus.QUEUED:
                queued += 1
            elif current_status == AgentJob.JobStatus.RUNNING:
                running += 1
            elif current_status == AgentJob.JobStatus.SUCCESS:
                success += 1
            elif current_status == AgentJob.JobStatus.CANCELED:
                canceled += 1
            elif current_status == AgentJob.JobStatus.TIMEOUT:
                timeout += 1
            else:
                failed += 1

        terminal = success + failed + canceled + timeout
        if terminal == total:
            next_status = (
                AnsibleExecutionJob.Status.SUCCESS
                if failed + canceled + timeout == 0
                else AnsibleExecutionJob.Status.FAILED
            )
        elif running > 0 or terminal > 0:
            next_status = AnsibleExecutionJob.Status.RUNNING
        else:
            next_status = AnsibleExecutionJob.Status.PENDING

        update_fields = ['status', 'result_summary']
        now = timezone.now()
        if automation_job.start_time is None and (running > 0 or terminal > 0):
            automation_job.start_time = now
            update_fields.append('start_time')

        if next_status in {AnsibleExecutionJob.Status.SUCCESS, AnsibleExecutionJob.Status.FAILED}:
            automation_job.end_time = now
            update_fields.append('end_time')
            if automation_job.start_time is None:
                automation_job.start_time = now
                update_fields.append('start_time')
            automation_job.duration_seconds = (automation_job.end_time - automation_job.start_time).total_seconds()
            update_fields.append('duration_seconds')

        automation_job.status = next_status
        automation_job.result_summary = {
            'message': 'Agent execution synced',
            'total': total,
            'queued': queued,
            'running': running,
            'success': success,
            'failed': failed,
            'canceled': canceled,
            'timeout': timeout,
        }

        current_status = str(job.status or '').strip().lower()
        if previous_status != current_status and current_status in {
            AgentJob.JobStatus.SUCCESS,
            AgentJob.JobStatus.FAILED,
            AgentJob.JobStatus.CANCELED,
            AgentJob.JobStatus.TIMEOUT,
        }:
            host_id = params.get('host_id')
            host_ip = params.get('host_ip')
            header = (
                f"\n\n===== Agent Host #{host_id or '-'} ({host_ip or '-'}) | "
                f"status={current_status} | agent_job={job.job_id} =====\n"
            )
            output_chunks = [header]
            if stdout_text:
                output_chunks.append(stdout_text.rstrip('\n') + '\n')
            if stderr_text:
                output_chunks.append('[stderr]\n' + stderr_text.rstrip('\n') + '\n')
            if job.error_message:
                output_chunks.append('[error]\n' + str(job.error_message).rstrip('\n') + '\n')
            automation_job.job_output = str(automation_job.job_output or '') + ''.join(output_chunks)
            update_fields.append('job_output')

        automation_job.save(update_fields=list(dict.fromkeys(update_fields)))
