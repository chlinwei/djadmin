from django.contrib import admin

from .models import PlaybookTemplate, ShellScriptTemplate, AutomationExecutionJob


@admin.register(PlaybookTemplate)
class PlaybookTemplateAdmin(admin.ModelAdmin):
    list_display = ('id', 'name', 'create_time', 'update_time')
    search_fields = ('name',)


@admin.register(ShellScriptTemplate)
class ShellScriptTemplateAdmin(admin.ModelAdmin):
    list_display = ('id', 'name', 'create_time', 'update_time')
    search_fields = ('name', 'description')


@admin.register(AutomationExecutionJob)
class AutomationExecutionJobAdmin(admin.ModelAdmin):
    list_display = ('id', 'job_id', 'task_name_snapshot', 'template_name_snapshot', 'status', 'requested_username', 'start_time', 'end_time')
    search_fields = ('job_id', 'requested_username')
    list_filter = ('status', 'trigger_type')

