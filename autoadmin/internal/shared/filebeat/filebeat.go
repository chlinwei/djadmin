// Package filebeat 提供 Filebeat 软件包默认的安装/卸载 Playbook 与 systemd unit 内容。
// 两个域共用：软件包仓库（monitor）在创建/编辑/回填软件包配置时写入这些内容，
// 日志采集（logcollect）在派发安装时作为 service unit 缺失的兜底。
//
// 内容是**写入软件包配置**的（automation_playbook_template 与软件包的
// install/uninstall_playbook_template_id、service_file_content），不是运行时兜底；
// 用户可以在软件仓库编辑弹窗里看到并修改。派发方注入的额外变量：
// service_name（固定 filebeat）、package_local_directory、package_file_name、package_sha256。
// 安装只解压 + enable，不 start：此时还没有 filebeat.yml，首次「下发配置」写入后再启动。
package filebeat

// ServiceUnit Filebeat 默认 systemd unit 内容：写入软件包配置的 service_file_content，
// 派发时作为 extra_vars.service_file_content 传给 playbook（与 exporter 安装同一约定）。
const ServiceUnit = `[Unit]
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

// InstallPlaybook 便携包（tar.gz）离线安装：拷贝 → 校验 sha256 → 解压到 /opt/filebeat
// → 写 systemd unit → enable。不 start（见包注释）。
const InstallPlaybook = `---
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

// UninstallPlaybook 停用并删除 systemd unit、/opt/filebeat。
const UninstallPlaybook = `---
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

// InstallPlaybookContent 安装 Playbook 内容（写入软件包配置用）。
func InstallPlaybookContent() string { return InstallPlaybook }

// UninstallPlaybookContent 卸载 Playbook 内容（写入软件包配置用）。
func UninstallPlaybookContent() string { return UninstallPlaybook }

// ServiceUnitContent systemd unit 内容（写入软件包配置、或派发时兜底）。
func ServiceUnitContent() string { return ServiceUnit }
