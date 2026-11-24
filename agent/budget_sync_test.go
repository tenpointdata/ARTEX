package agent

import "testing"

// SyncSpend raises a ledger to the spend recorded by every node and never
// lowers it, so a dispatch decision counts the other nodes' calls (E7).
func TestBudgetSyncSpendOnlyRaises(t *testing.T) {
	l := NewBudgetLedger(NewBudget(10000, 100), NewBudget(0, 0))
	l.RestoreSpend(Spend{Tokens: 3000, Turns: 10})
	l.SyncSpend(Spend{Tokens: 9500, Turns: 5})
	st := l.State()
	if st.Spent.Tokens != 9500 || st.Spent.Turns != 10 {
		t.Fatalf("spent = %+v, want tokens raised to 9500 and turns kept at 10", st.Spent)
	}
	if st.Dispatch {
		t.Fatal("a ledger synced to 9500 of 10000 tokens still dispatches a worker")
	}
	var none *BudgetLedger
	none.SyncSpend(Spend{Tokens: 1}) // nil ledger (no budget) is a no-op
}
