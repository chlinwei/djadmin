from django.db import migrations


def remove_shell_automation_data(apps, schema_editor):
    """删除已废弃的 Shell 任务，并移除 Workflow 中对这些任务的引用。"""
    AutomationTask = apps.get_model('automation', 'AutomationTask')
    Workflow = apps.get_model('automation', 'AutomationWorkflowTemplate')

    shell_task_ids = set(
        AutomationTask.objects.filter(shell_script_template__isnull=False)
        .values_list('id', flat=True)
    )
    if not shell_task_ids:
        return

    for workflow in Workflow.objects.all():
        nodes = workflow.nodes if isinstance(workflow.nodes, list) else []
        removed_keys = {
            str(node.get('key') or '')
            for node in nodes
            if isinstance(node, dict)
            and node.get('node_type') == 'task'
            and node.get('task_id') in shell_task_ids
        }
        if not removed_keys:
            continue

        workflow.nodes = [
            node for node in nodes
            if not (
                isinstance(node, dict)
                and node.get('node_type') == 'task'
                and node.get('task_id') in shell_task_ids
            )
        ]
        edges = workflow.edges if isinstance(workflow.edges, list) else []
        workflow.edges = [
            edge for edge in edges
            if not (
                isinstance(edge, dict)
                and (str(edge.get('source') or '') in removed_keys
                     or str(edge.get('target') or '') in removed_keys)
            )
        ]
        workflow.save(update_fields=['nodes', 'edges'])

    AutomationTask.objects.filter(id__in=shell_task_ids).delete()


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0047_automationcontrollersshkey'),
    ]

    operations = [
        migrations.RunPython(remove_shell_automation_data, migrations.RunPython.noop),
        migrations.RemoveField(
            model_name='automationtask',
            name='shell_script_template',
        ),
        migrations.RemoveField(
            model_name='automationtask',
            name='shell_parameters',
        ),
        migrations.RemoveField(
            model_name='automationexecutionjob',
            name='shell_parameters',
        ),
        migrations.RemoveField(
            model_name='automationexecutionjob',
            name='shell_env_vars',
        ),
        migrations.DeleteModel(
            name='ShellScriptTemplate',
        ),
    ]