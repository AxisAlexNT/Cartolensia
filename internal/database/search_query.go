package database

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type ReadOnlyQueryResult struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	Limit   int              `json:"limit"`
	Count   int              `json:"count"`
	SQL     string           `json:"sql"`
	Views   []string         `json:"views"`
	Note    string           `json:"note"`
}

var (
	errUnsafeReadOnlyQuery = errors.New("query must be a single read-only SELECT against cartolensia_search_* views")
	fromJoinRE             = regexp.MustCompile(`(?i)\b(?:from|join)\s+([a-zA-Z_][a-zA-Z0-9_\.]*)`)
	blockedSQLRE           = regexp.MustCompile(`(?i)\b(insert|update|delete|merge|drop|alter|truncate|create|grant|revoke|copy|call|do|execute|vacuum|analyze|refresh|reindex|cluster|listen|notify|lock|set|reset)\b`)
)

var allowedReadOnlySearchViews = map[string]struct{}{
	"cartolensia_search_assets":              {},
	"cartolensia_search_ai_predictions":      {},
	"cartolensia_search_tags":                {},
	"cartolensia_search_transcripts":         {},
	"cartolensia_search_transcript_segments": {},
	"cartolensia_search_documents":           {},
	"cartolensia_search_video_captions":      {},
	"cartolensia_search_audio_features":      {},
	"cartolensia_search_tracks":              {},
	"cartolensia_search_places":              {},
	"cartolensia_search_knowledge_facts":     {},
	"cartolensia_search_knowledge_relations": {},
}

func (db *DB) ReadOnlySearchQuery(ctx context.Context, rawSQL string, limit int) (ReadOnlyQueryResult, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	normalized, views, err := validateReadOnlySearchSQL(rawSQL)
	if err != nil {
		return ReadOnlyQueryResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := db.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return ReadOnlyQueryResult{}, err
	}
	defer rollback(ctx, tx)
	if _, err := tx.Exec(ctx, `set local statement_timeout = '7000ms'`); err != nil {
		return ReadOnlyQueryResult{}, err
	}
	rows, err := tx.Query(ctx, `select * from (`+normalized+`) cartolensia_readonly_query limit $1`, limit)
	if err != nil {
		return ReadOnlyQueryResult{}, err
	}
	defer rows.Close()
	fields := rows.FieldDescriptions()
	columns := make([]string, 0, len(fields))
	for _, field := range fields {
		columns = append(columns, string(field.Name))
	}
	outRows := []map[string]any{}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return ReadOnlyQueryResult{}, err
		}
		row := make(map[string]any, len(columns))
		for i, column := range columns {
			row[column] = queryJSONValue(values[i])
		}
		outRows = append(outRows, row)
	}
	if err := rows.Err(); err != nil {
		return ReadOnlyQueryResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReadOnlyQueryResult{}, err
	}
	return ReadOnlyQueryResult{
		Columns: columns,
		Rows:    outRows,
		Limit:   limit,
		Count:   len(outRows),
		SQL:     normalized,
		Views:   views,
		Note:    "read-only transaction; query wrapped with a server-side row limit; only cartolensia_search_* views are allowed",
	}, nil
}

func validateReadOnlySearchSQL(rawSQL string) (string, []string, error) {
	query := strings.TrimSpace(rawSQL)
	if query == "" {
		return "", nil, fmt.Errorf("%w: empty query", errUnsafeReadOnlyQuery)
	}
	if strings.Contains(query, ";") || strings.Contains(query, "--") || strings.Contains(query, "/*") || strings.Contains(query, "*/") {
		return "", nil, fmt.Errorf("%w: comments and semicolons are not allowed", errUnsafeReadOnlyQuery)
	}
	lower := strings.ToLower(query)
	if !strings.HasPrefix(lower, "select ") {
		return "", nil, fmt.Errorf("%w: only SELECT is allowed", errUnsafeReadOnlyQuery)
	}
	if blockedSQLRE.MatchString(lower) {
		return "", nil, fmt.Errorf("%w: mutation or session-control keyword found", errUnsafeReadOnlyQuery)
	}
	matches := fromJoinRE.FindAllStringSubmatch(lower, -1)
	if len(matches) == 0 {
		return "", nil, fmt.Errorf("%w: at least one FROM view is required", errUnsafeReadOnlyQuery)
	}
	views := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		view := strings.Trim(match[1], `"`)
		if strings.Contains(view, ".") {
			parts := strings.Split(view, ".")
			view = parts[len(parts)-1]
		}
		if _, ok := allowedReadOnlySearchViews[view]; !ok {
			return "", nil, fmt.Errorf("%w: %s is not an allowed search view", errUnsafeReadOnlyQuery, view)
		}
		if _, ok := seen[view]; !ok {
			views = append(views, view)
			seen[view] = struct{}{}
		}
	}
	return query, views, nil
}

func queryJSONValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	default:
		return typed
	}
}
