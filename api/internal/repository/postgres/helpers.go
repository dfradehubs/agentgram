package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// insertPermissions inserts permission rows within a transaction
func insertPermissions(ctx context.Context, tx pgx.Tx, table, fkCol, fkVal, valueCol string, values []string) error {
	for _, v := range values {
		_, err := tx.Exec(ctx,
			fmt.Sprintf(`INSERT INTO %s (%s, %s) VALUES ($1, $2) ON CONFLICT DO NOTHING`, table, fkCol, valueCol),
			fkVal, v,
		)
		if err != nil {
			return fmt.Errorf("insert %s permission: %w", table, err)
		}
	}
	return nil
}

// queryStrings queries a single string column and returns results as a slice
func queryStrings(ctx context.Context, pool *pgxpool.Pool, query string, args ...interface{}) ([]string, error) {
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, nil
}
