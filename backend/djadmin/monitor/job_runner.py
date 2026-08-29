"""监控安装/卸载的进程内执行池。

不能用 Celery：安装链路第一步是通过 agent gRPC 下发控制机公钥，而 gRPC 会话注册表
是 runserver 进程内的纯内存结构（见 assets/grpc_transfer/server.py 的进程约束），
worker 进程里查不到任何 agent 连接，任务必然全部失败。

所以用固定大小线程池：既脱离 HTTP 请求线程（避免 ASGI 连接超时被杀），
又给批量勾选加了并发上限，不会一次拉起几十个 SSH 会话把控制机打满。
"""
import logging
import threading
from concurrent.futures import ThreadPoolExecutor

from django.conf import settings
from django.db import close_old_connections

logger = logging.getLogger(__name__)

_executor = None
_executor_lock = threading.Lock()


def _resolve_max_workers():
    raw_value = getattr(settings, 'MONITOR_JOB_MAX_WORKERS', 8)
    try:
        max_workers = int(raw_value)
    except (TypeError, ValueError):
        max_workers = 8
    return max(1, max_workers)


def _get_executor():
    global _executor
    with _executor_lock:
        if _executor is None:
            _executor = ThreadPoolExecutor(
                max_workers=_resolve_max_workers(),
                thread_name_prefix='monitor-job',
            )
        return _executor


def submit_monitor_job(func, *args, **kwargs):
    """提交后台执行；失败只记日志，不影响已经返回的 HTTP 响应。"""
    def run():
        # 线程复用会带上别的请求留下的连接，执行前后都要清一次。
        close_old_connections()
        try:
            func(*args, **kwargs)
        except Exception:
            logger.exception('监控后台任务执行异常: func=%s args=%s', getattr(func, '__name__', func), args)
        finally:
            close_old_connections()

    _get_executor().submit(run)
