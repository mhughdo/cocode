package devexport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/db"
)

const redacted = "<redacted>"

type Dump struct {
	ExportedAt string      `json:"exported_at"`
	Tables     []TableDump `json:"tables"`
}

type TableDump struct {
	Name string           `json:"name"`
	Rows []map[string]any `json:"rows"`
}

func OpenReadOnly(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, errors.New("db path is required")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("stat db path: %w", err)
	}

	dsn := (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}).String()
	database, err := sql.Open(db.DriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open db read-only: %w", err)
	}
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping db read-only: %w", err)
	}
	return database, nil
}

func Export(ctx context.Context, database *sql.DB, exportedAt time.Time) (Dump, error) {
	if database == nil {
		return Dump{}, errors.New("db is required")
	}
	if exportedAt.IsZero() {
		exportedAt = time.Now().UTC()
	}

	tableNames, err := listExportTables(ctx, database)
	if err != nil {
		return Dump{}, err
	}

	dump := Dump{
		ExportedAt: exportedAt.UTC().Format(time.RFC3339Nano),
		Tables:     make([]TableDump, 0, len(tableNames)),
	}
	for _, tableName := range tableNames {
		rows, err := dumpTable(ctx, database, tableName)
		if err != nil {
			return Dump{}, err
		}
		dump.Tables = append(dump.Tables, TableDump{Name: tableName, Rows: rows})
	}
	return dump, nil
}

func listExportTables(ctx context.Context, database *sql.DB) ([]string, error) {
	rows, err := database.QueryContext(ctx, `
SELECT name
FROM sqlite_schema
WHERE type IN ('table', 'virtual table')
  AND name NOT LIKE 'sqlite_%'
ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		if shouldExportTable(name) {
			names = append(names, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table names: %w", err)
	}
	sort.Strings(names)
	return names, nil
}

func shouldExportTable(name string) bool {
	switch name {
	case "finding_search", "evidence_search":
		return false
	}
	for _, prefix := range []string{
		"finding_search_",
		"evidence_search_",
	} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

func dumpTable(ctx context.Context, database *sql.DB, tableName string) ([]map[string]any, error) {
	query := "SELECT * FROM " + quoteIdentifier(tableName)
	rows, err := database.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("dump table %s: %w", tableName, err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read columns for %s: %w", tableName, err)
	}

	result := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, fmt.Errorf("scan row from %s: %w", tableName, err)
		}

		row := make(map[string]any, len(columns))
		for i, column := range columns {
			row[column] = redactValue(column, normalizeSQLValue(values[i]))
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows from %s: %w", tableName, err)
	}
	return result, nil
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func normalizeSQLValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	default:
		return typed
	}
}

func redactValue(column string, value any) any {
	if value == nil {
		return nil
	}
	if isSensitiveKey(column) {
		return redacted
	}

	text, ok := value.(string)
	if !ok || !strings.HasSuffix(strings.ToLower(column), "_json") {
		return value
	}

	var parsed any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return value
	}
	return redactJSON(parsed)
}

func redactJSON(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, nested := range typed {
			if isSensitiveKey(key) {
				out[key] = redacted
				continue
			}
			out[key] = redactJSON(nested)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, nested := range typed {
			out[i] = redactJSON(nested)
		}
		return out
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(key)
	for _, marker := range []string{
		"secret",
		"token",
		"password",
		"private_key",
		"api_key",
		"storage_key",
		"credential",
	} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}
