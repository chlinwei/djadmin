package monitor

// Filebeat 软件包默认的安装/卸载 Playbook 内容：在创建/编辑软件包时**写入软件包配置**
// （automation_playbook_template + install/uninstall_playbook_template_id），不是运行时兜底；
// 用户可以在软件仓库编辑弹窗里看到并修改。额外变量由 dispatchLogTargetInstall 注入：
//   service_name（固定 filebeat）、package_local_directory、package_file_name、package_sha256。
// 安装只解压 + enable，不 start：此时还没有 filebeat.yml，首次「下发配置」写入后再启动。

// Filebeat 默认 systemd unit 内容：创建/编辑/回填时写入软件包配置的 service_file_content，
// dispatch 时作为 extra_vars.service_file_content 传给 playbook（与 exporter 安装同一约定）。
const builtinFilebeatServiceUnit = `[Unit]
Description=Filebeat
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/opt/filebeat/filebeat -c /etc/filebeat/filebeat.yml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`

const builtinFilebeatInstallPlaybook = `---
- name: Install Filebeat (portable tar.gz)
  hosts: all
  become: true
  gather_facts: false
  vars:
    fb_home: /opt/filebeat
    archive: "/tmp/{{ package_file_name }}"
  tasks:
    - name: Copy package to target
      ansible.builtin.copy:
        src: "{{ package_local_directory }}/{{ package_file_name }}"
        dest: "{{ archive }}"
        mode: "0644"
    - name: Verify checksum
      ansible.builtin.shell: 'echo "{{ package_sha256 }}  {{ archive }}" | sha256sum -c -'
      args:
        executable: /bin/bash
    - name: Extract to {{ fb_home }}
      ansible.builtin.shell: |
        set -euo pipefail
        rm -rf {{ fb_home }}.new && mkdir -p {{ fb_home }}.new
        tar -xzf {{ archive }} -C {{ fb_home }}.new --strip-components=1
        rm -rf {{ fb_home }} && mv {{ fb_home }}.new {{ fb_home }}
        rm -f {{ archive }}
      args:
        executable: /bin/bash
    - name: Ensure data dir
      ansible.builtin.file:
        path: /var/lib/filebeat
        state: directory
        mode: "0755"
    - name: Write systemd unit
      ansible.builtin.copy:
        dest: "/etc/systemd/system/{{ service_name }}.service"
        mode: "0644"
        content: "{{ service_file_content }}"
    - name: Enable service (start after first config apply)
      ansible.builtin.systemd:
        daemon_reload: true
        name: "{{ service_name }}"
        enabled: true
`

const builtinFilebeatUninstallPlaybook = `---
- name: Uninstall Filebeat
  hosts: all
  become: true
  gather_facts: false
  tasks:
    - name: Stop and disable service
      ansible.builtin.systemd:
        name: "{{ service_name }}.service"
        enabled: false
        state: stopped
      failed_when: false
    - name: Remove systemd unit
      ansible.builtin.file:
        path: "/etc/systemd/system/{{ service_name }}.service"
        state: absent
    - name: Remove Filebeat home
      ansible.builtin.file:
        path: /opt/filebeat
        state: absent
    - name: Reload systemd
      ansible.builtin.systemd:
        daemon_reload: true
`

func builtinFilebeatInstallPlaybookContent() string   { return builtinFilebeatInstallPlaybook }
func builtinFilebeatUninstallPlaybookContent() string { return builtinFilebeatUninstallPlaybook }
func builtinFilebeatServiceUnitContent() string       { return builtinFilebeatServiceUnit }
