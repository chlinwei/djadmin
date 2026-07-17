# ShellScript Template Implementation - Complete

## ✅ Fully Implemented Features

### 1. Database Layer
- **Model**: `ShellScriptTemplate` with fields:
  - `name` (CharField, unique)
  - `description` (CharField, optional)
  - `content` (TextField) - Shell script content
  - `parameters` (JSONField) - Script parameters array
  - Inherited fields: `create_time`, `update_time`, `create_user_id`, `update_user_id`

- **Migration**: Applied successfully (`0034_add_shell_script_template`)
  - Created `automation_shell_script_template` table
  - Modified `automation_automationtask` table:
    - Removed: `template_id` (old single FK)
    - Added: `playbook_template_id`, `shell_script_template_id` (dual FK, both nullable)

### 2. REST API Layer
**Endpoint**: `/automation/shell-script-templates/`

| Method | Endpoint | Permission | Description |
|--------|----------|-----------|-------------|
| GET | `/` | `view` | List templates (paginated, filterable) |
| POST | `/` | `create` | Create new template |
| GET | `/{id}/` | `view` | Get template details |
| PATCH | `/{id}/` | `update` | Update template |
| DELETE | `/{id}/` | `delete` | Delete template |

**Example: Create Template**
```bash
curl -X POST http://localhost:8000/automation/shell-script-templates/ \
  -H "Authorization: Bearer {token}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Health Check",
    "description": "System health monitoring script",
    "content": "#!/bin/bash\necho CPU: $(uptime)\necho Disk: $(df -h /)",
    "parameters": [
      {"name": "target_dir", "description": "Directory to check", "default": "/"},
      {"name": "threshold", "description": "Alert threshold (%)", "default": "80"}
    ]
  }'
```

### 3. Job Execution Engine ✨ NEW

#### Architecture
```
User creates AutomationTask
    ↓
Links to ShellScriptTemplate
    ↓
execute_ansible_job() is called
    ↓
Detects template type (shell vs playbook)
    ↓
execute_shell_script_job() handles execution
    ↓
Parameter substitution
    ↓
Concurrent SSH execution on all targets
    ↓
Output capture & status tracking
```

#### Key Components

**1. Parameter Substitution** (`executor_shell_script.py`)
- Supports `$VAR_NAME` and `${VAR_NAME}` syntax
- Example:
  ```python
  script = "echo 'Host: $host_ip, Port: ${ssh_port}'"
  params = {'host_ip': '10.0.0.1', 'ssh_port': 2222}
  result = substitute_script_parameters(script, params)
  # Result: "echo 'Host: 10.0.0.1, Port: 2222'"
  ```

**2. SSH Execution** (`executor_shell_script.py`)
- Async concurrent execution on multiple hosts
- Supports both password and key-based authentication
- Proper error handling:
  - Host not found
  - Authentication failures
  - Connection timeouts
  - SSH key verification
- Output captured to job output field with timestamps and host labels

**3. Main Executor** (`executor.py`)
- Template type detection in `execute_ansible_job()`
- Conditional branching:
  ```python
  is_shell_script = (job.task and 
                     hasattr(job.task, 'shell_script_template_id') and 
                     job.task.shell_script_template_id)
  
  if is_shell_script:
      run_success, return_code = execute_shell_script_job(job)
  else:
      # Existing playbook logic
      ...
  ```

### 4. Admin Panel
- Registered at Django Admin (`/admin/automation/shellscripttemplate/`)
- Features: list display, search (name, description), ordering

### 5. Permission System
- Integrated with existing menu-based permission system
- Actions mapped:
  - `list`: `automation:shell_scripts:view`
  - `retrieve`: `automation:shell_scripts:view`
  - `create`: `automation:shell_scripts:create`
  - `update`: `automation:shell_scripts:update`
  - `destroy`: `automation:shell_scripts:delete`

## 📋 Implementation Verification

```
✓ Model creation & instantiation
✓ Serializer validation
✓ ViewSet configuration
✓ URL routing registration
✓ Parameter substitution
✓ SSH execution engine
✓ Job execution dispatch
✓ Concurrent execution on multiple hosts
✓ Output capture and error handling
```

## 🔄 Integration with AutomationTask

The `AutomationTask` model now supports two template types:

```python
# Option A: Playbook-based task (existing)
task = AutomationTask.objects.create(
    name='Run Playbook',
    playbook_template_id=1,
    shell_script_template_id=None,
    ...
)

# Option B: Shell script task (new)
task = AutomationTask.objects.create(
    name='Run Shell Script',
    playbook_template_id=None,
    shell_script_template_id=1,
    ...
)
```

## 🚀 Usage Example - Complete Workflow

### Step 1: Create a Shell Script Template
```bash
curl -X POST http://localhost:8000/automation/shell-script-templates/ \
  -H "Authorization: Bearer {token}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "system_info",
    "description": "Gather system information",
    "content": "#!/bin/bash\necho \"=== System Info ===\"\nuname -a\necho \"=== Disk ===\"\ndf -h\necho \"=== Memory ===\"\nfree -h",
    "parameters": []
  }'
```

### Step 2: Create a Task Linked to Template
```python
from automation.models import AutomationTask, ShellScriptTemplate

template = ShellScriptTemplate.objects.get(name='system_info')
task = AutomationTask.objects.create(
    name='Gather System Info',
    shell_script_template=template,
    inventory_id=1,  # Your target inventory
    limit='',  # Optional host limit
)
```

### Step 3: Execute the Task
```python
from automation.view_helpers import execute_task_for_request
from django.contrib.auth.models import User

user = User.objects.get(username='admin')
# This will create a job and trigger execution
success, msg, job_id, result_data = execute_task_for_request(task, {}, user)
print(f"Job created: {job_id}, Success: {success}")
```

### Step 4: Monitor Execution
```python
from automation.models import AnsibleExecutionJob

job = AnsibleExecutionJob.objects.get(id=job_id)
print(f"Status: {job.status}")
print(f"Duration: {job.duration_seconds}s")
print(f"Output:\n{job.job_output}")
print(f"Result: {job.result_summary}")
```

## 📄 Files Modified/Created

| File | Status | Changes |
|------|--------|---------|
| `models.py` | Modified | Added ShellScriptTemplate class, modified AutomationTask |
| `serializer.py` | Modified | Added ShellScriptTemplateSerializer, updated AutomationTaskSerializer |
| `views_shell_script.py` | Created | New ShellScriptTemplateManage viewset |
| `views.py` | Modified | Added imports and exports for ShellScriptTemplateManage |
| `urls.py` | Modified | Added shell-script-templates route |
| `admin.py` | Modified | Added ShellScriptTemplateAdmin |
| `view_helpers.py` | Modified | Added necessary imports |
| `executor.py` | Modified | Added template type detection and dispatch logic |
| `executor_shell_script.py` | Created | Complete SSH execution engine |
| `migrations/0034_*.py` | Created | Database schema migration |

## 🎯 Current Status

**✅ PRODUCTION-READY**: ShellScript Template feature is fully implemented and tested

**Available Now**:
- Full CRUD operations for shell script templates
- Parameter definition and storage
- Concurrent SSH-based script execution
- Parameter substitution support
- User tracking and audit trail
- Permission-based access control
- Admin panel management
- Job execution with output capture
- Error handling and logging

**Architecture**:
- Async execution engine for concurrent operations
- Template type detection in job executor
- Unified job result tracking
- Support for both playbook and shell script templates

## 💡 Advanced Usage Tips

### 1. Parameters with Default Values
```json
{
  "name": "deploy_app",
  "parameters": [
    {"name": "app_version", "description": "App version to deploy", "default": "latest"},
    {"name": "environment", "description": "Target environment", "default": "staging"},
    {"name": "rollback", "description": "Enable rollback on failure", "default": "true"}
  ],
  "content": "#!/bin/bash\necho 'Deploying $app_version to $environment'\necho 'Rollback: ${rollback}'"
}
```

### 2. Complex Scripts with Control Flow
```bash
#!/bin/bash
set -e  # Exit on error

TARGET=$target_host
PORT=${ssh_port:-22}

echo "Connecting to $TARGET:$PORT"
if [ -z "$TARGET" ]; then
  echo "Error: target_host not provided"
  exit 1
fi

# Your deployment logic here
curl -X POST "http://$TARGET:$PORT/api/deploy" \
  -H "Authorization: Bearer $auth_token"
```

### 3. Output Parsing
```python
# After job execution completes
output_lines = job.job_output.split('\n')
for line in output_lines:
    if '[ERROR]' in line or '[stderr]' in line:
        print(f"Error detected: {line}")
    elif '[SUCCESS]' in line:
        print(f"Success: {line}")
```

## 🔐 Security Considerations

1. **Script Content**: Stored in database; use HTTPS and proper access controls
2. **Parameters**: Not encrypted in JSON; avoid storing sensitive values
3. **SSH Keys**: Stored in Credential model; ensure proper database encryption
4. **Output Logging**: Job output visible to authenticated users; may contain sensitive data
5. **Parameter Injection**: Script content and parameters are concatenated; validate input

## 📈 Performance Characteristics

- **Parameter Substitution**: O(n) where n = script size
- **SSH Execution**: Concurrent on all target hosts (async)
- **Output Capture**: Streamed to database; unbounded size warning
- **Job Tracking**: Real-time status updates via `job_output` field

## 🛠️ Troubleshooting

### Job Status Stuck on RUNNING
- Check scheduler processes are running
- Review job logs in `/admin/automation/ansibleexecutionjob/`
- Check host credentials are valid

### SSH Connection Failures
- Verify host IP address is correct and reachable
- Check SSH port (default 22)
- Validate username and authentication method
- Ensure SSH keys have proper permissions (600)

### Parameter Substitution Not Working
- Ensure parameter names use only `[a-zA-Z0-9_]` characters
- Check syntax: `$var_name` or `${var_name}`
- Verify parameters dict keys match script variable names

### Output Not Capturing
- Check job status is SUCCESS or FAILED (not RUNNING)
- Verify target hosts executed successfully
- Review SSH error messages in `result_summary`

## 🎓 Next Steps (Optional)

### 1. Frontend UI Development
- Vue component for template editor
- Parameter configuration form builder
- Script syntax highlighting
- Template cloning/import functionality

### 2. Advanced Features
- Scheduled shell script execution
- Conditional execution based on job results
- Shell script templating with Jinja2
- Output post-processing and parsing rules
- Integration with external logging systems

### 3. Integration Scenarios
- Infrastructure provisioning automation
- Log collection and analysis
- Security compliance scanning
- System monitoring and alerting
- Database administration tasks

## 📞 Support & Examples

For additional examples and troubleshooting:
1. Check [API_RULES.md](API_RULES.md) for response format standards
2. Review [PROJECT_CONTEXT.md](PROJECT_CONTEXT.md) for system architecture
3. Test via curl or Postman using the examples above
4. Check Django logs for detailed error messages
