package assets

import (
	"strings"
	"testing"
)

const testAgentPlaybook = `---
- name: Install or update dj-agent
  hosts: target
  tasks:
    - name: Write config
      ansible.builtin.copy:
        content: |
          DJ_AGENT_ID={{ dj_agent_id }}
          DJ_AGENT_GRPC_FILE_ADDR={{ dj_agent_grpc_addr }}
        dest: /etc/dj-agent/config.env
    - name: Write unit
      ansible.builtin.copy:
        content: |
          [Unit]
          ExecStart=/usr/local/bin/dj-agent
        dest: /usr/lib/systemd/system/dj-agent.service
`

func TestPlaybookCopyContentsExtractsCopyTasks(t *testing.T) {
	contents, err := playbookCopyContents(testAgentPlaybook)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(contents["/etc/dj-agent/config.env"], "DJ_AGENT_ID=") {
		t.Fatalf("config.env content missing: %q", contents["/etc/dj-agent/config.env"])
	}
	if !strings.Contains(contents["/usr/lib/systemd/system/dj-agent.service"], "ExecStart=") {
		t.Fatalf("unit content missing: %q", contents["/usr/lib/systemd/system/dj-agent.service"])
	}
}

func TestPlaybookCopyContentsRejectsInvalidYAML(t *testing.T) {
	if _, err := playbookCopyContents("\t-bad: [yaml"); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestPlaybookAgentTemplates(t *testing.T) {
	contents, err := playbookCopyContents(testAgentPlaybook)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env, unit, err := playbookAgentTemplates(contents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(env, "{{ dj_agent_id }}") || !strings.Contains(env, "{{ dj_agent_grpc_addr }}") {
		t.Fatalf("env template should keep placeholders: %q", env)
	}
	if !strings.Contains(unit, "ExecStart=") {
		t.Fatalf("unit template wrong: %q", unit)
	}
}

func TestPlaybookAgentTemplatesRejectsMissingTasks(t *testing.T) {
	broken, err := playbookCopyContents("---\n- hosts: target\n  tasks: []\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, _, err = playbookAgentTemplates(broken); err == nil {
		t.Fatal("expected error when copy tasks are missing")
	}
}

func TestRenderAgentEnvTemplate(t *testing.T) {
	rendered := renderAgentEnvTemplate("DJ_AGENT_ID={{ dj_agent_id }}\nDJ_AGENT_GRPC_FILE_ADDR={{ dj_agent_grpc_addr }}\nDJ_AGENT_LOG_LEVEL=info", "agent-001", "10.0.0.1:9100")
	want := "DJ_AGENT_ID=agent-001\nDJ_AGENT_GRPC_FILE_ADDR=10.0.0.1:9100\nDJ_AGENT_LOG_LEVEL=info\n"
	if rendered != want {
		t.Fatalf("rendered mismatch:\n got: %q\nwant: %q", rendered, want)
	}
}

func TestParseAnsibleRecap(t *testing.T) {
	output := "PLAY RECAP ****\ntarget : ok=5 changed=2 unreachable=0 failed=1 skipped=0\n"
	recap := parseAnsibleRecap(output)
	if recap["ok"] != 5 || recap["changed"] != 2 || recap["failed"] != 1 {
		t.Fatalf("recap wrong: %v", recap)
	}
	if recap["unreachable"] != 0 || recap["skipped"] != 0 {
		t.Fatalf("recap defaults wrong: %v", recap)
	}
	empty := parseAnsibleRecap("no recap here")
	if empty["ok"] != 0 || len(empty) != 5 {
		t.Fatalf("empty recap wrong: %v", empty)
	}
}
