package db

import (
	"database/sql"
	"fmt"
	"slices"
	"strconv"
)

// Client bindings (migration 0019, artex_user_companies): the companies a
// console user may see. Only the client_read role is restricted by them; the
// server enforces that (server/client_scope.go). Writes run under withActor so
// the audit trigger records who granted or withdrew access.

// UserCompanies lists the company ids userID is bound to, ascending.
func (d *DB) UserCompanies(userID int64) ([]int64, error) {
	rows, err := d.Query(`SELECT company_id FROM artex_user_companies WHERE user_id = $1 ORDER BY company_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetUserCompanies replaces userID's bindings with companyIDs (duplicates
// ignored; empty withdraws every binding). It fails with ErrUserNotFound or
// ErrCompanyNotFound and then changes nothing.
func (d *DB) SetUserCompanies(actor string, userID int64, companyIDs []int64) error {
	want := slices.Clone(companyIDs)
	slices.Sort(want)
	want = slices.Compact(want)
	return d.withActor(actor, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM artex_users WHERE id = $1)`, userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrUserNotFound
		}
		for _, cid := range want {
			if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM companies WHERE id = $1)`, cid).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("%w: %d", ErrCompanyNotFound, cid)
			}
		}
		if _, err := tx.Exec(`DELETE FROM artex_user_companies
			WHERE user_id = $1 AND NOT (company_id = ANY($2::bigint[]))`, userID, int64Array(want)); err != nil {
			return err
		}
		for _, cid := range want {
			if _, err := tx.Exec(`INSERT INTO artex_user_companies (user_id, company_id, created_by)
				VALUES ($1, $2, $3) ON CONFLICT (user_id, company_id) DO NOTHING`, userID, cid, actor); err != nil {
				return err
			}
		}
		return nil
	})
}

// TaskCompaniesIn returns the companies among companyIDs that taskID is
// scoped to by an operator: task_scope rows of kind 'company' whose source is
// not 'agent'. Agent-added rows do not count, so an agent (driven by untrusted
// target output) cannot widen which clients see a task. Empty = not visible.
func (d *DB) TaskCompaniesIn(taskID int64, companyIDs []int64) ([]int64, error) {
	if taskID <= 0 || len(companyIDs) == 0 {
		return nil, nil
	}
	rows, err := d.Query(`SELECT DISTINCT company_id FROM task_scope
		WHERE task_id = $1 AND kind = 'company' AND source <> 'agent' AND company_id = ANY($2::bigint[])
		ORDER BY company_id`, taskID, int64Array(companyIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// FindingMatches reports whether finding id passes filter f (the list
// endpoint's rule, including Company and StrictCompany).
func (d *DB) FindingMatches(f FindingFilter, id int64) (bool, error) {
	if id <= 0 {
		return false, nil
	}
	f, err := d.applyAssetScope(f)
	if err != nil {
		return false, err
	}
	where, args := f.where()
	args = append(args, id)
	cond := fmt.Sprintf("f.id = $%d", len(args))
	if where == "" {
		where = " WHERE " + cond
	} else {
		where += " AND " + cond
	}
	var ok bool
	err = d.QueryRow(`SELECT EXISTS(SELECT 1 FROM findings f LEFT JOIN tasks t ON f.task_id = t.id`+where+`)`, args...).Scan(&ok)
	return ok, err
}

// int64Array renders ids as a PostgreSQL bigint[] literal ("{1,2}").
func int64Array(ids []int64) string {
	b := []byte{'{'}
	for i, id := range ids {
		if i > 0 {
			b = append(b, ',')
		}
		b = strconv.AppendInt(b, id, 10)
	}
	return string(append(b, '}'))
}

// CompanySummary is the client-safe view of a company: no scope rules.
type CompanySummary struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Logo       *string `json:"logo,omitempty"`
	AssetCount int     `json:"asset_count"`
}

// CompanySummaries lists the companies among ids (nil = every company), by name.
func (d *DB) CompanySummaries(ids []int64) ([]CompanySummary, error) {
	q := `SELECT c.id, c.name, c.logo, COUNT(a.id) FROM companies c
		LEFT JOIN assets a ON a.company_id = c.id`
	args := []any{}
	if ids != nil {
		q += ` WHERE c.id = ANY($1::bigint[])`
		args = append(args, int64Array(ids))
	}
	rows, err := d.Query(q+` GROUP BY c.id ORDER BY c.name, c.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CompanySummary{}
	for rows.Next() {
		var c CompanySummary
		if err := rows.Scan(&c.ID, &c.Name, &c.Logo, &c.AssetCount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CompanyTaskIDs lists the tasks an operator scoped to companyID, with the
// TaskCompaniesIn rule (agent-added rows do not count), ascending.
func (d *DB) CompanyTaskIDs(companyID int64) ([]int64, error) {
	rows, err := d.Query(`SELECT DISTINCT task_id FROM task_scope
		WHERE company_id = $1 AND kind = 'company' AND source <> 'agent' ORDER BY task_id`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
