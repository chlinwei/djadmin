from django.apps import AppConfig


class InspectionConfig(AppConfig):
    default_auto_field = 'django.db.models.BigAutoField'
    name = 'inspection'
    verbose_name = '巡检中心'

    def ready(self):
        # 巡检靠 gRPC 通道下发作业，而通道注册表只存在于 runserver 进程内存里，
        # 所以定时分发必须跟着这个进程走，不能交给 Celery beat/worker。
        from .scheduling import start_inspection_scheduler_in_background
        start_inspection_scheduler_in_background()