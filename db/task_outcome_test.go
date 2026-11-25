package db

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// A task created with a company records it in task_scope only; the outcome
// card must report that company (the RSI proposer's transfer scope reads it),
// not the legacy tasks.company_id column, which task creation leaves NULL.
func TestTaskOutcomeCompanyFromTaskScope(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })

	stamp := time.Now().UnixNano()
	company, _, err := d.Companies().UpsertCompany(fmt.Sprintf("OutcomeCo %d", stamp), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupCompany(d, company) })
	scoped, err := d.CreateTaskWithOptions("outcome scope", "goal", TaskCreateOptions{
		Name: fmt.Sprintf("outcome-co-%d", stamp), CompanyIDs: []int64{company},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupErr(t, "scoped task", d.DeleteTask(scoped.ID)) })
	bare, err := d.CreateTaskWithOptions("outcome bare", "goal", TaskCreateOptions{
		Name: fmt.Sprintf("outcome-bare-%d", stamp),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupErr(t, "bare task", d.DeleteTask(bare.ID)) })

	ctx := context.Background()
	o, err := d.TaskOutcome(ctx, scoped.ID)
	if err != nil {
		t.Fatal(err)
	}
	if o.CompanyID != company {
		t.Fatalf("scoped task outcome company_id = %d, want %d", o.CompanyID, company)
	}
	o, err = d.TaskOutcome(ctx, bare.ID)
	if err != nil {
		t.Fatal(err)
	}
	if o.CompanyID != 0 {
		t.Fatalf("task without a company: company_id = %d, want 0", o.CompanyID)
	}
}
