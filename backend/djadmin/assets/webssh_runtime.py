class WebSSHRuntimeRegistry:
    """In-process registry for active WebSSH sessions and their live consumers."""

    _active_session_ids = set()
    _active_consumers = {}

    @classmethod
    def mark_active(cls, session_id, consumer=None):
        if not session_id:
            return
        cls._active_session_ids.add(session_id)
        if consumer is not None:
            cls._active_consumers[session_id] = consumer

    @classmethod
    def mark_inactive(cls, session_id):
        if not session_id:
            return
        cls._active_session_ids.discard(session_id)
        cls._active_consumers.pop(session_id, None)

    @classmethod
    def is_active(cls, session_id):
        return session_id in cls._active_session_ids

    @classmethod
    def get_active_session_ids(cls):
        return set(cls._active_session_ids)

    @classmethod
    async def flush_session_buffers(cls, session_id):
        consumer = cls._active_consumers.get(session_id)
        if consumer is None:
            return False
        await consumer._flush_content_buffer(force=True)
        return True