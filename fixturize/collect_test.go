package fixturize

// Unit tests for collect.go's value-formatting helpers. These run without a
// database — they pin down the conversion-from-driver-value-to-fixture-value
// behaviour that integration tests cover only indirectly.
//
// After the pgx native migration, convertValue's second parameter switched
// from *sql.ColumnType (a pointer that can be nil for "type unknown") to
// uint32 (a PostgreSQL type OID; zero means "no OID hint, fall through to
// type-switch fallback"). The tests below pass 0 as a stand-in for the old
// nil — semantically identical from the caller's perspective: no type-driven
// formatting, just rely on the value's Go type.

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestFormatUUID(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{
			"standard UUID",
			[]byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00},
			"550e8400-e29b-41d4-a716-446655440000",
		},
		{
			"all zeros",
			[]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			"00000000-0000-0000-0000-000000000000",
		},
		{
			"all ones",
			[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			"ffffffff-ffff-ffff-ffff-ffffffffffff",
		},
		{
			"non-16 bytes falls back to hex",
			[]byte{0xab, 0xcd},
			"abcd",
		},
		{
			"empty falls back to hex",
			[]byte{},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatUUID(tt.in)
			if got != tt.want {
				t.Errorf("formatUUID(%x) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildPKKey(t *testing.T) {
	t.Run("single PK", func(t *testing.T) {
		row := map[string]any{"id": int64(42), "name": "test"}
		got := buildPKKey(row, []string{"id"})
		if got != int64(42) {
			t.Errorf("got %v, want 42", got)
		}
	})

	t.Run("composite PK", func(t *testing.T) {
		row := map[string]any{"tenant_id": int64(1), "user_id": int64(2), "name": "test"}
		got := buildPKKey(row, []string{"tenant_id", "user_id"})
		if got != "1|2" {
			t.Errorf("got %v, want '1|2'", got)
		}
	})

	t.Run("composite PK with string", func(t *testing.T) {
		row := map[string]any{"region": "us-east", "id": int64(5)}
		got := buildPKKey(row, []string{"region", "id"})
		if got != "us-east|5" {
			t.Errorf("got %v, want 'us-east|5'", got)
		}
	})

	t.Run("single PK nil value", func(t *testing.T) {
		row := map[string]any{"id": nil}
		got := buildPKKey(row, []string{"id"})
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

func TestConvertValue(t *testing.T) {
	// OID 0 means "no type hint" — convertValue's OID switch will miss every
	// case and the fallback type-switch on the Go value decides the output.
	// That mirrors the pre-migration behaviour of passing a nil *sql.ColumnType.
	const noOID uint32 = 0

	t.Run("nil returns nil", func(t *testing.T) {
		got := convertValue(nil, noOID)
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("int64 passthrough", func(t *testing.T) {
		got := convertValue(int64(42), noOID)
		if got != int64(42) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("int32 passthrough", func(t *testing.T) {
		got := convertValue(int32(-7), noOID)
		if got != int32(-7) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("float64 passthrough", func(t *testing.T) {
		got := convertValue(float64(3.14), noOID)
		if got != float64(3.14) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("string passthrough", func(t *testing.T) {
		got := convertValue("hello", noOID)
		if got != "hello" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("bool passthrough", func(t *testing.T) {
		got := convertValue(true, noOID)
		if got != true {
			t.Errorf("got %v", got)
		}
	})

	t.Run("[16]byte as UUID via fallback", func(t *testing.T) {
		// Even without the UUIDOID hint, the Go-type-switch fallback recognises
		// [16]byte and formats it as a canonical UUID string. This covers the
		// case where pgx returns a UUID value without surfacing the OID
		// (rare, but it can happen with bespoke type registrations).
		val := [16]byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00}
		got := convertValue(val, noOID)
		want := "550e8400-e29b-41d4-a716-446655440000"
		if got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("[16]byte with UUIDOID", func(t *testing.T) {
		// With the explicit OID hint, the OID branch fires before the type
		// switch — same observable result here, but it's the code path that
		// gets exercised in production for any uuid-typed column.
		val := [16]byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00}
		got := convertValue(val, pgtype.UUIDOID)
		want := "550e8400-e29b-41d4-a716-446655440000"
		if got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("[]byte with ByteaOID", func(t *testing.T) {
		// Bytea is hex-encoded with the Postgres '\x' prefix so it round-trips
		// cleanly into an INSERT statement.
		got := convertValue([]byte{0xde, 0xad, 0xbe, 0xef}, pgtype.ByteaOID)
		want := `\xdeadbeef`
		if got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("[]byte with JSONBOID becomes string", func(t *testing.T) {
		// pgx returns JSON/JSONB as []byte; we widen to string so it stays
		// human-readable in the fixture.
		got := convertValue([]byte(`{"k":"v"}`), pgtype.JSONBOID)
		if got != `{"k":"v"}` {
			t.Errorf("got %v, want JSON string", got)
		}
	})

	t.Run("unknown type uses Sprintf", func(t *testing.T) {
		type custom struct{ X int }
		got := convertValue(custom{X: 1}, noOID)
		if got != "{1}" {
			t.Errorf("got %v, want '{1}'", got)
		}
	})
}
