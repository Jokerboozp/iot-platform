package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Set IOT_TEST_POSTGRES_DSN to run the migration against a disposable PostgreSQL database.
func TestMigrateLegacyAIAnalysisTenantOwnership(t *testing.T) {
	dsn := os.Getenv("IOT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("IOT_TEST_POSTGRES_DSN is not configured")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()

	schemaName := fmt.Sprintf("migration_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schemaName}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE") }()

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schemaName
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	legacy := `
CREATE TABLE alarm_record (
  tenant_id text NOT NULL, id text NOT NULL, rule_id text NOT NULL, device_id text NOT NULL,
  status text NOT NULL, level text NOT NULL, source text NOT NULL,
  last_triggered_at bigint NOT NULL, body jsonb NOT NULL,
  PRIMARY KEY (tenant_id, id)
);
CREATE TABLE alarm_ai_analysis (
  alarm_id text PRIMARY KEY, body jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO alarm_record(tenant_id,id,rule_id,device_id,status,level,source,last_triggered_at,body) VALUES
  ('tenant_a','alarm_a','rule','device_a','ACTIVE','HIGH','iot',1,'{}'),
  ('tenant_b','alarm_b','rule','device_b','ACTIVE','HIGH','iot',1,'{}'),
  ('tenant_a','alarm_ambiguous','rule','device_c','ACTIVE','HIGH','iot',1,'{}'),
  ('tenant_b','alarm_ambiguous','rule','device_d','ACTIVE','HIGH','iot',1,'{}');
INSERT INTO alarm_ai_analysis(alarm_id,body) VALUES
  ('alarm_a','{"alarmId":"alarm_a"}'),
  ('alarm_b','{"alarmId":"alarm_b"}'),
  ('alarm_ambiguous','{"alarmId":"alarm_ambiguous"}'),
  ('alarm_orphan','{"alarmId":"alarm_orphan"}');`
	if _, err = pool.Exec(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if err = (&Repository{pool: pool}).Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	for alarmID, wantTenant := range map[string]string{
		"alarm_a":         "tenant_a",
		"alarm_b":         "tenant_b",
		"alarm_ambiguous": "__legacy_orphaned__",
		"alarm_orphan":    "__legacy_orphaned__",
	} {
		var got string
		if err = pool.QueryRow(ctx, `SELECT tenant_id FROM alarm_ai_analysis WHERE alarm_id=$1`, alarmID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != wantTenant {
			t.Fatalf("alarm %s migrated to tenant %q, want %q", alarmID, got, wantTenant)
		}
	}
	if _, err = pool.Exec(ctx, `INSERT INTO alarm_ai_analysis(tenant_id,alarm_id,body) VALUES('tenant_b','alarm_a','{}')`); err != nil {
		t.Fatalf("composite tenant/alarm primary key was not installed: %v", err)
	}
}
