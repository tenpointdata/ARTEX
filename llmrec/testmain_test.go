package llmrec

import (
	"os"
	"testing"

	"github.com/Autumn-27/artex/db"
)

// Run against this package's own database (db.SuffixTestDatabase, #80): the
// recorder tests require an isolated ARTEX_PG_DSN, and the suffix gives them
// one even inside a full parallel ./... run.
func TestMain(m *testing.M) {
	db.SuffixTestDatabase("llmrec")
	// LLM record bodies are sealed at rest: load a fixed test key (env, no key file).
	if dsn, _, err := db.DSN(); err == nil {
		if pg, err := db.Open(dsn); err == nil {
			os.Setenv("ARTEX_SECRET_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
			if err := pg.InitSecrets(""); err != nil {
				panic(err)
			}
			pg.Close()
		}
	}
	os.Exit(m.Run())
}
