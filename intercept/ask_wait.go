package intercept

import (
	"context"
	"time"

	"github.com/Autumn-27/artex/db"
)

// askDecided is the way an ask wait ends when the row carries a decision.
const askDecided = "decided"

// awaitAsk waits for the decision on a classic ask row. It returns the
// decision and askDecided, or false and the way the wait ended without one
// (waitRuleTimeout, waitCancelled, waitGone). It never settles the row.
//
// A decision taken in this process arrives on ch at once. One taken in another
// process that shares the database (a worker's ask decided on the control node)
// never reaches ch, so the row is also re-read every targetPollInterval. A read
// error is not a decision: the wait goes on, and only a decided row, the
// timeout or the context ends it. A nil timeout never fires.
func (i *Interceptor) awaitAsk(ctx context.Context, id int64, ch <-chan bool, timeout <-chan time.Time) (bool, string) {
	poll := time.NewTicker(targetPollInterval)
	defer poll.Stop()
	for {
		select {
		case allowed := <-ch:
			return allowed, askDecided
		case <-poll.C:
			row, err := i.db.GetInterceptDetail(id)
			if err != nil {
				continue
			}
			if row == nil {
				return false, waitGone
			}
			if row.Status != "pending" {
				return askAllowed(row), askDecided
			}
		case <-timeout:
			return false, waitRuleTimeout
		case <-ctx.Done():
			return false, waitCancelled
		}
	}
}

// askAllowed reports whether a decided ask row lets the call run: approved, or
// settled by a policy whose effective action is allow (a timeout with
// TimeoutAction allow).
func askAllowed(row *db.InterceptDetail) bool {
	return row.Status == "allowed" || (row.Audit != nil && row.Audit.EffectiveAction == "allow")
}
