package product

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	valid := Input{SKU: " cafe-01 ", Name: " Café ", PriceMinor: 125050, Currency: "COP"}
	got, err := valid.Normalize()
	if err != nil || got.SKU != "CAFE-01" || got.Name != "Café" || got.PriceMinor != 125050 {
		t.Fatalf("unexpected normalization: %+v, %v", got, err)
	}
	for _, change := range []func(*Input){
		func(in *Input) { in.SKU = "" },
		func(in *Input) { in.SKU = "has space" },
		func(in *Input) { in.SKU = strings.Repeat("A", 65) },
		func(in *Input) { in.Name = " \t " },
		func(in *Input) { in.Name = strings.Repeat("ñ", 121) },
		func(in *Input) { in.Name = "bad\x00name" },
		func(in *Input) { in.PriceMinor = 0 },
		func(in *Input) { in.PriceMinor = -1 },
		func(in *Input) { in.PriceMinor = 1_000_000_000_001 },
		func(in *Input) { in.Currency = "USD" },
	} {
		input := valid
		change(&input)
		if _, err := input.Normalize(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted invalid input: %+v", input)
		}
	}
}

func TestRepositoryRejectsMissingScopeBeforeQuery(t *testing.T) {
	store := NewStore(nil)
	if _, err := store.Create(context.Background(), "", Input{}); err == nil {
		t.Fatal("create accepted missing merchant")
	}
	if _, err := store.Get(context.Background(), "", "11111111-1111-4111-8111-111111111111"); err == nil {
		t.Fatal("get accepted missing merchant")
	}
	if _, err := store.List(context.Background(), "", "", 50); err == nil {
		t.Fatal("list accepted missing merchant")
	}
}
