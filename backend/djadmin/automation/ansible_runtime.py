import shutil
import sys
from pathlib import Path


def get_ansible_playbook_command() -> str:
    """Resolve Ansible from the Python environment running the backend."""
    environment_command = Path(sys.executable).with_name('ansible-playbook')
    if environment_command.is_file() and environment_command.stat().st_mode & 0o111:
        return str(environment_command)
    return shutil.which('ansible-playbook') or 'ansible-playbook'
