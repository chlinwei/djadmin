from django.db import migrations


def forwards_func(apps, schema_editor):
    ApiToken = apps.get_model('user', 'ApiToken')

    # 旧枚举回填：shared/none_shared -> agent/api
    ApiToken.objects.filter(bind_mode='shared').update(bind_mode='agent')
    ApiToken.objects.filter(bind_mode='none_shared').update(bind_mode='api')

    # 纠正中间错误状态：api(共享)/agent(绑定) -> agent(共享)/api(绑定)
    ApiToken.objects.filter(bind_mode='api', agent_id='global').update(bind_mode='agent')
    ApiToken.objects.filter(bind_mode='agent').exclude(agent_id='global').update(bind_mode='api')


def backwards_func(apps, schema_editor):
    ApiToken = apps.get_model('user', 'ApiToken')

    # 回滚到上一版语义：agent(共享)/api(绑定) -> api(共享)/agent(绑定)
    ApiToken.objects.filter(bind_mode='agent', agent_id='global').update(bind_mode='api')
    ApiToken.objects.filter(bind_mode='api').exclude(agent_id='global').update(bind_mode='agent')


class Migration(migrations.Migration):

    dependencies = [
        ('user', '0011_rename_agenttoken_to_apitoken'),
    ]

    operations = [
        migrations.RunPython(forwards_func, backwards_func),
    ]
