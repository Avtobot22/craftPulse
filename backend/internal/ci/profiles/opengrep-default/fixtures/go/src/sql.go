package fixture

import (
	"database/sql"
	"fmt"
)

func findUnsafe(db *sql.DB, name string) (*sql.Rows, error) {
	return db.Query(fmt.Sprintf("SELECT id FROM users WHERE name = '%s'", name))
}

func findSafe(db *sql.DB, name string) (*sql.Rows, error) {
	return db.Query("SELECT id FROM users WHERE name = $1", name)
}
