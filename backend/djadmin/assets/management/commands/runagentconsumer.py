"""
dj-agent 消费者：从 RabbitMQ 消费心跳、任务结果等消息，更新数据库。
"""
import json
import logging
import pika
from django.core.management.base import BaseCommand
from django.conf import settings
from django.utils import timezone
from assets.models import Host

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
        host = Host.objects.filter(agent_id=agent_id).first()
        if not host:
            logger.warning(f'Heartbeat from unknown agent: {agent_id}')
            return
        
        # RabbitMQ 心跳只记录存活时间；是否可执行任务由 Web/gRPC 进程内 Session Registry 决定。
        now = timezone.now()
        host.agent_online_time = now
        host.save(update_fields=['agent_online_time', 'update_time'])
        
        logger.debug(f'Updated heartbeat for agent: {agent_id}')

    def _handle_agent_status(self, payload):
        """处理 agent 上下线状态事件（online/offline）。"""
        agent_id = str(payload.get('agent_id') or '').strip()
        status = str(payload.get('status') or '').strip().lower()
        if not agent_id:
            logger.warning('Agent status message missing agent_id')
            return

        host = Host.objects.filter(agent_id=agent_id).first()
        if not host:
            logger.warning(f'Agent status from unknown agent: {agent_id}')
            return

        if status == 'online':
            now = timezone.now()
            host.agent_online_time = now
            host.save(update_fields=['agent_online_time', 'update_time'])
            logger.info(f'Recorded agent status event: {agent_id}')
            return

        if status == 'offline':
            logger.info(f'Recorded agent offline event: {agent_id}')
            logger.info(f'Marked agent offline by status event: {agent_id}')
            return

        logger.warning(f'Unknown agent status value: {status!r}, agent_id={agent_id}')
    
    def _sync_monitor_target_install_status(self, job, status):
        # 兼容保留：安装/卸载执行与状态回写已在 dispatch 链路内完成，
        # runagentconsumer 不再参与该链路，避免双写/分叉。
        return

    
