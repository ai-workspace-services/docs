package dbprobe

import (
	"context"
	"database/sql"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Ping checks the configured primary without keeping a second database pool
// alive. Content does not currently read metadata from PostgreSQL, so a
// readiness request opens one short-lived connection only when a runtime URI
// has explicitly been configured.
func Ping(ctx context.Context, uri string) error {
	if strings.TrimSpace(uri) == "" {
		return nil
	}
	db, err := sql.Open("pgx", uri)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	return db.PingContext(ctx)
}
