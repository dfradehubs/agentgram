package db

import (
	"strings"
	"testing"
)

func TestConvertPostgresToSQLite(t *testing.T) {
	in := `CREATE TABLE t (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), n JSONB, ts TIMESTAMPTZ NOT NULL DEFAULT NOW(), ok BOOLEAN DEFAULT TRUE);`
	out := convertPostgresToSQLite(in)
	for _, token := range []string{"JSONB", "TIMESTAMPTZ", "gen_random_uuid"} {
		if strings.Contains(out, token) {
			t.Fatalf("postgres token %q remains in %q", token, out)
		}
	}
	for _, want := range []string{"TEXT", "DATETIME", "CURRENT_TIMESTAMP"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}
