package ownerfw

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner runs one command and returns its combined output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// ExecRunner runs the command with os/exec.
func ExecRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// ErrNoBinary is returned (wrapped) when the family's iptables binary cannot be run.
var ErrNoBinary = errors.New("iptables binary not found")

// Apply runs the plan with run. It stops at the first step that fails (after its
// fallback), so a partly built chain refuses at most what was added before.
func Apply(ctx context.Context, run Runner, p Plan) error {
	bin := p.Family.Binary()
	for _, s := range p.Steps {
		args := s.Args
		out, err := run(ctx, bin, args...)
		if err != nil && s.Fallback != nil {
			args = s.Fallback
			out, err = run(ctx, bin, args...)
		}
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%w: %s", ErrNoBinary, bin)
		}
		if err != nil {
			return fmt.Errorf("%s %s: %v: %s", bin, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
