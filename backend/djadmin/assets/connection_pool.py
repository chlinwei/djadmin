import threading
import time
from dataclasses import dataclass


@dataclass
class _PoolItem:
    ssh_client: object
    last_used: float


class SSHConnectionPool:
    def __init__(self, max_per_key=2, idle_seconds=120):
        self._max_per_key = max(1, int(max_per_key or 1))
        self._idle_seconds = max(1, int(idle_seconds or 1))
        self._lock = threading.RLock()
        self._pool = {}

    @staticmethod
    def _is_alive(ssh_client):
        try:
            transport = ssh_client.get_transport()
            return bool(transport and transport.is_active())
        except Exception:
            return False

    @staticmethod
    def _close_client(ssh_client):
        try:
            ssh_client.close()
        except Exception:
            pass

    def _prune_locked(self, key, now_ts):
        items = self._pool.get(key) or []
        if not items:
            return
        kept = []
        for item in items:
            if now_ts - item.last_used > self._idle_seconds:
                self._close_client(item.ssh_client)
                continue
            if not self._is_alive(item.ssh_client):
                self._close_client(item.ssh_client)
                continue
            kept.append(item)
        if kept:
            self._pool[key] = kept
        else:
            self._pool.pop(key, None)

    def acquire(self, key, factory):
        now_ts = time.time()
        with self._lock:
            self._prune_locked(key, now_ts)
            items = self._pool.get(key) or []
            while items:
                item = items.pop()
                if self._is_alive(item.ssh_client):
                    if items:
                        self._pool[key] = items
                    else:
                        self._pool.pop(key, None)
                    return item.ssh_client
                self._close_client(item.ssh_client)
            self._pool.pop(key, None)
        return factory()

    def release(self, key, ssh_client):
        if ssh_client is None:
            return
        if not self._is_alive(ssh_client):
            self._close_client(ssh_client)
            return
        now_ts = time.time()
        with self._lock:
            self._prune_locked(key, now_ts)
            items = self._pool.get(key) or []
            if len(items) >= self._max_per_key:
                self._close_client(ssh_client)
                return
            items.append(_PoolItem(ssh_client=ssh_client, last_used=now_ts))
            self._pool[key] = items

    def discard(self, ssh_client):
        if ssh_client is None:
            return
        self._close_client(ssh_client)