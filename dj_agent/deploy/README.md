# dj-agent deployment

`dj-agent.yml` is the batch installation and upgrade entrypoint. It copies the CGO-free Agent binary, writes its protected environment file, installs the systemd unit, and verifies that the service is active. The default backend Agent Gateway port is `9001`.

## Example inventory

```ini
[dj_agents]
agent-01 ansible_host=10.25.66.179
agent-02 ansible_host=10.25.66.180
```

Run from the repository root with a vaulted variables file:

```bash
ansible-playbook -i inventory.ini dj_agent/deploy/dj-agent.yml \
  -e @dj_agent/deploy/dj-agent-vars.vault.yml
```

The vaulted file must provide `dj_agent_backend_token`. The default Agent ID is the inventory hostname; override it with `dj_agent_id` when the backend asset record uses another ID.

The playbook expects the binary at `dj_agent/bin/dj-agent`. Override `dj_agent_binary_source` for a release artifact or CI build output.

For a single host without Ansible, `install.sh` remains available as the bootstrap path.
