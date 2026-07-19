"""
dj-agent 消费者：从 RabbitMQ 消费心跳、任务结果等消息，更新数据库。
"""
import json
import logging
import pika
from django.core.management.base import BaseCommand
from django.conf import settings
from django.utils import timezone
from assets.models import Host, AgentJob, AgentJobEvent, HostSystem, HostHardware

logger = logging.getLogger(__name__)


class Command(BaseCommand):
    help = 'Consume dj-agent messages from RabbitMQ (heartbeats, job results, etc.)'

    def add_arguments(self, parser):
        parser.add_argument(
            '--loglevel',
            type=str,
            default='info',
            help='Log level (debug, info, warning, error)',
        )

    def handle(self, *args, **options):
        loglevel = options.get('loglevel', 'info').upper()
        logging.basicConfig(level=loglevel)

        rabbitmq_url = str(getattr(settings, 'RABBITMQ_URL', 'amqp://127.0.0.1:5672//') or '').strip()
        if not rabbitmq_url:
            raise RuntimeError('RABBITMQ_URL未配置')

        # 连接 RabbitMQ
        try:
            connection = pika.BlockingConnection(pika.URLParameters(rabbitmq_url))
            channel = connection.channel()
            
            # 声明报告队列（用于接收心跳和其他上报）
            reports_queue = getattr(settings, 'RABBITMQ_AGENT_REPORTS_QUEUE', 'agent.reports')
            channel.queue_declare(queue=reports_queue, durable=True, auto_delete=False)
            
            # 设置 QoS
            channel.basic_qos(prefetch_count=1)
            
            # 定义消费回调
            def on_message(ch, method, properties, body):
                try:
                    payload = json.loads(body.decode('utf-8'))
                    message_type = str(payload.get('type') or '').strip().lower()
                    
                    if message_type == 'heartbeat':
                        self._handle_heartbeat(payload)
                    elif message_type == 'agent_status':
                        self._handle_agent_status(payload)
                    elif message_type == 'host_snapshot':
                        self._handle_host_snapshot(payload)
                    elif message_type == 'job_result':
                        self._handle_job_result(payload)
                    elif message_type == 'job_event':
                        self._handle_job_event(payload)
                    else:
                        logger.warning(f"Unknown message type: {message_type}")
                    
                    # 确认消息已处理
                    ch.basic_ack(delivery_tag=method.delivery_tag)
                except Exception as e:
                    logger.error(f"Error processing message: {e}", exc_info=True)
                    # 拒绝消息，放回队列
                    ch.basic_nack(delivery_tag=method.delivery_tag, requeue=True)
            
            # 开始消费
            channel.basic_consume(
                queue=reports_queue,
                on_message_callback=on_message,
            )
            
            logger.info(f"Started consuming from {reports_queue}")
            channel.start_consuming()
        except Exception as e:
            logger.error(f"Consumer error: {e}", exc_info=True)
            raise
    
    def _handle_heartbeat(self, payload):
        """处理 agent 周期心跳消息（仅用于保活更新时间）。"""
        agent_id = str(payload.get('agent_id') or '').strip()
        if not agent_id:
            logger.warning('Heartbeat message missing agent_id')
            return
        
        # 查找或创建主机
        host = Host.objects.filter(instance_name=agent_id).first()
        if not host:
            logger.warning(f'Heartbeat from unknown agent: {agent_id}')
            return
        
        # 更新 agent 在线状态（记录最后心跳时间）
        now = timezone.now()
        host.agent_online = True
        host.agent_online_time = now
        host.save(update_fields=['agent_online', 'agent_online_time', 'update_time'])
        
        logger.debug(f'Updated heartbeat for agent: {agent_id}')

    def _handle_agent_status(self, payload):
        """处理 agent 上下线状态事件（online/offline）。"""
        agent_id = str(payload.get('agent_id') or '').strip()
        status = str(payload.get('status') or '').strip().lower()
        if not agent_id:
            logger.warning('Agent status message missing agent_id')
            return

        host = Host.objects.filter(instance_name=agent_id).first()
        if not host:
            logger.warning(f'Agent status from unknown agent: {agent_id}')
            return

        if status == 'online':
            now = timezone.now()
            host.agent_online = True
            host.agent_online_time = now
            host.save(update_fields=['agent_online', 'agent_online_time', 'update_time'])
            logger.info(f'Marked agent online by status event: {agent_id}')
            return

        if status == 'offline':
            host.agent_online = False
            host.save(update_fields=['agent_online', 'update_time'])
            logger.info(f'Marked agent offline by status event: {agent_id}')
            return

        logger.warning(f'Unknown agent status value: {status!r}, agent_id={agent_id}')
    
    def _handle_job_result(self, payload):
        """处理 agent 任务结果消息。"""
        job_id = str(payload.get('job_id') or '').strip()
        agent_id = str(payload.get('agent_id') or '').strip()
        status = str(payload.get('status') or '').strip().lower()
        
        if not job_id:
            logger.warning('Job result message missing job_id')
            return
        
        # 查找任务
        job = AgentJob.objects.filter(job_id=job_id).first()
        if not job:
            logger.warning(f'Job result for unknown job: {job_id}')
            return
        
        # 更新任务状态
        try:
            if status in {'success', 'failed', 'canceled', 'timeout'}:
                raw_result_data = payload.get('result_data')
                # RabbitMQ 上报中 result_data 可能为 null，数据库列不允许 NULL。
                normalized_result_data = raw_result_data if isinstance(raw_result_data, dict) else {}
                job.status = status
                job.result_data = normalized_result_data
                job.error_message = str(payload.get('error_message') or '')
                job.stdout = str(payload.get('stdout') or '')
                job.stderr = str(payload.get('stderr') or '')
                job.exit_code = int(payload.get('exit_code', -1))
                job.finished_at = timezone.now()
                job.save(update_fields=[
                    'status', 'result_data', 'error_message', 
                    'stdout', 'stderr', 'exit_code', 'finished_at', 'update_time'
                ])
                self._sync_monitor_target_install_status(job, status)
                logger.info(f'Updated job {job_id} status to {status}')
        except Exception as e:
            logger.error(f'Error updating job {job_id}: {e}', exc_info=True)

    def _sync_monitor_target_install_status(self, job, status):
        action = str(getattr(job, 'action', '') or '').strip().lower()
        if action not in {'install_node_exporter', 'uninstall_node_exporter'}:
            return

        host = getattr(job, 'host', None)
        if host is None:
            return

        from monitor.models import MonitorTarget

        target = MonitorTarget.objects.filter(
            host=host,
            exporter_type=MonitorTarget.ExporterType.NODE_EXPORTER,
        ).first()
        if target is None:
            return

        error_message = str(getattr(job, 'error_message', '') or '').strip()
        stdout = str(getattr(job, 'stdout', '') or '').strip()
        stderr = str(getattr(job, 'stderr', '') or '').strip()

        def _clip(text):
            return text if len(text) <= 300 else text[:297] + '...'

        if status == 'success':
            target.install_status = MonitorTarget.InstallStatus.SUCCESS
            if action == 'install_node_exporter':
                target.managed_enabled = True
                target.install_message = _clip(stdout or 'node_exporter 安装成功')
            else:
                target.managed_enabled = False
                target.install_message = _clip(stdout or 'node_exporter 卸载成功')
        else:
            target.install_status = MonitorTarget.InstallStatus.FAILED
            if action == 'install_node_exporter':
                target.install_message = _clip(error_message or stderr or 'node_exporter 安装失败')
            else:
                target.install_message = _clip(error_message or stderr or 'node_exporter 卸载失败')

        target.save(update_fields=['install_status', 'managed_enabled', 'install_message', 'update_time'])
    
    def _handle_host_snapshot(self, payload):
        """处理 agent 主机快照消息，解析 result_data 写入 HostSystem/HostHardware。"""
        agent_id = str(payload.get('agent_id') or '').strip()
        if not agent_id:
            logger.warning('Host snapshot missing agent_id')
            return

        host = Host.objects.filter(instance_name=agent_id).first()
        if not host:
            logger.warning(f'Host snapshot from unknown agent: {agent_id}')
            return

        status = str(payload.get('status') or '').strip()
        result_data = payload.get('result_data') or {}
        now = timezone.now()

        try:
            # 仅成功时更新采集时间，失败不覆盖上次成功的时间戳
            update_fields = ['collect_status', 'collect_message', 'update_time']
            host.collect_status = 'success' if status == 'success' else 'failed'
            host.collect_message = str(payload.get('error') or '')
            if status == 'success':
                host.collect_time = now
                update_fields.append('collect_time')
                # 存储原始快照供调试
                if isinstance(result_data, dict) and result_data:
                    host.host_snapshot = result_data
                    update_fields.append('host_snapshot')
            host.save(update_fields=update_fields)

            if status != 'success' or not isinstance(result_data, dict):
                logger.warning(f'Host snapshot failed for {agent_id}: {payload.get("error")}')
                return

            # 更新 HostSystem（os_type/hostname/agent_version/collector_source）
            # get_host_info 返回字段：os, hostname, agent_version, arch, cpu_count 等
            HostSystem.objects.update_or_create(
                host=host,
                defaults={
                    'os_type': str(result_data.get('os') or '').strip() or None,
                    'hostname': str(result_data.get('hostname') or '').strip() or None,
                    'agent_version': str(result_data.get('agent_version') or '').strip() or None,
                    'collector_source': 'agent',  # 来自 dj-agent 上报，固定为 agent
                    'collected_at': now,
                },
            )

            # 更新 HostHardware（cpu_cores/architecture）
            cpu_count = result_data.get('cpu_count')
            arch = str(result_data.get('arch') or '').strip() or None
            hw_defaults = {'collected_at': now}
            if cpu_count is not None:
                hw_defaults['cpu_cores'] = int(cpu_count)
            if arch:
                hw_defaults['architecture'] = arch
            HostHardware.objects.update_or_create(host=host, defaults=hw_defaults)

            logger.info(f'Updated host snapshot for agent: {agent_id}, os={result_data.get("os")}, hostname={result_data.get("hostname")}')
        except Exception as e:
            logger.error(f'Error processing host snapshot for {agent_id}: {e}', exc_info=True)
    
    def _handle_job_event(self, payload):
        """处理 agent 任务事件消息（如状态变化）。"""
        job_id = str(payload.get('job_id') or '').strip()
        event_type = str(payload.get('event_type') or '').strip()
        
        if not job_id or not event_type:
            logger.warning('Job event missing job_id or event_type')
            return
        
        # 可根据需要处理任务事件
        logger.debug(f'Received job event - job_id: {job_id}, event_type: {event_type}')
