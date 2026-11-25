package intercept

import (
	"os"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

func TestMain(m *testing.M) {
	// Own database per package (#80), as in db, agent, server and evidence.
	db.SuffixTestDatabase("intercept")
	// A short poll keeps the cross-instance tests fast; production uses 2 s.
	targetPollInterval = 50 * time.Millisecond
	// Check the rules revision on every read, so a change made through a
	// second Interceptor is seen on the next call (rules_cache.go).
	rulesRevisionTTL = 0
	os.Exit(m.Run())
}
