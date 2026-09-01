from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0050_remove_task_dead_scope_fields'),
    ]

    operations = [
        migrations.RunSQL(
            sql=(
                'ALTER TABLE automation_execution_job '
                'DROP FOREIGN KEY automation_ansible_e_workflow_run_id_a5541b6d_fk_automatio, '
                'DROP COLUMN workflow_run_id'
            ),
            reverse_sql=migrations.RunSQL.noop,
        ),
        migrations.DeleteModel(
            name='AutomationWorkflowRun',
        ),
        migrations.DeleteModel(
            name='AutomationWorkflowTemplate',
        ),
    ]