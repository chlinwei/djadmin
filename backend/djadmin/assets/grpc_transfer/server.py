"""
在 Django(Daphne) 主进程内以后台方式启动 AgentChannel gRPC Server。

关键约束：webssh_host_mixin.py 的文件操作视图与本 gRPC Server 必须运行在
【同一个 Python 进程】里——AgentSessionRegistry 是纯内存结构（agent_id ->
存活的 Session 对象，内含 queue.Queue），没有走数据库/Redis 等跨进程共享
存储。如果 gRPC Server 跑在独立进程（类似 runagentconsumer 那样的常驻命令），
Web 请求进程完全看不到这些 Session，agent-mode 分流会一直查不到连接。
本项目 `manage.py runserver` 走 Daphne 单进程模型（见
automation/management/commands/runserver.py），因此选择在该进程启动时
一并拉起 gRPC Server 后台线程，而不是拆成独立管理命令。
"""
import logging
import os
import sys
import threading
import atexit
from concurrent import futures

import grpc
from django.conf import settings

from .pb import agent_channel_pb2_grpc as pb_grpc
from .servicer import AgentChannelServicer

logger = logging.getLogger(__name__)

_start_lock = threading.Lock()
_started = False
_server = None  # 持有 gRPC server 引用，防止被 GC 回收导致端口关闭


def _should_autostart():
    # 只在真正长驻服务进程里启动，避免 migrate/makemigrations/test/shell 等
    # 一次性管理命令也意外占用 gRPC 监听端口。
    argv = sys.argv
    if len(argv) < 2:
        return False
    if argv[1] != 'runserver':
        return False
    # runserver 开启 autoreload 时会有两个进程：reloader 父进程 + 真正处理请求的
    # 子进程（RUN_MAIN=true）。gRPC 注册表是进程内内存，必须和处理 WebSSH/自动化
    # 请求的进程是同一个；否则父进程绑定了端口、agent 连到父进程注册，而请求由
    # 子进程处理却查不到会话（表现为“未建立 gRPC 通道连接；已连接 agent：（无）”）。
    # --noreload：单进程，直接启动。
    if '--noreload' in argv:
        return True
    # 开启 autoreload：仅在服务子进程（RUN_MAIN=true）启动。
    return os.environ.get('RUN_MAIN') == 'true'


def start_grpc_server_in_background():
    global _started, _server
    with _start_lock:
        if _started:
            return
        if not _should_autostart():
            return
        _started = True

    host = str(getattr(settings, 'AGENT_GRPC_LISTEN_HOST', '0.0.0.0'))
    port = int(getattr(settings, 'AGENT_GRPC_LISTEN_PORT', 9100))
    max_workers = int(getattr(settings, 'AGENT_GRPC_MAX_WORKERS', 64))
    max_message_mb = int(getattr(settings, 'AGENT_GRPC_MAX_MESSAGE_MB', 32))
    max_message_bytes = max(max_message_mb, 4) * 1024 * 1024

    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=max_workers),
        options=[
            ('grpc.max_send_message_length', max_message_bytes),
            ('grpc.max_receive_message_length', max_message_bytes),
        ],
    )
    pb_grpc.add_AgentChannelServicer_to_server(AgentChannelServicer(), server)
    bind_address = f'{host}:{port}'
    server.add_insecure_port(bind_address)
    server.start()
    _server = server  # 防止 GC 回收
    # 进程退出（含 autoreload 重启子进程退出）时主动停掉 gRPC server，尽快释放监听端口
    # 与内部线程，避免旧子进程半关闭状态下继续持有端口/线程，导致新子进程 bind 失败，
    # 或残留请求在解释器 finalize 阶段触发 "cannot schedule new futures after interpreter shutdown"。
    atexit.register(stop_grpc_server)
    logger.warning('[agent-grpc] agent channel gRPC server listening on %s', bind_address)
    return server


def stop_grpc_server():
    global _server
    server = _server
    if server is None:
        return
    _server = None
    try:
        # grace=0：立即取消在途 RPC 并关闭端口；此处仅用于快速释放资源，不等待优雅收尾。
        server.stop(grace=0)
    except Exception:
        pass
