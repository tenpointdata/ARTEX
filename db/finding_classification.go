package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Autumn-27/artex/vulnmeta"
)

// Client-grade classification columns on findings (migration 0017): CVSS v4.0
// vector + computed score, optional EPSS, remediation and framework tags. The
// typed contract and its validation live in package vulnmeta; this file only
// maps it to columns.

// classificationCols is the select fragment for one table alias, in the order
// classificationScan.dest scans it.
func classificationCols(alias string) string {
	p := alias + "."
	return fmt.Sprintf(`COALESCE(%[1]scvss_vector,''), %[1]scvss_score::float8, %[1]sepss_probability, %[1]sepss_percentile,
	       %[1]sepss_date, COALESCE(%[1]sepss_cve,''), COALESCE(%[1]sremediation,''), COALESCE(%[1]sframework_tags,'[]'::jsonb)`, p)
}

// classificationScan receives the columns of classificationCols.
type classificationScan struct {
	vector      string
	score       sql.NullFloat64
	probability sql.NullFloat64
	percentile  sql.NullFloat64
	date        sql.NullTime
	cve         string
	remediation string
	tags        []byte
}

func (c *classificationScan) dest() []any {
	return []any{&c.vector, &c.score, &c.probability, &c.percentile, &c.date, &c.cve, &c.remediation, &c.tags}
}

// value rebuilds the typed classification. Stored rows were normalized on
// write, so no re-validation runs here; a tag array that no longer decodes is
// dropped rather than failing the whole read.
func (c *classificationScan) value() vulnmeta.Classification {
	out := vulnmeta.Classification{Remediation: c.remediation}
	if c.vector != "" && c.score.Valid {
		out.CVSS = &vulnmeta.CVSS{Vector: c.vector, Score: c.score.Float64, Rating: vulnmeta.Rating(c.score.Float64)}
	}
	if c.probability.Valid {
		e := &vulnmeta.EPSS{Probability: c.probability.Float64, Percentile: c.percentile.Float64, CVE: c.cve}
		if c.date.Valid {
			e.Date = c.date.Time.Format(time.DateOnly)
		}
		out.EPSS = e
	}
	var tags []vulnmeta.Tag
	if json.Unmarshal(c.tags, &tags) == nil && len(tags) > 0 {
		out.Tags = tags
	}
	return out
}

// classificationArgs flattens a normalized classification into the column
// values of the UPDATE in SetFindingClassification, in column order.
func classificationArgs(c vulnmeta.Classification) []any {
	var vector string
	var score, probability, percentile, date any
	var cve string
	if c.CVSS != nil {
		vector, score = c.CVSS.Vector, c.CVSS.Score
	}
	if c.EPSS != nil {
		probability, percentile, cve = c.EPSS.Probability, c.EPSS.Percentile, c.EPSS.CVE
		if c.EPSS.Date != "" {
			date = c.EPSS.Date
		}
	}
	tags := c.Tags
	if tags == nil {
		tags = []vulnmeta.Tag{}
	}
	tagsJSON, _ := json.Marshal(tags)
	return []any{vector, score, probability, percentile, date, cve, c.Remediation, string(tagsJSON)}
}

// SetFindingClassification applies a partial classification update to one
// finding and returns the stored result. The read-modify-write runs under a row
// lock, so two concurrent patches touching different parts both survive. found
// is false when no finding has that id; a validation failure wraps
// vulnmeta.ErrInvalid and changes nothing.
func (d *DB) SetFindingClassification(ctx context.Context, id int64, p vulnmeta.Patch) (out vulnmeta.Classification, found bool, err error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return out, false, err
	}
	defer tx.Rollback()
	var cur classificationScan
	err = tx.QueryRowContext(ctx, `SELECT `+classificationCols("f")+` FROM findings f WHERE f.id=$1 FOR UPDATE`, id).Scan(cur.dest()...)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	if out, err = p.Apply(cur.value()); err != nil {
		return vulnmeta.Classification{}, true, err
	}
	args := append(classificationArgs(out), id)
	if _, err = tx.ExecContext(ctx, `UPDATE findings SET cvss_vector=$1, cvss_score=$2, epss_probability=$3, epss_percentile=$4,
		epss_date=$5::date, epss_cve=$6, remediation=$7, framework_tags=$8::jsonb WHERE id=$9`, args...); err != nil {
		return vulnmeta.Classification{}, true, err
	}
	return out, true, tx.Commit()
}
