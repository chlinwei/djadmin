import re

from django.utils import timezone
from rest_framework import serializers

from assets.models import WebSSHSessionLog
from assets.webssh_runtime import WebSSHRuntimeRegistry
from .models import LoginAuditLog, OperationAuditLog


class WebSSHSessionLogAuditSerializer(serializers.ModelSerializer):
    host_name = serializers.SerializerMethodField()
    host_ip = serializers.SerializerMethodField()
    status = serializers.SerializerMethodField()
    duration_seconds = serializers.SerializerMethodField()

    class Meta:
        model = WebSSHSessionLog
        fields = [
            'id',
            'session_id',
            'host',
            'host_name',
            'host_ip',
            'user_id',
            'username',
            'client_ip',
            'user_agent',
            'status',
            'start_time',
            'end_time',
            'duration_seconds',
            'close_code',
            'error_message',
            'input_bytes',
            'command_count',
            'recorded_content_bytes',
            'is_content_truncated',
        ]

    def get_host_name(self, obj):
        system = getattr(obj.host, 'system', None)
        hostname = getattr(system, 'hostname', None) if system else None
        return obj.host.instance_name or hostname or f'Host-{obj.host.id}'

    def get_host_ip(self, obj):
        return obj.host.ip

    def get_status(self, obj):
        if obj.status != WebSSHSessionLog.Status.CONNECTED:
            return obj.status
        return (
            WebSSHSessionLog.Status.CONNECTED
            if WebSSHRuntimeRegistry.is_active(obj.session_id)
            else WebSSHSessionLog.Status.CLOSED
        )

    def get_duration_seconds(self, obj):
        if obj.duration_seconds is not None:
            return obj.duration_seconds
        if self.get_status(obj) == WebSSHSessionLog.Status.CONNECTED and obj.start_time:
            return max(int((timezone.now() - obj.start_time).total_seconds()), 0)
        return None


class LoginAuditLogSerializer(serializers.ModelSerializer):
    class Meta:
        model = LoginAuditLog
        fields = '__all__'


class OperationAuditLogSerializer(serializers.ModelSerializer):
    class Meta:
        model = OperationAuditLog
        fields = '__all__'


class WebSSHSessionLogContentSerializer(serializers.ModelSerializer):
    raw_input_content = serializers.SerializerMethodField()
    raw_output_content = serializers.SerializerMethodField()
    input_content = serializers.SerializerMethodField()
    output_content = serializers.SerializerMethodField()
    readable_input_content = serializers.SerializerMethodField()
    readable_input_commands = serializers.SerializerMethodField()

    class Meta:
        model = WebSSHSessionLog
        fields = [
            'id',
            'session_id',
            'status',
            'start_time',
            'end_time',
            'duration_seconds',
            'raw_input_content',
            'raw_output_content',
            'input_content',
            'output_content',
            'readable_input_content',
            'readable_input_commands',
            'recorded_content_bytes',
            'is_content_truncated',
        ]

    _OSC_RE = re.compile(r'\x1B\][^\x07\x1B]*(?:\x07|\x1B\\)')
    _CSI_RE = re.compile(r'\x1B\[[0-?]*[ -/]*[@-~]')
    _ESC_RE = re.compile(r'\x1B[@-_]')
    _CTRL_RE = re.compile(r'[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]')
    _PROMPT_LINE_RE = re.compile(r'^(?:\[[^\]]+\]|[^\s]+@[^\s]+(?::[^\s]+)?)\s*[#$>]\s*(.*)$')

    def get_raw_input_content(self, obj):
        return obj.input_content or ''

    def get_raw_output_content(self, obj):
        return obj.output_content or ''

    def get_input_content(self, obj):
        return self._sanitize_terminal_input_text(obj.input_content)

    def get_output_content(self, obj):
        return self._sanitize_terminal_text(obj.output_content)

    def get_readable_input_content(self, obj):
        commands = self._get_readable_commands(obj)
        if commands:
            return '\n'.join(commands)
        return self._sanitize_terminal_input_text(obj.input_content)

    def get_readable_input_commands(self, obj):
        commands = self._get_readable_commands(obj)
        if commands:
            return commands

        content = self._sanitize_terminal_input_text(obj.input_content)
        if not content:
            return []
        return [line.strip() for line in content.split('\n') if line.strip()]

    def _get_readable_commands(self, obj):
        commands = self._extract_commands_from_output(obj.output_content)
        if commands:
            return commands
        return []

    @classmethod
    def _sanitize_terminal_text(cls, text):
        if not text:
            return ''

        # 先去掉 ANSI 控制序列，再处理退格和不可见控制字符，保留可读换行文本。
        cleaned = cls._OSC_RE.sub('', text)
        cleaned = cls._CSI_RE.sub('', cleaned)
        cleaned = cls._ESC_RE.sub('', cleaned)
        cleaned = cls._apply_backspace(cleaned)
        cleaned = cls._CTRL_RE.sub('', cleaned)
        cleaned = cleaned.replace('\r\n', '\n').replace('\r', '\n')
        return cleaned

    @classmethod
    def _sanitize_terminal_input_text(cls, text):
        """Normalize terminal key stream into readable command lines.

        Input stream may include control keys (Ctrl+C, arrows, DEL). Reconstructing
        line edits avoids artifacts like mixed commands or missing characters.
        """
        if not text:
            return ''

        source = text
        lines = []
        line = []
        cursor = 0
        i = 0
        n = len(source)

        def flush_line():
            nonlocal line, cursor
            lines.append(''.join(line))
            line = []
            cursor = 0

        while i < n:
            ch = source[i]

            # Handle CSI controls such as arrow keys: ESC [ D/C/H/F
            if ch == '\x1b' and i + 1 < n and source[i + 1] == '[':
                j = i + 2
                while j < n and not ('@' <= source[j] <= '~'):
                    j += 1
                if j >= n:
                    break

                final = source[j]
                if final == 'D':
                    cursor = max(0, cursor - 1)
                elif final == 'C':
                    cursor = min(len(line), cursor + 1)
                elif final == 'H':
                    cursor = 0
                elif final == 'F':
                    cursor = len(line)

                i = j + 1
                continue

            # CR/LF means command submitted (or newline in terminal stream).
            if ch in {'\r', '\n'}:
                flush_line()
                # Collapse CRLF as one newline.
                if ch == '\r' and i + 1 < n and source[i + 1] == '\n':
                    i += 2
                else:
                    i += 1
                continue

            # DEL / Backspace: remove one char before cursor.
            if ch in {'\x7f', '\b'}:
                if cursor > 0:
                    line.pop(cursor - 1)
                    cursor -= 1
                i += 1
                continue

            # Ctrl+U: clear current input line.
            if ch == '\x15':
                line = []
                cursor = 0
                i += 1
                continue

            # Ctrl+W: delete previous word.
            if ch == '\x17':
                while cursor > 0 and line[cursor - 1].isspace():
                    line.pop(cursor - 1)
                    cursor -= 1
                while cursor > 0 and not line[cursor - 1].isspace():
                    line.pop(cursor - 1)
                    cursor -= 1
                i += 1
                continue

            # Ignore other control characters (Ctrl+C/Ctrl+L etc.).
            if ord(ch) < 32:
                i += 1
                continue

            # Insert printable char at cursor.
            line.insert(cursor, ch)
            cursor += 1
            i += 1

        # Keep pending text so in-progress command can still be displayed.
        if line:
            lines.append(''.join(line))

        normalized = '\n'.join(lines)
        return cls._sanitize_terminal_text(normalized)

    @classmethod
    def _extract_commands_from_output(cls, output_text):
        if not output_text:
            return []

        cleaned = cls._sanitize_terminal_text(output_text)
        commands = []
        for line in cleaned.split('\n'):
            stripped = line.strip()
            if not stripped:
                continue

            match = cls._PROMPT_LINE_RE.match(stripped)
            if not match:
                continue

            cmd = (match.group(1) or '').strip()
            if not cmd:
                continue

            # Skip full-width IME transient lines that may occasionally echo only spaces.
            if not cmd.strip():
                continue

            commands.append(cmd)

        return commands

    @staticmethod
    def _apply_backspace(text):
        if '\b' not in text:
            return text

        buffer = []
        for ch in text:
            if ch == '\b':
                if buffer:
                    buffer.pop()
            else:
                buffer.append(ch)
        return ''.join(buffer)
