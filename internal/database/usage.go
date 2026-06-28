package database

import (
	"context"
)

type RelationUsage struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	Rows      int64  `json:"rows"`
}

type Usage struct {
	DatabaseName string          `json:"database_name"`
	SizeBytes    int64           `json:"size_bytes"`
	Relations    []RelationUsage `json:"relations"`
}

func (db *DB) DatabaseUsage(ctx context.Context) (Usage, error) {
	var usage Usage
	if err := db.pool.QueryRow(ctx, `select current_database(), pg_database_size(current_database())`).Scan(&usage.DatabaseName, &usage.SizeBytes); err != nil {
		return usage, err
	}
	rows, err := db.pool.Query(ctx, `
		select relid::regclass::text as name,
		       pg_total_relation_size(relid) as size_bytes,
		       coalesce(n_live_tup, 0)::bigint as rows
		from pg_stat_user_tables
		order by pg_total_relation_size(relid) desc
		limit 16
	`)
	if err != nil {
		return usage, err
	}
	defer rows.Close()
	for rows.Next() {
		var relation RelationUsage
		if err := rows.Scan(&relation.Name, &relation.SizeBytes, &relation.Rows); err != nil {
			return usage, err
		}
		usage.Relations = append(usage.Relations, relation)
	}
	return usage, rows.Err()
}
