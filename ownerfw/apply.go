package ownerfw

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// Runner runs one command with stdin as its input and returns its combined output.
type Runner func(ctx context.Context, stdin, name string, args ...string) ([]byte, error)

// ExecRunner runs the command with os/exec.
func ExecRunner(ctx context.Context, stdin, name string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		c.Stdin = strings.NewReader(stdin)
	}
	return c.CombinedOutput()
}

// ErrNoBinary is returned (wrapped) when the family's iptables binary cannot be run.
var ErrNoBinary = errors.New("iptables binary not found")

func runErr(name string, args []string, out []byte, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%w: %s", ErrNoBinary, name)
	}
	return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
}

// list returns the -S lines of chain, and false when the chain does not exist.
func list(ctx context.Context, run Runner, f Family, chain string) ([]string, bool, error) {
	args := []string{"-w", "-S", chain}
	out, err := run(ctx, "", f.Binary(), args...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || chain == "OUTPUT" {
			return nil, false, runErr(f.Binary(), args, out, err)
		}
		return nil, false, nil // no such chain
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines, true, nil
}

func restore(ctx context.Context, run Runner, f Family, input string) error {
	args := []string{"-w", "--noflush"}
	if out, err := run(ctx, input, f.RestoreBinary(), args...); err != nil {
		return runErr(f.RestoreBinary(), args, out, err)
	}
	return nil
}

// Keeper installs Rules for one family and tells when the kernel no longer
// holds what it installed. Set Rules before a call to change what is kept. A
// Keeper is not safe for concurrent use.
type Keeper struct {
	Rules  Rules
	Family Family
	Run    Runner

	applied string   // Rules.Render at the last Apply
	chain   []string // iptables -S Chain right after the last Apply
}

// Apply installs the rules in one transaction (see the package comment) and
// records what the chain looks like afterwards, for Drift.
func (k *Keeper) Apply(ctx context.Context) error {
	if err := k.Rules.Validate(); err != nil {
		return err
	}
	output, _, err := list(ctx, k.Run, k.Family, "OUTPUT")
	if err != nil {
		return err
	}
	if err := restore(ctx, k.Run, k.Family, k.Rules.restoreInput(k.Family, output)); err != nil {
		return err
	}
	chain, ok, err := list(ctx, k.Run, k.Family, k.Rules.Chain)
	if err != nil || !ok {
		k.applied, k.chain = "", nil
		return fmt.Errorf("%s: chain %s missing right after it was installed (%v)", k.Family, k.Rules.Chain, err)
	}
	k.applied, k.chain = k.Rules.Render(k.Family), chain
	return nil
}

// DriftRulesChanged is the Drift when the Rules, not the kernel, changed.
const DriftRulesChanged = "the rules to install changed"

// Drift says how the kernel or the Rules differ from the last Apply, or "" when
// nothing changed: the Rules changed, the chain is gone or its rules differ, or
// OUTPUT does not start with exactly one jump per owner (in order) and has no
// other jump to the chain.
func (k *Keeper) Drift(ctx context.Context) (string, error) {
	if k.applied == "" {
		return "not installed yet", nil
	}
	if k.Rules.Render(k.Family) != k.applied {
		return DriftRulesChanged, nil
	}
	chain, ok, err := list(ctx, k.Run, k.Family, k.Rules.Chain)
	if err != nil {
		return "", err
	}
	if !ok {
		return "chain " + k.Rules.Chain + " is gone", nil
	}
	if !slices.Equal(chain, k.chain) {
		return fmt.Sprintf("chain %s differs from what was installed (%d lines now, %d then)", k.Rules.Chain, len(chain), len(k.chain)), nil
	}
	output, _, err := list(ctx, k.Run, k.Family, "OUTPUT")
	if err != nil {
		return "", err
	}
	return jumpDrift(k.Rules, output), nil
}

// jumpDrift checks the OUTPUT rules against r's jumps.
func jumpDrift(r Rules, output []string) string {
	var rules []string
	for _, l := range output {
		if strings.HasPrefix(l, "-A OUTPUT") {
			rules = append(rules, l)
		}
	}
	for i, o := range r.Owners {
		if i >= len(rules) || !isJumpTo(rules[i], r.Chain) {
			return fmt.Sprintf("OUTPUT rule %d is not the jump to %s for uid %s", i+1, r.Chain, o)
		}
		if got := flagValue(fields(rules[i]), "--uid-owner"); got != o {
			return fmt.Sprintf("OUTPUT rule %d jumps to %s for uid %q, not %s", i+1, r.Chain, got, o)
		}
	}
	for i, l := range rules[len(r.Owners):] {
		if isJumpTo(l, r.Chain) {
			return fmt.Sprintf("OUTPUT rule %d is an extra jump to %s", len(r.Owners)+i+1, r.Chain)
		}
	}
	return ""
}

// Ensure re-applies the rules when Drift reports a difference, and returns it.
func (k *Keeper) Ensure(ctx context.Context) (string, error) {
	d, err := k.Drift(ctx)
	if err != nil || d == "" {
		return "", err
	}
	return d, k.Apply(ctx)
}

// Remove deletes every OUTPUT jump to chain and the chain, in one transaction.
// It returns how many jumps it deleted and whether the chain existed; when
// there was neither, it changes nothing.
func Remove(ctx context.Context, run Runner, f Family, chain string) (jumps int, existed bool, err error) {
	output, _, err := list(ctx, run, f, "OUTPUT")
	if err != nil {
		return 0, false, err
	}
	_, existed, err = list(ctx, run, f, chain)
	if err != nil {
		return 0, false, err
	}
	for _, l := range output {
		if isJumpTo(l, chain) {
			jumps++
		}
	}
	if jumps == 0 && !existed {
		return 0, false, nil
	}
	return jumps, existed, restore(ctx, run, f, removeInput(chain, output, existed))
}
