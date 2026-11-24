package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Autumn-27/artex/ownerfw"
)

// Installing, removing and keeping the agent egress floor (egress_floor.go).
//
// A start installs the floor in one iptables-restore transaction per family,
// which also removes the jumps of an earlier start whose owners are no longer
// current. With the floor off (egressFilterEnv=0, or no privilege drop) a start
// removes the floor an earlier start left. While the server runs, Watch checks
// every EgressFloorCheckInterval that the chain and its OUTPUT jumps (at the top)
// are still what was installed: a firewalld or ufw reload, a
// netfilter-persistent restore or a rule inserted above the jumps is logged as
// an ALERT and the floor is re-applied.

// EgressFloorCheckInterval is how often Watch checks the floor.
const EgressFloorCheckInterval = 60 * time.Second

// egressFloorTimeout bounds the iptables calls of one install or check.
const egressFloorTimeout = 30 * time.Second

// resolvConfPath is read for the nameservers DNS may reach in a refused range.
var resolvConfPath = "/etc/resolv.conf"

// AgentEgressFloor is the floor a start installed, kept while the server runs.
type AgentEgressFloor struct {
	Status EgressFloorStatus
	priv   ExecPrivilege
	// keepers hold every family the floor is for, installed or not: Watch
	// retries one that failed at start-up (a held xtables lock, a firewall
	// restarting).
	keepers []*ownerfw.Keeper
	lastErr map[ownerfw.Family]string // the last error Watch logged per family
}

// geteuid is os.Geteuid, replaced in tests.
var geteuid = os.Geteuid

// floorRules are the floor's rules now: the operator's exceptions and the
// resolver from the environment and resolv.conf.
func floorRules(p ExecPrivilege) (ownerfw.Rules, []string, error) {
	resolv, _ := os.ReadFile(resolvConfPath)
	return agentEgressRules(p, os.Getenv(egressAllowEnv), string(resolv))
}

// InstallAgentEgressFloor installs the floor on Linux with the privilege drop on
// and logs the result; run executes iptables (ownerfw.ExecRunner in production).
// It never fails the start itself: the self-test decides, from what an executor
// process can reach. Call Watch on the result to keep the floor in force.
func InstallAgentEgressFloor(goos string, p ExecPrivilege, run ownerfw.Runner) *AgentEgressFloor {
	f := installAgentEgressFloor(goos, p, run)
	st := f.Status
	switch {
	case len(st.Installed) == 0:
		log.Printf("[egress] WARNING: %s; agent processes are not refused the cloud instance-metadata and link-local addresses unless another filter does it", st)
	case st.Problem != "":
		log.Printf("[egress] WARNING: %s", st)
	default:
		log.Printf("[egress] %s for uids %s", st, strings.Join(f.keepers[0].Rules.Owners, ", "))
		return f
	}
	if len(f.keepers) > len(st.Installed) {
		log.Printf("[egress] the agent egress floor is retried every %s until it installs", EgressFloorCheckInterval)
	}
	return f
}

func installAgentEgressFloor(goos string, p ExecPrivilege, run ownerfw.Runner) *AgentEgressFloor {
	f := &AgentEgressFloor{priv: p, lastErr: map[ownerfw.Family]string{}}
	off := ""
	switch {
	case goos != "linux":
		f.Status.Problem = "it needs Linux (iptables owner match)"
		return f
	case !p.Enabled:
		off = "the privilege drop is off, so agent processes have no uid of their own to filter"
	case strings.TrimSpace(os.Getenv(egressFilterEnv)) == "0":
		off = egressFilterEnv + "=0 turns it off"
	}
	ctx, cancel := context.WithTimeout(context.Background(), egressFloorTimeout)
	defer cancel()
	if off != "" {
		f.Status.Problem = off
		if geteuid() == 0 { // only root can have installed one
			f.Status.Problem += removeAgentEgressFloor(ctx, run)
		}
		return f
	}
	rules, bad, err := floorRules(p)
	for _, b := range bad {
		log.Printf("[egress] WARNING: %s: %q is not an IP address or CIDR; ignored by the agent egress floor", egressAllowEnv, b)
	}
	if err != nil {
		f.Status.Problem = err.Error()
		return f
	}
	v4 := &ownerfw.Keeper{Rules: rules, Family: ownerfw.IPv4, Run: run}
	v6 := &ownerfw.Keeper{Rules: rules, Family: ownerfw.IPv6, Run: run}
	if err := v4.Apply(ctx); err != nil {
		// Without the ipv4 floor there is no floor; Watch retries both families
		// unless iptables is missing.
		f.Status.Problem = egressFloorHint(err)
		if !errors.Is(err, ownerfw.ErrNoBinary) {
			f.keepers = []*ownerfw.Keeper{v4, v6}
		}
		return f
	}
	f.keepers = []*ownerfw.Keeper{v4}
	f.Status.Installed = []ownerfw.Family{ownerfw.IPv4}
	if err := v6.Apply(ctx); err != nil {
		f.Status.Problem = "ipv6 rules not installed (" + egressFloorHint(err) + "); fd00:ec2::254 and fe80::/10 stay reachable if the host has IPv6"
		if !errors.Is(err, ownerfw.ErrNoBinary) {
			f.keepers = append(f.keepers, v6) // retried by Watch
		}
		return f
	}
	f.keepers = append(f.keepers, v6)
	f.Status.Installed = append(f.Status.Installed, ownerfw.IPv6)
	return f
}

// removeAgentEgressFloor removes a floor an earlier start left, and says what it
// removed (as a suffix of the status), or "" when there was none or iptables
// cannot be read (not root, no iptables: then there is no floor of ours either).
func removeAgentEgressFloor(ctx context.Context, run ownerfw.Runner) string {
	var done []string
	for _, fam := range []ownerfw.Family{ownerfw.IPv4, ownerfw.IPv6} {
		jumps, existed, err := ownerfw.Remove(ctx, run, fam, egressFloorChain)
		switch {
		case err != nil && (jumps > 0 || existed):
			done = append(done, fmt.Sprintf("could not remove the %s floor of an earlier start: %v", fam, err))
		case jumps > 0 || existed:
			done = append(done, fmt.Sprintf("removed the %s floor of an earlier start (%d OUTPUT jumps)", fam, jumps))
		}
	}
	if len(done) == 0 {
		return ""
	}
	return "; " + strings.Join(done, "; ")
}

// Watch checks the floor every interval until ctx ends, and re-applies it when
// it changed. It returns at once when no floor was installed.
func (f *AgentEgressFloor) Watch(ctx context.Context, every time.Duration) {
	if f == nil || len(f.keepers) == 0 {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f.check(ctx)
		}
	}
}

// check is one round of Watch. It returns the changes it found, for tests.
func (f *AgentEgressFloor) check(parent context.Context) []string {
	ctx, cancel := context.WithTimeout(parent, egressFloorTimeout)
	defer cancel()
	rules, _, err := floorRules(f.priv)
	if err != nil {
		log.Printf("[egress] ALERT: the agent egress floor's rules cannot be built: %v; the installed rules stay", err)
		return nil
	}
	var found []string
	for _, k := range f.keepers {
		k.Rules = rules
		d, err := k.Ensure(ctx)
		if msg := fmt.Sprint(err); err != nil && f.lastErr[k.Family] == msg {
			continue // logged already; the next change is logged again
		} else if err != nil {
			f.lastErr[k.Family] = msg
		} else if f.lastErr[k.Family] != "" {
			delete(f.lastErr, k.Family)
			log.Printf("[egress] the %s agent egress floor can be checked again", k.Family)
		}
		switch {
		case d == "" && err != nil:
			log.Printf("[egress] ALERT: cannot check the %s agent egress floor: %v (logged once until it changes)", k.Family, err)
		case err != nil && d == ownerfw.DriftNotInstalled:
			log.Printf("[egress] ALERT: the %s agent egress floor still cannot be installed: %v (logged once until it changes)", k.Family, err)
		case d == ownerfw.DriftNotInstalled:
			log.Printf("[egress] the %s agent egress floor is installed now (it failed at start-up)", k.Family)
		case err != nil:
			log.Printf("[egress] ALERT: the %s agent egress floor changed while running (%s) and could not be re-applied: %v; agent processes may reach the cloud instance-metadata addresses", k.Family, d, err)
		case d == ownerfw.DriftRulesChanged:
			log.Printf("[egress] the %s agent egress floor was rebuilt: %s or %s changed", k.Family, egressAllowEnv, resolvConfPath)
		case d != "":
			log.Printf("[egress] ALERT: the %s agent egress floor changed while running (%s), possibly a firewall reload or a rule inserted above it; re-applied", k.Family, d)
		}
		if d != "" {
			found = append(found, k.Family.String()+": "+d)
		}
	}
	return found
}

func egressFloorHint(err error) string {
	if errors.Is(err, ownerfw.ErrNoBinary) {
		return err.Error() + " (install iptables)"
	}
	return err.Error() + " (missing CAP_NET_ADMIN, or no owner match in the kernel?)"
}
