from django.db import migrations


INSTALL_TEMPLATE_NAME = 'Fluent Bit 离线安装（RPM/DEB）'
UNINSTALL_TEMPLATE_NAME = 'Fluent Bit 离线卸载（RPM/DEB）'

INSTALL_PLAYBOOK = r'''---
- name: Install Fluent Bit from the djadmin offline repository
  hosts: all
  gather_facts: false
  become: false
  vars:
    remote_package_dir: "/tmp/djadmin-fluent-bit-offline"
    remote_package_path: "{{ remote_package_dir }}/{{ package_file_name }}"
  tasks:
    - name: Validate the offline package format
      ansible.builtin.assert:
        that:
          - package_format in ['rpm', 'deb']
        fail_msg: "Fluent Bit only supports an exact RPM or DEB package"

    - name: Reset the remote package directory
      ansible.builtin.file:
        path: "{{ remote_package_dir }}"
        state: absent

    - name: Create the remote package directory
      ansible.builtin.file:
        path: "{{ remote_package_dir }}"
        state: directory
        mode: "0755"

    - name: Transfer the platform package directory from djadmin
      ansible.builtin.copy:
        src: "{{ package_local_directory }}/"
        dest: "{{ remote_package_dir }}/"
        mode: "0644"

    - name: Calculate the transferred package checksum
      ansible.builtin.stat:
        path: "{{ remote_package_path }}"
        checksum_algorithm: sha256
      register: fluent_bit_package_stat

    - name: Verify the transferred package checksum
      ansible.builtin.assert:
        that:
          - fluent_bit_package_stat.stat.checksum == package_sha256
        fail_msg: "Fluent Bit offline package SHA256 verification failed"
      when: package_sha256 | length > 0

    - name: Validate and install the local RPM transaction
      ansible.builtin.shell: |
        set -euo pipefail
        packages=("{{ remote_package_dir }}"/*.rpm)
        test -e "${packages[0]}"
        rpm -Uvh --replacepkgs --test "${packages[@]}"
        rpm -Uvh --replacepkgs "${packages[@]}"
      args:
        executable: /bin/bash
      when: package_format == 'rpm'

    - name: Install the local DEB transaction
      ansible.builtin.shell: |
        set -euo pipefail
        packages=("{{ remote_package_dir }}"/*.deb)
        test -e "${packages[0]}"
        dpkg -i "${packages[@]}"
      args:
        executable: /bin/bash
      when: package_format == 'deb'

    - name: Create Fluent Bit configuration and state directories
      ansible.builtin.file:
        path: "{{ item }}"
        state: directory
        owner: root
        group: root
        mode: "0755"
      loop:
        - /etc/fluent-bit/inputs.d
        - /etc/fluent-bit/outputs.d
        - /var/lib/fluent-bit
        - /var/lib/fluent-bit/storage

    - name: Write the bootstrap input fragment
      ansible.builtin.copy:
        content: |
          [INPUT]
              Name  dummy
              Tag   djadmin.bootstrap
        dest: /etc/fluent-bit/inputs.d/_djadmin_bootstrap.conf
        owner: root
        group: root
        mode: "0644"

    - name: Write the bootstrap output fragment
      ansible.builtin.copy:
        content: |
          [OUTPUT]
              Name   null
              Match  djadmin.bootstrap
        dest: /etc/fluent-bit/outputs.d/_djadmin_bootstrap.conf
        owner: root
        group: root
        mode: "0644"

    - name: Write the Fluent Bit main configuration
      ansible.builtin.copy:
        content: "{{ fluent_bit_main_config }}"
        dest: /etc/fluent-bit/fluent-bit.conf
        owner: root
        group: root
        mode: "0644"

    - name: Enable and start Fluent Bit
      ansible.builtin.systemd:
        name: "{{ service_name }}"
        enabled: true
        state: restarted
        daemon_reload: true

    - name: Wait for Fluent Bit startup
      ansible.builtin.pause:
        seconds: 2

    - name: Confirm Fluent Bit remains active
      ansible.builtin.command:
        argv:
          - systemctl
          - is-active
          - "{{ service_name }}"
      changed_when: false

    - name: Remove the transferred package directory
      ansible.builtin.file:
        path: "{{ remote_package_dir }}"
        state: absent
'''

UNINSTALL_PLAYBOOK = r'''---
- name: Uninstall Fluent Bit
  hosts: all
  gather_facts: false
  become: false
  tasks:
    - name: Stop and disable Fluent Bit
      ansible.builtin.systemd:
        name: "{{ service_name }}"
        enabled: false
        state: stopped
      failed_when: false

    - name: Check whether the RPM package is installed
      ansible.builtin.command:
        argv:
          - rpm
          - -q
          - fluent-bit
      register: fluent_bit_rpm_query
      changed_when: false
      failed_when: fluent_bit_rpm_query.rc not in [0, 1]
      when: package_format == 'rpm'

    - name: Remove the RPM package
      ansible.builtin.command:
        argv:
          - rpm
          - -e
          - fluent-bit
      when:
        - package_format == 'rpm'
        - fluent_bit_rpm_query.rc == 0

    - name: Check whether the DEB package is installed
      ansible.builtin.command:
        argv:
          - dpkg-query
          - -W
          - "-f=${Status}"
          - fluent-bit
      register: fluent_bit_deb_query
      changed_when: false
      failed_when: fluent_bit_deb_query.rc not in [0, 1]
      when: package_format == 'deb'

    - name: Remove the DEB package
      ansible.builtin.command:
        argv:
          - dpkg
          - --purge
          - fluent-bit
      when:
        - package_format == 'deb'
        - fluent_bit_deb_query.rc == 0

    - name: Remove Fluent Bit configuration and offsets
      ansible.builtin.file:
        path: "{{ item }}"
        state: absent
      loop:
        - /etc/fluent-bit
        - /var/lib/fluent-bit

    - name: Reload systemd units
      ansible.builtin.systemd:
        daemon_reload: true
'''


def bind_fluent_bit_playbooks(apps, schema_editor):
    PlaybookTemplate = apps.get_model('automation', 'PlaybookTemplate')
    SoftwarePackage = apps.get_model('monitor', 'SoftwarePackage')

    install_template, _ = PlaybookTemplate.objects.update_or_create(
        name=INSTALL_TEMPLATE_NAME,
        defaults={
            'description': '从 djadmin 本地仓库传输并安装精确匹配平台的 Fluent Bit RPM/DEB 包',
            'content': INSTALL_PLAYBOOK,
            'category': 'software_package',
        },
    )
    uninstall_template, _ = PlaybookTemplate.objects.update_or_create(
        name=UNINSTALL_TEMPLATE_NAME,
        defaults={
            'description': '离线卸载 Fluent Bit 并清理配置和 offset 数据',
            'content': UNINSTALL_PLAYBOOK,
            'category': 'software_package',
        },
    )
    SoftwarePackage.objects.filter(
        package_type='fluent_bit', name='fluent-bit',
    ).update(
        install_playbook_template_id=install_template.id,
        uninstall_playbook_template_id=uninstall_template.id,
    )
    LogCollectionTarget = apps.get_model('monitor', 'LogCollectionTarget')
    LogCollectionTarget.objects.filter(agent_installed=True).update(
        install_status='success', install_message='Fluent Bit 已安装',
    )


def unbind_fluent_bit_playbooks(apps, schema_editor):
    PlaybookTemplate = apps.get_model('automation', 'PlaybookTemplate')
    SoftwarePackage = apps.get_model('monitor', 'SoftwarePackage')

    SoftwarePackage.objects.filter(
        package_type='fluent_bit', name='fluent-bit',
    ).update(install_playbook_template_id=None, uninstall_playbook_template_id=None)
    PlaybookTemplate.objects.filter(
        name__in=[INSTALL_TEMPLATE_NAME, UNINSTALL_TEMPLATE_NAME],
    ).delete()


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0048_remove_shell_script_automation'),
        ('monitor', '0055_logcollectiontarget_install_message_and_more'),
    ]

    operations = [
        migrations.RunPython(bind_fluent_bit_playbooks, unbind_fluent_bit_playbooks),
    ]