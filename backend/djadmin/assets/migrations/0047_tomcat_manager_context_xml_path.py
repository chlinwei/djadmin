from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0046_tomcat_baseline_config'),
    ]

    operations = [
        migrations.AddField(
            model_name='tomcatbaselineconfig',
            name='manager_context_xml_path',
            field=models.CharField(blank=True, default='${APP_HOME}/webapps/manager/META-INF/context.xml', max_length=512),
        ),
    ]
