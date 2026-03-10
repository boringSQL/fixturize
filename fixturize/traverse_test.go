package fixturize

import (
	"testing"
)

func TestBuildTupleQuery(t *testing.T) {
	cols := []string{`"workspace_id"`, `"task_id"`}
	tuples := [][]any{
		{1, 10},
		{1, 20},
		{2, 30},
	}

	query, params := buildTupleQuery("SELECT * FROM comments", cols, tuples)

	wantQuery := `SELECT * FROM comments WHERE (("workspace_id" = $1 AND "task_id" = $2) OR ("workspace_id" = $3 AND "task_id" = $4) OR ("workspace_id" = $5 AND "task_id" = $6))`
	if query != wantQuery {
		t.Errorf("query =\n  %s\nwant\n  %s", query, wantQuery)
	}

	if len(params) != 6 {
		t.Fatalf("got %d params, want 6", len(params))
	}
	wantParams := []any{1, 10, 1, 20, 2, 30}
	for i, p := range params {
		if p != wantParams[i] {
			t.Errorf("params[%d] = %v, want %v", i, p, wantParams[i])
		}
	}
}

func TestBuildTupleQuerySingleTuple(t *testing.T) {
	cols := []string{`"id"`}
	tuples := [][]any{{42}}

	query, params := buildTupleQuery("SELECT * FROM t", cols, tuples)

	wantQuery := `SELECT * FROM t WHERE (("id" = $1))`
	if query != wantQuery {
		t.Errorf("query = %s, want %s", query, wantQuery)
	}
	if len(params) != 1 || params[0] != 42 {
		t.Errorf("params = %v, want [42]", params)
	}
}

func TestUniqueValues(t *testing.T) {
	vals := []any{1, 2, 3, 2, 1, 4}
	result := uniqueValues(vals)

	if len(result) != 4 {
		t.Fatalf("got %d values, want 4", len(result))
	}
	// should preserve order of first occurrence
	want := []any{1, 2, 3, 4}
	for i, v := range result {
		if v != want[i] {
			t.Errorf("result[%d] = %v, want %v", i, v, want[i])
		}
	}
}

func TestUniqueValuesEmpty(t *testing.T) {
	result := uniqueValues(nil)
	if len(result) != 0 {
		t.Errorf("got %d values, want 0", len(result))
	}
}

func TestUniqueTuples(t *testing.T) {
	tuples := [][]any{
		{1, "a"},
		{2, "b"},
		{1, "a"}, // duplicate
		{3, "c"},
	}

	result := uniqueTuples(tuples)
	if len(result) != 3 {
		t.Fatalf("got %d tuples, want 3", len(result))
	}
}

func TestUniqueTuplesEmpty(t *testing.T) {
	result := uniqueTuples(nil)
	if len(result) != 0 {
		t.Errorf("got %d tuples, want 0", len(result))
	}
}
