package db

import (
	"net/netip"
	"strings"
)

// TaskScopeRules is the task-level block and allow rules of one task.
type TaskScopeRules struct {
	TaskID int64
	Block  []AssetInterceptRule
	Allow  []AssetInterceptRule
}

// ActiveTaskScopeRules returns the enabled task-level rules of every task that
// can still run (created, running or paused; not archived or deleted), one
// entry per task that has at least one enabled allow rule.
func (s *AssetStore) ActiveTaskScopeRules() ([]TaskScopeRules, error) {
	rows, err := s.db.Query(`
SELECT r.task_id, r.action, r.kind, r.pattern
  FROM task_intercept_rules r JOIN tasks t ON t.id = r.task_id
 WHERE r.enabled AND t.deleted_at IS NULL AND t.archived_at IS NULL
   AND t.status IN ('created','running','paused')
 ORDER BY r.task_id, r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskScopeRules
	for rows.Next() {
		var taskID int64
		r := AssetInterceptRule{Enabled: true}
		if err := rows.Scan(&taskID, &r.Action, &r.Kind, &r.Pattern); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].TaskID != taskID {
			out = append(out, TaskScopeRules{TaskID: taskID})
		}
		t := &out[len(out)-1]
		if r.Action == "allow" {
			t.Allow = append(t.Allow, r)
		} else {
			t.Block = append(t.Block, r)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	kept := out[:0]
	for _, t := range out {
		if len(t.Allow) > 0 {
			kept = append(kept, t)
		}
	}
	return kept, nil
}

// HostInTaskAllowScope reports whether host (a name or an IP, no port) is
// named by an enabled allow rule of some task in tasks, that task's block
// rules and the global block rules do not hit it. Unlike EvaluateAssetGate,
// an empty allow list never matches: the caller widens trust only for what a
// task names explicitly. The host is matched as written, with no DNS lookup:
// a domain rule covers a name, an exact_ip, fuzzy_ip or cidr rule an address,
// and a fuzzy_url rule the origin https://host/.
func HostInTaskAllowScope(host string, global []AssetInterceptRule, tasks []TaskScopeRules) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]")), ".")
	if host == "" {
		return false
	}
	var domains, ips []string
	origin := host
	if a, err := netip.ParseAddr(host); err == nil {
		a = a.Unmap()
		ips = []string{a.String()}
		if a.Is6() {
			origin = "[" + a.String() + "]"
		}
	} else {
		domains = []string{host}
	}
	urls := []string{"https://" + origin + "/"}
	if _, _, hit := MatchAssetInterceptRules(global, domains, ips, urls); hit {
		return false
	}
	for _, t := range tasks {
		if _, _, ok := MatchAssetAllowRules(t.Allow, domains, ips, urls); !ok {
			continue
		}
		if _, _, hit := MatchAssetInterceptRules(t.Block, domains, ips, urls); !hit {
			return true
		}
	}
	return false
}
