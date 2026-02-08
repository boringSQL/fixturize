package fixturize

import "testing"

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
	t.Run("nil returns nil", func(t *testing.T) {
		got := convertValue(nil, nil)
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("int64 passthrough", func(t *testing.T) {
		got := convertValue(int64(42), nil)
		if got != int64(42) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("int32 passthrough", func(t *testing.T) {
		got := convertValue(int32(-7), nil)
		if got != int32(-7) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("float64 passthrough", func(t *testing.T) {
		got := convertValue(float64(3.14), nil)
		if got != float64(3.14) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("string passthrough", func(t *testing.T) {
		got := convertValue("hello", nil)
		if got != "hello" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("bool passthrough", func(t *testing.T) {
		got := convertValue(true, nil)
		if got != true {
			t.Errorf("got %v", got)
		}
	})

	t.Run("[16]byte as UUID", func(t *testing.T) {
		val := [16]byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00}
		got := convertValue(val, nil)
		want := "550e8400-e29b-41d4-a716-446655440000"
		if got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("unknown type uses Sprintf", func(t *testing.T) {
		type custom struct{ X int }
		got := convertValue(custom{X: 1}, nil)
		if got != "{1}" {
			t.Errorf("got %v, want '{1}'", got)
		}
	})
}
