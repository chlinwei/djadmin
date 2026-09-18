# Fluent Bit 卸载 playbook（RHEL/CentOS 7/8/9、Ubuntu 22.04）

采集器已迁到 Filebeat（见 [LOG_COLLECTOR_MIGRATION.md](../plans/LOG_COLLECTOR_MIGRATION.md)）。
本文给出把主机上遗留的 Fluent Bit 彻底卸干净的 playbook，用于迁移后的存量机器清理。

## 背景：本平台当年装的是什么

从历史软件包（已随迁移 000026 删除）可以看到实际装过的形态，卸载要对齐这些名字：

| 发行版 | 主包 | 一并装的依赖 |
|---|---|---|
| RHEL/CentOS 7 | `fluent-bit-5.1.1.rhel7.x86_64.rpm` | `libyaml-0.1.4`、`postgresql-libs-9.2` |
| RHEL/CentOS 8 | `fluent-bit-5.1.1.rhel8.x86_64.rpm` | `libyaml-0.1.7`、`libpq-13.3` |
| RHEL/CentOS 9 | `fluent-bit-4.0.1-1.rhel9.x86_64.rpm` | `libpq-13.5` |
| Ubuntu 22.04 | `fluent-bit_5.1.1_ubuntu22_amd64.deb` | `libyaml-0-2`、`libpq5` |

所以**包名两阵营都是 `fluent-bit`**（不是老的 `td-agent-bit`），systemd 单元为 `fluent-bit.service`，
配置在 `/etc/fluent-bit/`、缓冲与 offset 库在 `/var/lib/fluent-bit/`。

**卸载不用 `package` 模块**：它可能先去刷新 yum/apt 仓库元数据，离线主机会直接失败
（2026-09-18 实测 24 台里 3 台报 `Cannot download repomd.xml`）。改用 `rpm -e` / `dpkg -r`，只查本地包库。

**通用库不默认删**：`libyaml`/`libpq5`/`postgresql-libs` 是通用库，别的软件可能在用，删了可能连带
搞坏别的东西。确实要一并清理时用 `-e fluent_bit_remove_shared_libs=true`。

## playbook

存成「自动化 → Playbook 模板」后在「自动化 → 任务」里对目标主机执行即可。
幂等：机器上没装也不报错，可重复跑；全部走 `ansible.builtin.*`，不需要外网与额外 collection。

```yaml
---
# 卸载 Fluent Bit（RHEL/CentOS 7/8/9、Ubuntu 22.04）。
# 幂等：未安装的主机不报错，可重复执行。采集器已迁到 Filebeat，本 playbook 只清采集器本身。
- name: Uninstall Fluent Bit
  hosts: all
  become: true
  gather_facts: false
  vars:
    # 当年为 fluent-bit 一并装的通用库：默认不删（可能被其它软件使用）。
    # 需要连带清理时：-e fluent_bit_remove_shared_libs=true
    fluent_bit_remove_shared_libs: false
    fluent_bit_shared_libs:
      - libyaml
      - libyaml-0-2
      - libpq5
      - postgresql-libs
    # 单元名（不带 .service，systemd 模块会自己补）。td-agent-bit 是更早的命名，兼容处理。
    fluent_bit_units:
      - fluent-bit
      - td-agent-bit
    # 包外的残留：配置、缓冲/offset 库、日志、环境文件、手工装的 tar 包落点。
    fluent_bit_paths:
      - /etc/fluent-bit
      - /etc/td-agent-bit
      - /opt/fluent-bit
      - /var/lib/fluent-bit
      - /var/lib/td-agent-bit
      - /var/log/fluent-bit
      - /var/log/fluent-bit.log
      - /var/log/td-agent-bit
      - /etc/default/fluent-bit
      - /etc/sysconfig/fluent-bit
      - /usr/lib/systemd/system/fluent-bit.service
      - /usr/lib/systemd/system/fluent-bit.service.d
      - /etc/systemd/system/fluent-bit.service
      - /etc/systemd/system/fluent-bit.service.d
      - /lib/systemd/system/fluent-bit.service
      - /usr/lib/systemd/system/td-agent-bit.service
      - /etc/systemd/system/td-agent-bit.service
  tasks:
    # 1) 先停服并禁用（单元可能已不存在，容错继续）。
    #    必须先停：直接删包时若进程仍持有 /var/lib/fluent-bit 下的 offset 库，
    #    可能留下"半卸载"状态；也避免与 Filebeat 同时 tail 同一批文件造成重复写入。
    - name: Stop and disable Fluent Bit services
      ansible.builtin.systemd:
        name: "{{ item }}"
        state: stopped
        enabled: false
      loop: "{{ fluent_bit_units }}"
      failed_when: false

    # 2) 查已安装包，据此决定要卸哪些（避免对不存在的包调用包管理器而报错）。
    - name: Collect installed packages
      ansible.builtin.package_facts:
      failed_when: false

    - name: Determine Fluent Bit packages to remove
      ansible.builtin.set_fact:
        fluent_bit_packages_to_remove: "{{ ['fluent-bit', 'td-agent-bit'] | select('in', ansible_facts.packages | default({})) | list }}"

    - name: Check for dpkg (Debian/Ubuntu)
      ansible.builtin.stat:
        path: /usr/bin/dpkg
      register: fluent_bit_dpkg

    # 3) 直接调 rpm/dpkg，**不要用 package 模块**：package 模块在部分主机会先去刷新仓库元数据，
    #    而本环境的主机连不到外部 yum/apt 源，会直接报
    #    "Failed to download metadata for repo 'baseos': Cannot download repomd.xml" 而失败
    #    （2026-09-18 实测：24 台里 3 台如此，其余 21 台成功）。rpm -e / dpkg -r 只查本地包库，
    #    不碰网络，符合"离线环境"的前提。
    #    这里只对 package_facts 确认已安装的包执行，所以 rc != 0 是真失败（如依赖冲突），要报出来。
    - name: "Remove Fluent Bit packages (RHEL/CentOS: rpm -e)"
      ansible.builtin.command: "rpm -e {{ item }}"
      loop: "{{ fluent_bit_packages_to_remove }}"
      when: not fluent_bit_dpkg.stat.exists
      changed_when: true

    - name: "Remove Fluent Bit packages (Ubuntu: dpkg -r)"
      ansible.builtin.command: "dpkg -r {{ item }}"
      loop: "{{ fluent_bit_packages_to_remove }}"
      when: fluent_bit_dpkg.stat.exists
      changed_when: true

    # 3) 清包外的残留（配置、offset 库、日志、单元文件与 drop-in）。
    - name: Remove Fluent Bit leftover files and unit files
      ansible.builtin.file:
        path: "{{ item }}"
        state: absent
      loop: "{{ fluent_bit_paths }}"

    - name: Reload systemd daemon
      ansible.builtin.systemd:
        daemon_reload: true
      failed_when: false

    # 4) 可选：连带删掉当年为它装的通用库（默认关闭，风险见文件头注释）。
    - name: Determine shared libraries to remove (opt-in)
      ansible.builtin.set_fact:
        fluent_bit_shared_to_remove: "{{ fluent_bit_shared_libs | select('in', ansible_facts.packages | default({})) | list }}"
      when: fluent_bit_remove_shared_libs | bool

    - name: "Remove shared libraries installed for Fluent Bit (RHEL/CentOS)"
      ansible.builtin.command: "rpm -e {{ item }}"
      loop: "{{ fluent_bit_shared_to_remove | default([]) }}"
      when: (fluent_bit_remove_shared_libs | bool) and not fluent_bit_dpkg.stat.exists
      changed_when: true

    - name: "Remove shared libraries installed for Fluent Bit (Ubuntu)"
      ansible.builtin.command: "dpkg -r {{ item }}"
      loop: "{{ fluent_bit_shared_to_remove | default([]) }}"
      when: (fluent_bit_remove_shared_libs | bool) and fluent_bit_dpkg.stat.exists
      changed_when: true

    # 5) 校验：二进制与单元都不应还在，否则明确失败而不是"看起来成功了"。
    #    check_mode: false —— 这两条是只读探测，必须在检查模式下也真跑：否则注册变量只有
    #    "已跳过"、没有 rc，下面的报告与失败判定会误判（检查模式跑出来是假报"未清干净"）。
    - name: Check fluent-bit binary
      ansible.builtin.command: command -v fluent-bit
      register: fluent_bit_binary
      changed_when: false
      failed_when: false
      check_mode: false

    # 单元不存在时 systemctl 退出码 1 且 stdout 为空，因此用"stdout 非空"判残留。
    - name: Check fluent-bit service unit
      ansible.builtin.command: systemctl list-unit-files fluent-bit.service td-agent-bit.service --no-legend
      register: fluent_bit_unit
      changed_when: false
      failed_when: false
      check_mode: false

    - name: Report uninstall result
      ansible.builtin.debug:
        msg: >-
          fluent-bit 二进制：{{ '仍存在 ' + fluent_bit_binary.stdout if (fluent_bit_binary.rc | default(1)) == 0 else '已移除' }}
          ｜ unit：{{ (fluent_bit_unit.stdout | default('', true) | trim) or '已移除' }}

    - name: Fail when Fluent Bit is still present
      ansible.builtin.fail:
        msg: >-
          Fluent Bit 未清干净：{{ '二进制仍在 ' + fluent_bit_binary.stdout if (fluent_bit_binary.rc | default(1)) == 0
          else '检测到残留 unit：' + (fluent_bit_unit.stdout | trim) }}。
          多半是用 tar 包/手工方式装到了非默认路径，请登录主机确认。
      when: (fluent_bit_binary.rc | default(1)) == 0 or (fluent_bit_unit.stdout | default('', true) | trim | length > 0)
```

## 使用与注意事项

- **执行时机**：应在该主机已经用 Filebeat 接管采集**之后**再卸 Fluent Bit，否则期间日志断采。
  两者并存时会同时 tail 同一批文件 → 同一行日志被写入两遍（ES 里 document 重复），
  所以迁移期不要长期双跑。
- **offset 不通用**：Fluent Bit 的 offset 库在 `/var/lib/fluent-bit/`，与 Filebeat 的 registry 不共享。
  删掉它不影响已交接的 Filebeat；但**回滚**到 Fluent Bit 会从文件末尾重读或重复采集一段。
  需要留回滚余地时，把 `fluent_bit_paths` 里的 `/var/lib/fluent-bit` 与 `/etc/fluent-bit` 两行去掉再跑。
- **数据不删**：本 playbook 只清采集器，**不动 Elasticsearch/OpenSearch 里的历史数据**。
  旧索引按各自保留策略（ILM）到期即可；要提前清见架构文档 §9.6 的数据流清理。
- **批量执行**：建议分批（如每批 50 台）执行，避免几百台同时重启采集器对 ES 写入造成抖动；
  平台的批量动作目前是请求内串行（见 [LOG_COLLECTION_LIFECYCLE](../plans/LOG_COLLECTION_LIFECYCLE.md) §8 规模基线）。
- **检查模式（--check）**：校验环节的两条命令带 `check_mode: false`，会在检查模式下真跑（只读），
  所以 `--check` 也能给出可信的结论，不会假报"未清干净"。
- **校验失败的含义**：最后的 `fail` 只在"二进制还在"或"仍有 unit"时触发，说明主机上有非平台装的
  Fluent Bit（tar 包/手工装到了别处），需要登机确认，不要直接忽略。
