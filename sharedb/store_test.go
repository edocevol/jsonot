package sharedb

import (
	"context"
	"encoding/json"
	"testing"
)

func TestStoreSequentialAndRebasedSubmit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-1", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	left := json.RawMessage(`[{"p":["counter"],"na":1}]`)
	right := json.RawMessage(`[{"p":["counter"],"na":2}]`)

	leftResult, err := store.Submit(ctx, "doc-1", 0, left, "left-client")
	if err != nil {
		t.Fatalf("submit left failed: %v", err)
	}
	if leftResult.Version != 1 {
		t.Fatalf("unexpected left version: got %d want 1", leftResult.Version)
	}
	if leftResult.Rebased {
		t.Fatalf("left operation should not be rebased")
	}

	rightResult, err := store.Submit(ctx, "doc-1", 0, right, "right-client")
	if err != nil {
		t.Fatalf("submit right failed: %v", err)
	}
	if rightResult.Version != 2 {
		t.Fatalf("unexpected right version: got %d want 2", rightResult.Version)
	}
	if !rightResult.Rebased {
		t.Fatalf("right operation should be rebased")
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-1")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}

	if got := string(snapshot.Document); got != `{"counter":3}` {
		t.Fatalf("unexpected final document: got %s want %s", got, `{"counter":3}`)
	}
}

func TestStoreSubscribe(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-2", json.RawMessage(`{"title":"hello"}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	events, cancel, err := store.Subscribe(ctx, "doc-2", 1)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer cancel()

	op := json.RawMessage(`[{"p":["title"],"od":"hello","oi":"world"}]`)
	_, err = store.Submit(ctx, "doc-2", 0, op, "client-a")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	select {
	case event := <-events:
		if event.DocumentID != "doc-2" {
			t.Fatalf("unexpected document id: %s", event.DocumentID)
		}
		if event.Version != 1 {
			t.Fatalf("unexpected version: got %d want 1", event.Version)
		}
		if event.Source != "client-a" {
			t.Fatalf("unexpected source: %s", event.Source)
		}
		if got := string(event.Document); got != `{"title":"world"}` {
			t.Fatalf("unexpected document: got %s", got)
		}
	default:
		t.Fatalf("expected one event")
	}
}

func TestStoreInvalidVersion(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-3", json.RawMessage(`{"ok":true}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	_, err = store.Submit(ctx, "doc-3", 5, json.RawMessage(`[]`), "")
	if err == nil {
		t.Fatalf("expected invalid version error")
	}
}
