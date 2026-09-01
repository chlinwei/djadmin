package ansible

import (
	"context"
	"fmt"

	"github.com/apenella/go-ansible/v2/pkg/playbook"
)

type Executor struct {
	binary string
}

func NewExecutor(binary string) *Executor {
	if binary == "" {
		binary = playbook.DefaultAnsiblePlaybookBinary
	}
	return &Executor{binary: binary}
}

func (executor *Executor) Run(ctx context.Context, playbookPath string) error {
	runner := playbook.NewAnsiblePlaybookExecute(playbookPath).WithBinary(executor.binary)
	if err := runner.Execute(ctx); err != nil {
		return fmt.Errorf("execute ansible playbook: %w", err)
	}
	return nil
}
