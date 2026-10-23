package access

import (
	"context"
	"errors"
	"testing"
)

func TestKeysPageStableTiesAndBoundaries(t *testing.T) {
	s := contextTestStore(t)
	seedContextLists(t, s)
	all, err := s.Keys()
	if err != nil {
		t.Fatal(err)
	}
	cursor := ""
	got := []Key{}
	for i := 0; i < 10; i++ {
		page, err := s.KeysPageContext(context.Background(), 1, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) > 1 {
			t.Fatal("unbounded")
		}
		got = append(got, page.Items...)
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(got) != len(all) {
		t.Fatal(len(got), len(all))
	}
	for i := range all {
		if got[i].ID != all[i].ID {
			t.Fatal("order mismatch")
		}
	}
	for _, cursor := range []string{"!", "e30", "aaaa"} {
		if _, err := s.KeysPageContext(context.Background(), 1, cursor); !errors.Is(err, ErrInvalid) {
			t.Fatal(cursor, err)
		}
	}
	for _, limit := range []int{0, 101} {
		if _, err := s.KeysPageContext(context.Background(), limit, ""); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.KeysPageContext(ctx, 1, ""); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
