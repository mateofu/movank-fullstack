package sale

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCanonicalRequest(t *testing.T) {
	a := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	b := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	original := Input{Items: []Line{{b, 2}, {strings.ToUpper(a), 1}}}
	_, first, err := original.normalize()
	if err != nil {
		t.Fatal(err)
	}
	_, second, _ := (Input{Items: []Line{{a, 1}, {b, 2}}}).normalize()
	if first != second {
		t.Fatal("equivalent carts have different hashes")
	}
	_, changed, _ := (Input{Items: []Line{{a, 2}, {b, 2}}}).normalize()
	if first == changed {
		t.Fatal("quantity change was ignored")
	}
	if original.Items[0].ProductID != b {
		t.Fatal("caller input mutated")
	}
}

func TestInvalidRequestBeforeDatabase(t *testing.T) {
	id := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	for _, input := range []Input{
		{}, {Items: []Line{{id, 0}}}, {Items: []Line{{id, 10001}}},
		{Items: []Line{{"invalid", 1}}}, {Items: []Line{{id, 1}, {strings.ToUpper(id), 1}}},
		{Items: make([]Line, 101)},
	} {
		if _, err := NewStore(nil).Create(context.Background(), id, "key", input); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected invalid: %v", err)
		}
	}
	for _, key := range []string{"", "contains space", strings.Repeat("a", 129)} {
		if _, err := NewStore(nil).Create(context.Background(), id, key, Input{Items: []Line{{id, 1}}}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted key %q", key)
		}
	}
}
