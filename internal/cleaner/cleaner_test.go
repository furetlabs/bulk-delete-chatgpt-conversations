package cleaner

import (
	"context"
	"errors"
	"testing"
)

func TestPreviewExcludesProtectedAndUnknown(t *testing.T) {
	items := []Item{{ID: "a", Title: "A"}, {ID: "b", Title: "B", Protected: true}, {Title: "unknown"}}
	got := Preview(items, nil, true)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("unexpected preview: %+v", got)
	}
}
func TestCancelStopsBeforeNext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	Run(ctx, []Item{{ID: "a"}, {ID: "b"}}, func(ctx context.Context, id string) error { n++; cancel(); return nil }, nil)
	if n != 1 {
		t.Fatalf("executed %d", n)
	}
}

func TestRunStopsOnBackendError(t *testing.T) {
	calls := 0
	results := Run(context.Background(), []Item{{ID: "a"}, {ID: "b"}}, func(context.Context, string) error { calls++; return errors.New("network failure") }, nil)
	if calls != 1 || len(results) != 1 || results[0].Err == nil {
		t.Fatalf("error was not preserved, calls %d", calls)
	}
}
