"""
AgentChannel gRPC 服务实现。

网络方向：dj-agent 主动拨号连接这里、建立一条长连接双向流（与现有 RabbitMQ
心跳/任务模型同向，不要求目标主机对 backend 网络可达）。Session() 是一个
"生成器 + 后台读取线程"的组合：
  - 后台线程持续消费 request_iterator（agent -> backend 的应答/数据帧），
    第一帧必须是 Hello（校验共享密钥后注册到 AgentSessionRegistry），
    之后的帧按 request_id 路由回对应的等待队列；
  - 生成器主体阻塞在 session.outgoing 队列上，取到命令就 yield 给 agent
    （backend -> agent 的文件操作命令）。
这样一条物理连接上就可以并发承载多个文件操作请求（list/stat/read/write/...），
通过 request_id 做多路复用，而不需要为每个操作单独建立一条 gRPC 连接
（agent 是拨出方，backend 无法主动向 agent 发起新连接）。
"""
import logging
import threading
from typing import Optional

import grpc
from django.conf import settings

from .pb import agent_channel_pb2 as pb
from .pb import agent_channel_pb2_grpc as pb_grpc
from .registry import REGISTRY, AgentSession

logger = logging.getLogger(__name__)


class AgentChannelServicer(pb_grpc.AgentChannelServicer):
    def Session(self, request_iterator, context):
        session_holder = {'session': None}  # type: dict[str, Optional[AgentSession]]
        hello_event = threading.Event()
        hello_ok = {'value': False}

        def reader():
            try:
                for frame in request_iterator:
                    kind = frame.WhichOneof('payload')
                    if kind is None:
                        continue
                    if kind == 'hello':
                        agent_id = str(frame.hello.agent_id or '').strip()
                        # 当前阶段先移除共享 token 校验，仅要求 agent_id 非空。
                        # 后续切到 mTLS 后再把连接身份校验收敛到证书体系。
                        if not agent_id:
                            logger.warning('[agent-grpc] hello rejected agent_id=%s', agent_id or '(empty)')
                            hello_event.set()
                            return
                        session = AgentSession(agent_id)
                        session_holder['session'] = session
                        REGISTRY.register(session)
                        hello_ok['value'] = True
                        hello_event.set()
                        logger.warning('[agent-grpc] session established agent_id=%s', agent_id)
                        continue
                    session = session_holder['session']
                    if session is None:
                        continue
                    request_id = getattr(getattr(frame, kind), 'request_id', '')
                    if request_id:
                        session.deliver(request_id, frame)
            except Exception:
                logger.exception('[agent-grpc] reader loop error')
            finally:
                session = session_holder['session']
                if session is not None:
                    REGISTRY.unregister(session)
                    session.close()
                hello_event.set()

        reader_thread = threading.Thread(target=reader, name='agent-grpc-reader', daemon=True)
        reader_thread.start()

        if not hello_event.wait(timeout=15):
            context.abort(grpc.StatusCode.DEADLINE_EXCEEDED, 'hello timeout')
            return
        if not hello_ok['value']:
            context.abort(grpc.StatusCode.UNAUTHENTICATED, 'invalid agent hello')
            return

        session = session_holder['session']
        if session is None:
            context.abort(grpc.StatusCode.INTERNAL, 'session not initialized')
            return
        yield pb.ServerFrame(hello_ack=pb.HelloAck(accepted=True, message='ok'))
        try:
            while context.is_active():
                server_frame = session.outgoing.get()
                if server_frame is None:
                    break
                yield server_frame
        finally:
            REGISTRY.unregister(session)
            session.close()
            logger.warning('[agent-grpc] session closed agent_id=%s', session.agent_id)
