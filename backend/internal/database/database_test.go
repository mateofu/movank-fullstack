package database

import (
	"context"
	"testing"
)

func TestMissingConfiguration(t *testing.T) {
	for _, name := range []string{"PGHOST", "PGDATABASE", "PGUSER", "PGPASSWORD", "PGSSLMODE"} {
		t.Run(name, func(t *testing.T) {
			for _, key := range []string{"PGHOST", "PGDATABASE", "PGUSER", "PGPASSWORD", "PGSSLMODE"} {
				t.Setenv(key, "test")
			}
			t.Setenv(name, "")
			pool, err := Open(context.Background())
			if err == nil || pool != nil || err.Error() != name+" is required" {
				t.Fatalf("expected missing %s error; got %v", name, err)
			}
		})
	}
}

func TestInvalidConfigurationDoesNotExposeCredentials(t *testing.T) {
	t.Setenv("PGHOST", "localhost")
	t.Setenv("PGDATABASE", "movank")
	t.Setenv("PGUSER", "movank")
	t.Setenv("PGPASSWORD", "private-test-value")
	t.Setenv("PGSSLMODE", "invalid")
	pool, err := Open(context.Background())
	if err == nil || pool != nil || err.Error() != "invalid PostgreSQL configuration" {
		t.Fatalf("expected sanitized configuration error; got %v", err)
	}
}
