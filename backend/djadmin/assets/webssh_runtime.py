class WebSSHRuntimeRegistry:
    """In-process registry for active WebSSH sessions and their live consumers."""

    _active_session_ids = set()
    _active_consumers = {}
    _active_session_host_ids = {}

    @classmethod
    def mark_active(cls, session_id, consumer=None, host_id=None):
        if not session_id:
            return
        cls._active_session_ids.add(session_id)
        if consumer is not None:
            cls._active_consumers[session_id] = consumer
        if host_id is None and consumer is not None:
            host_id = getattr(consumer, 'host_id', None)
        if host_id is not None:
            cls._active_session_host_ids[session_id] = int(host_id)

    @classmethod
    def mark_inactive(cls, session_id):
        if not session_id:
            return
        cls._active_session_ids.discard(session_id)
        cls._active_consumers.pop(session_id, None)
        cls._active_session_host_ids.pop(session_id, None)

    @classmethod
    def is_active(cls, session_id):
        return session_id in cls._active_session_ids

    @classmethod
    def get_active_session_ids(cls):
        return set(cls._active_session_ids)

    @classmethod
    def get_active_count_for_host(cls, host_id):
        if host_id in [None, '', 0, '0']:
            return 0
        try:
            target_host_id = int(host_id)
        except (TypeError, ValueError):
            return 0
        return sum(1 for value in cls._active_session_host_ids.values() if value == target_host_id)

    @classmethod
    def get_active_session_ids_for_host(cls, host_id):
        if host_id in [None, '', 0, '0']:
            return set()
        try:
            target_host_id = int(host_id)
        except (TypeError, ValueError):
            return set()
        return {
            session_id
            for session_id, mapped_host_id in cls._active_session_host_ids.items()
            if mapped_host_id == target_host_id
        }

    @classmethod
    async def flush_session_buffers(cls, session_id):
        consumer = cls._active_consumers.get(session_id)
        if consumer is None:
            return False
        await consumer._flush_content_buffer(force=True)
        return True

    @classmethod
    async def close_active_sessions_for_hosts(
        cls,
        host_ids,
        message='关联主机已删除，终端连接已断开',
        close_code=4410,
    ):
        if not host_ids:
            return 0

        target_host_ids = set()
        for host_id in host_ids:
            try:
                value = int(host_id)
            except (TypeError, ValueError):
                continue
            if value > 0:
                target_host_ids.add(value)

        if not target_host_ids:
            return 0

        target_session_ids = [
            session_id
            for session_id, mapped_host_id in list(cls._active_session_host_ids.items())
            if mapped_host_id in target_host_ids
        ]

        closed_count = 0
        for session_id in target_session_ids:
            consumer = cls._active_consumers.get(session_id)
            if consumer is None:
                cls.mark_inactive(session_id)
                continue

            try:
                await consumer._send_event('closed', {'message': message})
                if hasattr(consumer, 'audit_close_notified'):
                    consumer.audit_close_notified = True
            except Exception:
                pass

            try:
                await consumer.close(code=close_code)
            except Exception:
                cls.mark_inactive(session_id)
            finally:
                cls.mark_inactive(session_id)
            closed_count += 1

        return closed_count