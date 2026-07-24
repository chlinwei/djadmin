"""
Agent gRPC 文件传输会话注册表。

每个 dj-agent 通过 AgentChannel.Session 建立一条长连接（agent 主动拨号），
在这条连接上可以并发发起多个文件操作（list/stat/read/write/rename/delete），
通过 request_id 做多路复用。本模块维护 agent_id -> AgentSession 的映射，
供 Django 视图（webssh_host_mixin.py）在 host.agent_online 为真时查找并调用。
"""
import queue
import threading


class AgentSession:
    """单个 agent 的 gRPC Session：一条 outgoing 队列（backend->agent 的命令），
    以及一组按 request_id 索引的 incoming 队列（agent->backend 的应答/数据块）。
    """

    def __init__(self, agent_id):
        self.agent_id = agent_id
        self.outgoing = queue.Queue()
        self._lock = threading.Lock()
        self._pending = {}
        self._closed = False

    def new_request(self, request_id):
        q = queue.Queue()
        with self._lock:
            self._pending[request_id] = q
        return q

    def drop_request(self, request_id):
        with self._lock:
            self._pending.pop(request_id, None)

    def deliver(self, request_id, frame):
        with self._lock:
            q = self._pending.get(request_id)
        if q is not None:
            q.put(frame)

    def send(self, server_frame):
        if self._closed:
            raise RuntimeError('agent session 已关闭')
        self.outgoing.put(server_frame)

    def close(self):
        # 连接断开/被顶替时：唤醒所有阻塞等待应答的调用方（用 None 表示"连接已断开"），
        # 避免视图线程无限期挂起等待一个永远不会到来的响应。
        with self._lock:
            self._closed = True
            pending = list(self._pending.values())
            self._pending.clear()
        for q in pending:
            q.put(None)
        self.outgoing.put(None)  # 唤醒 Session() 生成器里阻塞在 outgoing.get() 的调用


class AgentSessionRegistry:
    def __init__(self):
        self._lock = threading.RLock()
        self._sessions = {}

    def register(self, session):
        with self._lock:
            old = self._sessions.get(session.agent_id)
            self._sessions[session.agent_id] = session
        if old is not None and old is not session:
            # 同一 agent_id 重新拨入：视为旧连接已失效（重启/重连），主动关闭旧会话，
            # 防止旧连接残留在注册表里但实际已不可用，导致后续请求超时才能发现。
            old.close()

    def unregister(self, session):
        with self._lock:
            if self._sessions.get(session.agent_id) is session:
                self._sessions.pop(session.agent_id, None)

    def get(self, agent_id):
        with self._lock:
            return self._sessions.get(agent_id)

    def is_connected(self, agent_id):
        return self.get(agent_id) is not None

    def connected_agent_ids(self):
        # 返回当前已建立 gRPC 会话的 agent_id 快照，主要用于排障（例如任务下发时
        # 目标 agent 不在线，可据此判断是完全没连、还是以别的 agent_id 连入）。
        with self._lock:
            return sorted(self._sessions.keys())


# 进程级单例：Django gRPC server 进程（runagentgrpc 管理命令）与 Web 请求进程
# 需要能访问同一份注册表——本项目里两者需运行在同一个 Python 进程/解释器内
# （见 runagentgrpc.py 的实现说明）。
REGISTRY = AgentSessionRegistry()
