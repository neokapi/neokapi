package store

import "strconv"

// Dialect identifies the SQL database backend.
type Dialect int

const (
	DialectSQLite   Dialect = iota
	DialectPostgres
)

// Rebind converts ?-placeholder SQL to $N-placeholder SQL for PostgreSQL.
// For SQLite, it returns the query unchanged.
func Rebind(dialect Dialect, query string) string {
	if dialect == DialectSQLite {
		return query
	}

	var out []byte
	n := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			out = append(out, '$')
			out = append(out, []byte(strconv.Itoa(n))...)
			n++
		} else {
			out = append(out, query[i])
		}
	}
	return string(out)
}
