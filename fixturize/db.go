package fixturize

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func OpenDB(ctx context.Context, pguri string) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, pguri)
}
