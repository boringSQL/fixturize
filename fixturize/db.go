package fixturize

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func OpenDB(pguri string) (*sql.DB, error) {
	return sql.Open("pgx", pguri)
}
