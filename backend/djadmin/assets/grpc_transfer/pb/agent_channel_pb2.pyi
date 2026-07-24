from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class ServerFrame(_message.Message):
    __slots__ = ("hello_ack", "list_request", "stat_request", "read_request", "write_open_request", "write_chunk_request", "write_close_request", "rename_request", "delete_request", "mkdir_request", "create_file_request", "terminal_open_request", "terminal_data_request", "terminal_resize_request", "terminal_close_request", "automation_execute_request")
    HELLO_ACK_FIELD_NUMBER: _ClassVar[int]
    LIST_REQUEST_FIELD_NUMBER: _ClassVar[int]
    STAT_REQUEST_FIELD_NUMBER: _ClassVar[int]
    READ_REQUEST_FIELD_NUMBER: _ClassVar[int]
    WRITE_OPEN_REQUEST_FIELD_NUMBER: _ClassVar[int]
    WRITE_CHUNK_REQUEST_FIELD_NUMBER: _ClassVar[int]
    WRITE_CLOSE_REQUEST_FIELD_NUMBER: _ClassVar[int]
    RENAME_REQUEST_FIELD_NUMBER: _ClassVar[int]
    DELETE_REQUEST_FIELD_NUMBER: _ClassVar[int]
    MKDIR_REQUEST_FIELD_NUMBER: _ClassVar[int]
    CREATE_FILE_REQUEST_FIELD_NUMBER: _ClassVar[int]
    TERMINAL_OPEN_REQUEST_FIELD_NUMBER: _ClassVar[int]
    TERMINAL_DATA_REQUEST_FIELD_NUMBER: _ClassVar[int]
    TERMINAL_RESIZE_REQUEST_FIELD_NUMBER: _ClassVar[int]
    TERMINAL_CLOSE_REQUEST_FIELD_NUMBER: _ClassVar[int]
    AUTOMATION_EXECUTE_REQUEST_FIELD_NUMBER: _ClassVar[int]
    hello_ack: HelloAck
    list_request: ListRequest
    stat_request: StatRequest
    read_request: ReadRequest
    write_open_request: WriteOpenRequest
    write_chunk_request: WriteChunkRequest
    write_close_request: WriteCloseRequest
    rename_request: RenameRequest
    delete_request: DeleteRequest
    mkdir_request: MkdirRequest
    create_file_request: CreateFileRequest
    terminal_open_request: TerminalOpenRequest
    terminal_data_request: TerminalDataRequest
    terminal_resize_request: TerminalResizeRequest
    terminal_close_request: TerminalCloseRequest
    automation_execute_request: AutomationExecuteRequest
    def __init__(self, hello_ack: _Optional[_Union[HelloAck, _Mapping]] = ..., list_request: _Optional[_Union[ListRequest, _Mapping]] = ..., stat_request: _Optional[_Union[StatRequest, _Mapping]] = ..., read_request: _Optional[_Union[ReadRequest, _Mapping]] = ..., write_open_request: _Optional[_Union[WriteOpenRequest, _Mapping]] = ..., write_chunk_request: _Optional[_Union[WriteChunkRequest, _Mapping]] = ..., write_close_request: _Optional[_Union[WriteCloseRequest, _Mapping]] = ..., rename_request: _Optional[_Union[RenameRequest, _Mapping]] = ..., delete_request: _Optional[_Union[DeleteRequest, _Mapping]] = ..., mkdir_request: _Optional[_Union[MkdirRequest, _Mapping]] = ..., create_file_request: _Optional[_Union[CreateFileRequest, _Mapping]] = ..., terminal_open_request: _Optional[_Union[TerminalOpenRequest, _Mapping]] = ..., terminal_data_request: _Optional[_Union[TerminalDataRequest, _Mapping]] = ..., terminal_resize_request: _Optional[_Union[TerminalResizeRequest, _Mapping]] = ..., terminal_close_request: _Optional[_Union[TerminalCloseRequest, _Mapping]] = ..., automation_execute_request: _Optional[_Union[AutomationExecuteRequest, _Mapping]] = ...) -> None: ...

class AgentFrame(_message.Message):
    __slots__ = ("hello", "list_response", "stat_response", "read_chunk", "write_open_response", "write_chunk_ack", "write_close_response", "rename_response", "delete_response", "mkdir_response", "create_file_response", "terminal_open_response", "terminal_data_response", "terminal_exit_response", "automation_execute_response")
    HELLO_FIELD_NUMBER: _ClassVar[int]
    LIST_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    STAT_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    READ_CHUNK_FIELD_NUMBER: _ClassVar[int]
    WRITE_OPEN_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    WRITE_CHUNK_ACK_FIELD_NUMBER: _ClassVar[int]
    WRITE_CLOSE_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    RENAME_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    DELETE_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    MKDIR_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    CREATE_FILE_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    TERMINAL_OPEN_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    TERMINAL_DATA_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    TERMINAL_EXIT_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    AUTOMATION_EXECUTE_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    hello: Hello
    list_response: ListResponse
    stat_response: StatResponse
    read_chunk: ReadChunk
    write_open_response: WriteOpenResponse
    write_chunk_ack: WriteChunkAck
    write_close_response: WriteCloseResponse
    rename_response: RenameResponse
    delete_response: DeleteResponse
    mkdir_response: MkdirResponse
    create_file_response: CreateFileResponse
    terminal_open_response: TerminalOpenResponse
    terminal_data_response: TerminalDataResponse
    terminal_exit_response: TerminalExitResponse
    automation_execute_response: AutomationExecuteResponse
    def __init__(self, hello: _Optional[_Union[Hello, _Mapping]] = ..., list_response: _Optional[_Union[ListResponse, _Mapping]] = ..., stat_response: _Optional[_Union[StatResponse, _Mapping]] = ..., read_chunk: _Optional[_Union[ReadChunk, _Mapping]] = ..., write_open_response: _Optional[_Union[WriteOpenResponse, _Mapping]] = ..., write_chunk_ack: _Optional[_Union[WriteChunkAck, _Mapping]] = ..., write_close_response: _Optional[_Union[WriteCloseResponse, _Mapping]] = ..., rename_response: _Optional[_Union[RenameResponse, _Mapping]] = ..., delete_response: _Optional[_Union[DeleteResponse, _Mapping]] = ..., mkdir_response: _Optional[_Union[MkdirResponse, _Mapping]] = ..., create_file_response: _Optional[_Union[CreateFileResponse, _Mapping]] = ..., terminal_open_response: _Optional[_Union[TerminalOpenResponse, _Mapping]] = ..., terminal_data_response: _Optional[_Union[TerminalDataResponse, _Mapping]] = ..., terminal_exit_response: _Optional[_Union[TerminalExitResponse, _Mapping]] = ..., automation_execute_response: _Optional[_Union[AutomationExecuteResponse, _Mapping]] = ...) -> None: ...

class Hello(_message.Message):
    __slots__ = ("agent_id", "token")
    AGENT_ID_FIELD_NUMBER: _ClassVar[int]
    TOKEN_FIELD_NUMBER: _ClassVar[int]
    agent_id: str
    token: str
    def __init__(self, agent_id: _Optional[str] = ..., token: _Optional[str] = ...) -> None: ...

class HelloAck(_message.Message):
    __slots__ = ("accepted", "message")
    ACCEPTED_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    accepted: bool
    message: str
    def __init__(self, accepted: _Optional[bool] = ..., message: _Optional[str] = ...) -> None: ...

class FileEntry(_message.Message):
    __slots__ = ("name", "size", "mode", "is_dir", "mtime")
    NAME_FIELD_NUMBER: _ClassVar[int]
    SIZE_FIELD_NUMBER: _ClassVar[int]
    MODE_FIELD_NUMBER: _ClassVar[int]
    IS_DIR_FIELD_NUMBER: _ClassVar[int]
    MTIME_FIELD_NUMBER: _ClassVar[int]
    name: str
    size: int
    mode: int
    is_dir: bool
    mtime: int
    def __init__(self, name: _Optional[str] = ..., size: _Optional[int] = ..., mode: _Optional[int] = ..., is_dir: _Optional[bool] = ..., mtime: _Optional[int] = ...) -> None: ...

class ListRequest(_message.Message):
    __slots__ = ("request_id", "path")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    path: str
    def __init__(self, request_id: _Optional[str] = ..., path: _Optional[str] = ...) -> None: ...

class ListResponse(_message.Message):
    __slots__ = ("request_id", "current_path", "entries", "error")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    CURRENT_PATH_FIELD_NUMBER: _ClassVar[int]
    ENTRIES_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    current_path: str
    entries: _containers.RepeatedCompositeFieldContainer[FileEntry]
    error: str
    def __init__(self, request_id: _Optional[str] = ..., current_path: _Optional[str] = ..., entries: _Optional[_Iterable[_Union[FileEntry, _Mapping]]] = ..., error: _Optional[str] = ...) -> None: ...

class StatRequest(_message.Message):
    __slots__ = ("request_id", "path")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    path: str
    def __init__(self, request_id: _Optional[str] = ..., path: _Optional[str] = ...) -> None: ...

class StatResponse(_message.Message):
    __slots__ = ("request_id", "normalized_path", "size", "mode", "is_dir", "mtime", "error")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    NORMALIZED_PATH_FIELD_NUMBER: _ClassVar[int]
    SIZE_FIELD_NUMBER: _ClassVar[int]
    MODE_FIELD_NUMBER: _ClassVar[int]
    IS_DIR_FIELD_NUMBER: _ClassVar[int]
    MTIME_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    normalized_path: str
    size: int
    mode: int
    is_dir: bool
    mtime: int
    error: str
    def __init__(self, request_id: _Optional[str] = ..., normalized_path: _Optional[str] = ..., size: _Optional[int] = ..., mode: _Optional[int] = ..., is_dir: _Optional[bool] = ..., mtime: _Optional[int] = ..., error: _Optional[str] = ...) -> None: ...

class ReadRequest(_message.Message):
    __slots__ = ("request_id", "path", "offset", "length")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    OFFSET_FIELD_NUMBER: _ClassVar[int]
    LENGTH_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    path: str
    offset: int
    length: int
    def __init__(self, request_id: _Optional[str] = ..., path: _Optional[str] = ..., offset: _Optional[int] = ..., length: _Optional[int] = ...) -> None: ...

class ReadChunk(_message.Message):
    __slots__ = ("request_id", "data", "eof", "file_size", "error")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    EOF_FIELD_NUMBER: _ClassVar[int]
    FILE_SIZE_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    data: bytes
    eof: bool
    file_size: int
    error: str
    def __init__(self, request_id: _Optional[str] = ..., data: _Optional[bytes] = ..., eof: _Optional[bool] = ..., file_size: _Optional[int] = ..., error: _Optional[str] = ...) -> None: ...

class WriteOpenRequest(_message.Message):
    __slots__ = ("request_id", "dir_path", "file_name")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    DIR_PATH_FIELD_NUMBER: _ClassVar[int]
    FILE_NAME_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    dir_path: str
    file_name: str
    def __init__(self, request_id: _Optional[str] = ..., dir_path: _Optional[str] = ..., file_name: _Optional[str] = ...) -> None: ...

class WriteOpenResponse(_message.Message):
    __slots__ = ("request_id", "error")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    error: str
    def __init__(self, request_id: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class WriteChunkRequest(_message.Message):
    __slots__ = ("request_id", "data")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    data: bytes
    def __init__(self, request_id: _Optional[str] = ..., data: _Optional[bytes] = ...) -> None: ...

class WriteChunkAck(_message.Message):
    __slots__ = ("request_id", "bytes_written", "error")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    BYTES_WRITTEN_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    bytes_written: int
    error: str
    def __init__(self, request_id: _Optional[str] = ..., bytes_written: _Optional[int] = ..., error: _Optional[str] = ...) -> None: ...

class WriteCloseRequest(_message.Message):
    __slots__ = ("request_id", "abort")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    ABORT_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    abort: bool
    def __init__(self, request_id: _Optional[str] = ..., abort: _Optional[bool] = ...) -> None: ...

class WriteCloseResponse(_message.Message):
    __slots__ = ("request_id", "path", "total_bytes", "error")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    TOTAL_BYTES_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    path: str
    total_bytes: int
    error: str
    def __init__(self, request_id: _Optional[str] = ..., path: _Optional[str] = ..., total_bytes: _Optional[int] = ..., error: _Optional[str] = ...) -> None: ...

class RenameRequest(_message.Message):
    __slots__ = ("request_id", "path", "new_name")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    NEW_NAME_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    path: str
    new_name: str
    def __init__(self, request_id: _Optional[str] = ..., path: _Optional[str] = ..., new_name: _Optional[str] = ...) -> None: ...

class RenameResponse(_message.Message):
    __slots__ = ("request_id", "path", "name", "error")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    path: str
    name: str
    error: str
    def __init__(self, request_id: _Optional[str] = ..., path: _Optional[str] = ..., name: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class DeleteRequest(_message.Message):
    __slots__ = ("request_id", "path", "recursive")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    RECURSIVE_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    path: str
    recursive: bool
    def __init__(self, request_id: _Optional[str] = ..., path: _Optional[str] = ..., recursive: _Optional[bool] = ...) -> None: ...

class DeleteResponse(_message.Message):
    __slots__ = ("request_id", "path", "error")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    path: str
    error: str
    def __init__(self, request_id: _Optional[str] = ..., path: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class MkdirRequest(_message.Message):
    __slots__ = ("request_id", "path", "name")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    path: str
    name: str
    def __init__(self, request_id: _Optional[str] = ..., path: _Optional[str] = ..., name: _Optional[str] = ...) -> None: ...

class MkdirResponse(_message.Message):
    __slots__ = ("request_id", "path", "name", "error")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    path: str
    name: str
    error: str
    def __init__(self, request_id: _Optional[str] = ..., path: _Optional[str] = ..., name: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class CreateFileRequest(_message.Message):
    __slots__ = ("request_id", "path", "name")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    path: str
    name: str
    def __init__(self, request_id: _Optional[str] = ..., path: _Optional[str] = ..., name: _Optional[str] = ...) -> None: ...

class CreateFileResponse(_message.Message):
    __slots__ = ("request_id", "path", "name", "error")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    path: str
    name: str
    error: str
    def __init__(self, request_id: _Optional[str] = ..., path: _Optional[str] = ..., name: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class TerminalOpenRequest(_message.Message):
    __slots__ = ("request_id", "cols", "rows", "target_user")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    COLS_FIELD_NUMBER: _ClassVar[int]
    ROWS_FIELD_NUMBER: _ClassVar[int]
    TARGET_USER_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    cols: int
    rows: int
    target_user: str
    def __init__(self, request_id: _Optional[str] = ..., cols: _Optional[int] = ..., rows: _Optional[int] = ..., target_user: _Optional[str] = ...) -> None: ...

class TerminalOpenResponse(_message.Message):
    __slots__ = ("request_id", "error", "effective_user")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    EFFECTIVE_USER_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    error: str
    effective_user: str
    def __init__(self, request_id: _Optional[str] = ..., error: _Optional[str] = ..., effective_user: _Optional[str] = ...) -> None: ...

class TerminalDataRequest(_message.Message):
    __slots__ = ("request_id", "data")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    data: bytes
    def __init__(self, request_id: _Optional[str] = ..., data: _Optional[bytes] = ...) -> None: ...

class TerminalDataResponse(_message.Message):
    __slots__ = ("request_id", "data")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    data: bytes
    def __init__(self, request_id: _Optional[str] = ..., data: _Optional[bytes] = ...) -> None: ...

class TerminalResizeRequest(_message.Message):
    __slots__ = ("request_id", "cols", "rows")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    COLS_FIELD_NUMBER: _ClassVar[int]
    ROWS_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    cols: int
    rows: int
    def __init__(self, request_id: _Optional[str] = ..., cols: _Optional[int] = ..., rows: _Optional[int] = ...) -> None: ...

class TerminalCloseRequest(_message.Message):
    __slots__ = ("request_id",)
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    def __init__(self, request_id: _Optional[str] = ...) -> None: ...

class TerminalExitResponse(_message.Message):
    __slots__ = ("request_id", "exit_code", "error")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    EXIT_CODE_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    exit_code: int
    error: str
    def __init__(self, request_id: _Optional[str] = ..., exit_code: _Optional[int] = ..., error: _Optional[str] = ...) -> None: ...

class AutomationExecuteRequest(_message.Message):
    __slots__ = ("request_id", "job_id", "type", "action", "params_json", "timeout_seconds")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    JOB_ID_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    ACTION_FIELD_NUMBER: _ClassVar[int]
    PARAMS_JSON_FIELD_NUMBER: _ClassVar[int]
    TIMEOUT_SECONDS_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    job_id: str
    type: str
    action: str
    params_json: str
    timeout_seconds: int
    def __init__(self, request_id: _Optional[str] = ..., job_id: _Optional[str] = ..., type: _Optional[str] = ..., action: _Optional[str] = ..., params_json: _Optional[str] = ..., timeout_seconds: _Optional[int] = ...) -> None: ...

class AutomationExecuteResponse(_message.Message):
    __slots__ = ("request_id", "job_id", "status", "exit_code", "stdout", "stderr", "result_data_json", "error_message", "cost_ms")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    JOB_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    EXIT_CODE_FIELD_NUMBER: _ClassVar[int]
    STDOUT_FIELD_NUMBER: _ClassVar[int]
    STDERR_FIELD_NUMBER: _ClassVar[int]
    RESULT_DATA_JSON_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    COST_MS_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    job_id: str
    status: str
    exit_code: int
    stdout: str
    stderr: str
    result_data_json: str
    error_message: str
    cost_ms: int
    def __init__(self, request_id: _Optional[str] = ..., job_id: _Optional[str] = ..., status: _Optional[str] = ..., exit_code: _Optional[int] = ..., stdout: _Optional[str] = ..., stderr: _Optional[str] = ..., result_data_json: _Optional[str] = ..., error_message: _Optional[str] = ..., cost_ms: _Optional[int] = ...) -> None: ...
