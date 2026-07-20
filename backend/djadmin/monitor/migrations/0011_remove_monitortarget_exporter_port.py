from django.db import migrations


class Migration(migrations.Migration):
    """移除 MonitorTarget.exporter_port 字段：exporter 监听端口不再由 backend/dj-agent 自动注入，
    改由用户自行在 manage_script 中硬编码或通过 env_vars 传递，避免"UI 填了端口但 manage_script
    没引用就静默不生效"的隐患。"""

    dependencies = [
        ('monitor', '0010_manage_script_backend_only_download'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='monitortarget',
            name='exporter_port',
        ),
    ]
