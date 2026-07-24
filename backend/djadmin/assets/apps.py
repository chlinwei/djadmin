from django.apps import AppConfig


class AssetsConfig(AppConfig):
    default_auto_field = 'django.db.models.BigAutoField'
    name = 'assets'

    def ready(self):
        # agent-mode 文件传输 gRPC Server 必须和处理 webssh_host_mixin.py 请求的
        # 进程共享内存里的 AgentSessionRegistry，因此在 runserver 进程启动时一并
        # 拉起（而不是作为独立的 run* 管理命令），详见 grpc_transfer/server.py。
        from assets.grpc_transfer.server import start_grpc_server_in_background
        start_grpc_server_in_background()
