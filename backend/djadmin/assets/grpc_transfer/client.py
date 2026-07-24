"""
供 webssh_host_mixin.py / 自动化任务执行调用的 agent gRPC 通道客户端封装。

每个方法都是同步阻塞调用（Django 视图是同步的）：把命令放进目标 agent 的
outgoing 队列，然后在为该 request_id 新建的队列上 blocking get() 等待 agent
经由 Session() 长连接回传的应答/数据帧。所有方法在 agent 未连接、超时、
或对端返回 error 字段时统一抛出 AgentGrpcTransferError，调用方按同一套
异常处理逻辑与现有 SSH 直连路径对齐（都是 except Exception 转 400 错误）。
"""
import json
import queue
import uuid

from django.conf import settings

from .pb import agent_channel_pb2 as pb
from .registry import REGISTRY


class AgentGrpcTransferError(Exception):
    pass


def is_agent_grpc_connected(agent_id):
    return REGISTRY.is_connected(agent_id)


class AgentWriteSession:
    """一次上传的有状态句柄：write_open() 建立后复用同一个 request_id 持续
    write_chunk()，最后 close() 触发 agent 侧原子 rename（或 abort 清理临时文件）。
    """

    def __init__(self, client, request_id, response_queue):
        self._client = client
        self.request_id = request_id
        self._queue = response_queue
        self._closed = False

    def write_chunk(self, data):
        self._client.session.send(pb.ServerFrame(
            write_chunk_request=pb.WriteChunkRequest(request_id=self.request_id, data=data),
        ))
        frame = self._client._recv(self._queue)
        ack = frame.write_chunk_ack
        if ack.error:
            raise AgentGrpcTransferError(ack.error)
        return ack.bytes_written

    def close(self, abort=False):
        if self._closed:
            return None
        self._closed = True
        try:
            self._client.session.send(pb.ServerFrame(
                write_close_request=pb.WriteCloseRequest(request_id=self.request_id, abort=abort),
            ))
            frame = self._client._recv(self._queue)
        finally:
            self._client.session.drop_request(self.request_id)
        resp = frame.write_close_response
        if resp.error:
            raise AgentGrpcTransferError(resp.error)
        return resp


class AgentTerminalSession:
    """WebSSH 终端会话句柄：open_shell() 建立后复用同一个 request_id 持续收发，
    数据面（stdin/resize）走 fire-and-forget（`session.send` 只是把帧放进 agent 的
    outgoing 队列，不等待 ack），控制面（recv）由调用方（consumers.py）在独立线程里
    阻塞轮询，通过 asyncio 线程桥接把输出转发回 WebSocket。
    """

    def __init__(self, client, request_id, response_queue, effective_user=''):
        self._client = client
        self.request_id = request_id
        self._queue = response_queue
        self._closed = False
        self.effective_user = str(effective_user or '')

    def send_stdin(self, data):
        if isinstance(data, str):
            data = data.encode('utf-8', errors='ignore')
        self._client.session.send(pb.ServerFrame(
            terminal_data_request=pb.TerminalDataRequest(request_id=self.request_id, data=data),
        ))

    def resize(self, cols, rows):
        self._client.session.send(pb.ServerFrame(
            terminal_resize_request=pb.TerminalResizeRequest(request_id=self.request_id, cols=cols, rows=rows),
        ))

    def recv(self, timeout=None):
        """阻塞等待下一帧（terminal_data_response 或 terminal_exit_response）。
        agent 连接断开时返回 None（registry 在 session.close() 时会向所有等待队列 put(None)）。
        """
        if timeout is None:
            return self._queue.get()
        try:
            return self._queue.get(timeout=timeout)
        except queue.Empty:
            return 'timeout'

    def close(self):
        if self._closed:
            return
        self._closed = True
        try:
            self._client.session.send(pb.ServerFrame(
                terminal_close_request=pb.TerminalCloseRequest(request_id=self.request_id),
            ))
        except Exception:
            pass
        self._client.session.drop_request(self.request_id)


class AgentChannelClient:
    def __init__(self, agent_id, timeout=None):
        self.agent_id = agent_id
        self.timeout = float(timeout or getattr(settings, 'AGENT_GRPC_REQUEST_TIMEOUT_SECONDS', 20))
        session = REGISTRY.get(agent_id)
        if session is None:
            # 附带当前已连接的 agent_id 列表，便于区分「agent 完全没建立 gRPC 通道」
            # 与「agent 以其它 agent_id 连入（instance_name 不匹配）」两类问题。
            connected = REGISTRY.connected_agent_ids()
            connected_hint = '、'.join(connected) if connected else '（无）'
            raise AgentGrpcTransferError(
                f'agent {agent_id} 未建立 gRPC 通道连接；当前已连接 agent：{connected_hint}'
            )
        self.session = session

    @staticmethod
    def _new_request_id():
        return uuid.uuid4().hex

    def _recv(self, response_queue):
        try:
            frame = response_queue.get(timeout=self.timeout)
        except queue.Empty as exc:
            raise AgentGrpcTransferError('等待 agent 响应超时') from exc
        if frame is None:
            raise AgentGrpcTransferError('agent 连接已断开')
        return frame

    def _call_unary(self, server_frame, request_id, response_field):
        response_queue = self.session.new_request(request_id)
        try:
            self.session.send(server_frame)
            frame = self._recv(response_queue)
        finally:
            self.session.drop_request(request_id)
        resp = getattr(frame, response_field)
        if resp.error:
            raise AgentGrpcTransferError(resp.error)
        return resp

    def list_dir(self, path):
        request_id = self._new_request_id()
        frame = pb.ServerFrame(list_request=pb.ListRequest(request_id=request_id, path=path))
        return self._call_unary(frame, request_id, 'list_response')

    def stat(self, path):
        request_id = self._new_request_id()
        frame = pb.ServerFrame(stat_request=pb.StatRequest(request_id=request_id, path=path))
        return self._call_unary(frame, request_id, 'stat_response')

    def read_stream(self, path, offset=0, length=0):
        """生成器：按到达顺序 yield ReadChunk（含 data/eof/file_size/error）。"""
        request_id = self._new_request_id()
        response_queue = self.session.new_request(request_id)
        try:
            self.session.send(pb.ServerFrame(read_request=pb.ReadRequest(
                request_id=request_id, path=path, offset=offset, length=length,
            )))
            while True:
                frame = self._recv(response_queue)
                chunk = frame.read_chunk
                if chunk.error:
                    raise AgentGrpcTransferError(chunk.error)
                yield chunk
                if chunk.eof:
                    break
        finally:
            self.session.drop_request(request_id)

    def open_write(self, dir_path, file_name):
        request_id = self._new_request_id()
        response_queue = self.session.new_request(request_id)
        try:
            self.session.send(pb.ServerFrame(write_open_request=pb.WriteOpenRequest(
                request_id=request_id, dir_path=dir_path, file_name=file_name,
            )))
            frame = self._recv(response_queue)
        except Exception:
            self.session.drop_request(request_id)
            raise
        resp = frame.write_open_response
        if resp.error:
            self.session.drop_request(request_id)
            raise AgentGrpcTransferError(resp.error)
        return AgentWriteSession(self, request_id, response_queue)

    def rename(self, path, new_name):
        request_id = self._new_request_id()
        frame = pb.ServerFrame(rename_request=pb.RenameRequest(request_id=request_id, path=path, new_name=new_name))
        return self._call_unary(frame, request_id, 'rename_response')

    def delete(self, path, recursive=False):
        request_id = self._new_request_id()
        frame = pb.ServerFrame(delete_request=pb.DeleteRequest(request_id=request_id, path=path, recursive=recursive))
        return self._call_unary(frame, request_id, 'delete_response')

    def mkdir(self, path, name):
        request_id = self._new_request_id()
        frame = pb.ServerFrame(mkdir_request=pb.MkdirRequest(request_id=request_id, path=path, name=name))
        return self._call_unary(frame, request_id, 'mkdir_response')

    def create_file(self, path, name):
        request_id = self._new_request_id()
        frame = pb.ServerFrame(create_file_request=pb.CreateFileRequest(request_id=request_id, path=path, name=name))
        return self._call_unary(frame, request_id, 'create_file_response')

    def execute_automation(self, job_id, params, timeout_seconds, task_type='custom', action='run_automation_task'):
        """在 agent 长连接上下发一次自动化任务（playbook/shell）并阻塞等待执行结果。

        与文件操作不同，自动化任务可能运行很久（最长数小时），因此这里不用
        self.timeout（默认 20s 面向文件操作），而是按传入的 timeout_seconds 等待，
        再加一段网络/回传缓冲；agent 侧超时会自行返回 status=timeout 的响应。
        params 里含 env_vars/extra_vars 等嵌套结构，用 JSON 字符串无损透传
        （见 proto AutomationExecuteRequest.params_json）。返回解码后的结果 dict。
        """
        request_id = self._new_request_id()
        wait_seconds = max(int(timeout_seconds or 0), 1) + 30
        response_queue = self.session.new_request(request_id)
        try:
            self.session.send(pb.ServerFrame(automation_execute_request=pb.AutomationExecuteRequest(
                request_id=request_id,
                job_id=str(job_id or ''),
                type=str(task_type or 'custom'),
                action=str(action or 'run_automation_task'),
                params_json=json.dumps(params or {}, ensure_ascii=False),
                timeout_seconds=int(timeout_seconds or 0),
            )))
            try:
                frame = response_queue.get(timeout=wait_seconds)
            except queue.Empty as exc:
                raise AgentGrpcTransferError('等待 agent 自动化任务响应超时') from exc
            if frame is None:
                raise AgentGrpcTransferError('agent 连接已断开')
        finally:
            self.session.drop_request(request_id)
        resp = frame.automation_execute_response
        if resp.error_message and not resp.status:
            # 无 status 且带 error_message 视为传输/协议层失败；业务失败仍带 status。
            raise AgentGrpcTransferError(resp.error_message)
        result_data = {}
        if resp.result_data_json:
            try:
                decoded = json.loads(resp.result_data_json)
                if isinstance(decoded, dict):
                    result_data = decoded
            except (ValueError, TypeError):
                result_data = {}
        return {
            'job_id': resp.job_id,
            'status': resp.status,
            'exit_code': resp.exit_code,
            'stdout': resp.stdout,
            'stderr': resp.stderr,
            'result_data': result_data,
            'error_message': resp.error_message,
            'cost_ms': resp.cost_ms,
        }

    def open_shell(self, cols=120, rows=32, target_user=''):
        """建立一个 WebSSH 终端会话：发送 terminal_open 并阻塞等待 agent 的 open ack，
        成功后返回 AgentTerminalSession（后续 stdin/resize/输出都通过它进行）。
        """
        request_id = self._new_request_id()
        response_queue = self.session.new_request(request_id)
        try:
            self.session.send(pb.ServerFrame(terminal_open_request=pb.TerminalOpenRequest(
                request_id=request_id,
                cols=cols,
                rows=rows,
                target_user=str(target_user or '').strip(),
            )))
            frame = self._recv(response_queue)
        except Exception:
            self.session.drop_request(request_id)
            raise
        resp = frame.terminal_open_response
        if resp.error:
            self.session.drop_request(request_id)
            raise AgentGrpcTransferError(resp.error)
        return AgentTerminalSession(
            self,
            request_id,
            response_queue,
            effective_user=getattr(resp, 'effective_user', ''),
        )
