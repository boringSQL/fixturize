package fixturize

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func OpenDB(ctx context.Context, pguri string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(pguri)
	if err != nil {
		return nil, err
	}
	// Preserve Postgres's text representation for JSON/JSONB. The default pgx
	// codec decodes into Go map/slice, which forces us to re-marshal — losing
	// key order and the spaces PG emits. A passthrough Unmarshal that stores
	// raw bytes keeps the wire text intact for rows.Values().
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		rawJSON := &pgtype.JSONCodec{
			Marshal: json.Marshal,
			Unmarshal: func(data []byte, v any) error {
				if p, ok := v.(*any); ok {
					*p = append([]byte(nil), data...)
					return nil
				}
				return json.Unmarshal(data, v)
			},
		}
		m := conn.TypeMap()
		m.RegisterType(&pgtype.Type{Codec: rawJSON, Name: "json", OID: pgtype.JSONOID})
		m.RegisterType(&pgtype.Type{Codec: rawJSON, Name: "jsonb", OID: pgtype.JSONBOID})
		return nil
	}
	return pgxpool.NewWithConfig(ctx, config)
}
