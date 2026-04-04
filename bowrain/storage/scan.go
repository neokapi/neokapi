package storage

import "database/sql"

// Scanner is the interface satisfied by *sql.Row and *sql.Rows.
type Scanner interface {
	Scan(dest ...any) error
}

// ScanRows iterates rows, applies the scan function to each row, and returns
// the collected results. It handles rows.Close() via defer and checks
// rows.Err() after iteration completes.
//
// This generic helper replaces the repetitive pattern of:
//
//	defer rows.Close()
//	var result []*T
//	for rows.Next() {
//	    item, err := scanT(rows)
//	    ...
//	    result = append(result, item)
//	}
//	return result, rows.Err()
func ScanRows[T any](rows *sql.Rows, scan func(Scanner) (T, error)) ([]T, error) {
	defer rows.Close()
	var result []T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
