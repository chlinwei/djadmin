from django.db import migrations, models


class Migration(migrations.Migration):
    """将“权限提升配置”（ansible become_enabled/become_method/become_user）替换为
    “执行身份配置”（run_as_user/run_as_group）+ 工作目录（work_directory）。
    dj-agent 改为以 root 运行，实际执行自动化任务时通过 setuid/setgid 降权到 run_as_user/run_as_group，
    不再依赖 ansible 的 become 机制（详见 dj_agent/internal/executor/automation.go）。

    run_as_user 迁移期对已有数据回填为 'root'（preserve_default=False，仅用于本次迁移的 ALTER TABLE 默认值，
    模型本身不再保留默认值，后续新建/编辑任务必须显式指定，避免遗漏配置导致以 root 静默执行）。
    """

    dependencies = [
        ('automation', '0043_alter_automationtask_execution_timeout_seconds'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='automationtask',
            name='become_enabled',
        ),
        migrations.RemoveField(
            model_name='automationtask',
            name='become_method',
        ),
        migrations.RemoveField(
            model_name='automationtask',
            name='become_user',
        ),
        migrations.RemoveField(
            model_name='automationexecutionjob',
            name='become_enabled_snapshot',
        ),
        migrations.RemoveField(
            model_name='automationexecutionjob',
            name='become_method_snapshot',
        ),
        migrations.RemoveField(
            model_name='automationexecutionjob',
            name='become_user_snapshot',
        ),
        migrations.AddField(
            model_name='automationtask',
            name='run_as_user',
            field=models.CharField(default='root', help_text='任务执行时降权切换到的系统用户（必填，不允许留空以 root 身份静默执行）', max_length=100),
            preserve_default=False,
        ),
        migrations.AddField(
            model_name='automationtask',
            name='run_as_group',
            field=models.CharField(blank=True, default='', help_text='任务执行时切换到的系统组，留空则使用 run_as_user 的主组', max_length=100),
        ),
        migrations.AddField(
            model_name='automationtask',
            name='work_directory',
            field=models.CharField(blank=True, default='/tmp', help_text='任务执行时的工作目录，默认为 /tmp', max_length=255),
        ),
        migrations.AddField(
            model_name='automationexecutionjob',
            name='run_as_user_snapshot',
            field=models.CharField(blank=True, default='', max_length=100),
        ),
        migrations.AddField(
            model_name='automationexecutionjob',
            name='run_as_group_snapshot',
            field=models.CharField(blank=True, default='', max_length=100),
        ),
        migrations.AddField(
            model_name='automationexecutionjob',
            name='work_directory_snapshot',
            field=models.CharField(blank=True, default='/tmp', max_length=255),
        ),
    ]
