package sharedb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func sameJSON(t *testing.T, got, want json.RawMessage) bool {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("got invalid JSON %q: %v", got, err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("want invalid JSON %q: %v", want, err)
	}
	return reflect.DeepEqual(gotValue, wantValue)
}

func TestSubmitDoesNotAdvanceSnapshotWhenCommitFails(t *testing.T) {
	ctx := context.Background()
	backend := &failingAppendBackend{MemoryBackend: NewMemoryBackend(), err: errors.New("append failed")}
	store := NewServer(backend, NewMemoryLocker())

	_, err := store.CreateDocument(ctx, "doc-atomic", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	_, err = store.Submit(ctx, "doc-atomic", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "client-a")
	if err == nil {
		t.Fatalf("expected submit to fail")
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-atomic")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if snapshot.Version != 0 {
		t.Fatalf("snapshot version advanced despite failed commit: got %d want 0", snapshot.Version)
	}
	if got := string(snapshot.Document); got != `{"counter":0}` {
		t.Fatalf("snapshot document changed despite failed commit: got %s", got)
	}

	ops, err := store.GetOperations(ctx, "doc-atomic", 0, 0)
	if err != nil {
		t.Fatalf("get operations failed: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("unexpected committed ops after failed commit: got %d", len(ops))
	}
}

type failingAppendBackend struct {
	*MemoryBackend
	err error
}

func (b *failingAppendBackend) AppendOp(context.Context, OpRecord) error {
	return b.err
}

func (b *failingAppendBackend) CommitOp(context.Context, DocRecord, OpRecord) error {
	return b.err
}

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
		if event.Type != EventTypeOp {
			t.Fatalf("unexpected event type: got %q want %q", event.Type, EventTypeOp)
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

func TestDeleteDocumentRemovesSnapshotAndPublishesDelete(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-delete", json.RawMessage(`{"title":"bye"}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	events, cancel, err := store.Subscribe(ctx, "doc-delete", 1)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer cancel()

	if err := store.DeleteDocument(ctx, "doc-delete", 0, "client-a"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := store.GetSnapshot(ctx, "doc-delete"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("expected document not found after delete, got %v", err)
	}
	if _, _, err := store.Subscribe(ctx, "doc-delete", 1); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("expected subscribe on deleted doc to fail with not found, got %v", err)
	}

	select {
	case event := <-events:
		if event.Type != EventTypeDelete {
			t.Fatalf("unexpected event type: got %q want %q", event.Type, EventTypeDelete)
		}
		if event.DocumentID != "doc-delete" || event.Version != 0 || event.Source != "client-a" {
			t.Fatalf("unexpected delete event: %+v", event)
		}
	default:
		t.Fatalf("expected delete event")
	}
}

func TestDeleteDocumentRejectsStaleVersion(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-delete-version", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	if _, err := store.Submit(ctx, "doc-delete-version", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "client-a"); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	err = store.DeleteDocument(ctx, "doc-delete-version", 0, "client-a")
	if !errors.Is(err, ErrInvalidVersion) {
		t.Fatalf("expected invalid version error, got %v", err)
	}
	snapshot, err := store.GetSnapshot(ctx, "doc-delete-version")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if snapshot.Version != 1 {
		t.Fatalf("stale delete changed snapshot version: got %d", snapshot.Version)
	}
}

func TestStoreGetOperationsReturnsCommittedHistoryRange(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-history", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	first := json.RawMessage(`[{"p":["counter"],"na":1}]`)
	second := json.RawMessage(`[{"p":["counter"],"na":2}]`)
	third := json.RawMessage(`[{"p":["counter"],"na":3}]`)

	if _, err := store.SubmitWithRequest(ctx, SubmitRequest{DocumentID: "doc-history", BaseVersion: 0, Operation: first, Source: "a", Sequence: 1}); err != nil {
		t.Fatalf("submit first failed: %v", err)
	}
	if _, err := store.SubmitWithRequest(ctx, SubmitRequest{DocumentID: "doc-history", BaseVersion: 1, Operation: second, Source: "b", Sequence: 2}); err != nil {
		t.Fatalf("submit second failed: %v", err)
	}
	if _, err := store.SubmitWithRequest(ctx, SubmitRequest{DocumentID: "doc-history", BaseVersion: 2, Operation: third, Source: "c", Sequence: 3}); err != nil {
		t.Fatalf("submit third failed: %v", err)
	}

	ops, err := store.GetOperations(ctx, "doc-history", 1, 3)
	if err != nil {
		t.Fatalf("get operations failed: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("unexpected operation count: got %d want 2", len(ops))
	}
	if ops[0].Version != 2 || ops[0].BaseVersion != 1 || ops[0].Source != "b" || ops[0].ID.Source != "b" || ops[0].ID.Sequence != 2 || !sameJSON(t, ops[0].Op, second) || !sameJSON(t, ops[0].SubmittedOp, second) {
		t.Fatalf("unexpected first history op: %+v", ops[0])
	}
	if ops[1].Version != 3 || ops[1].BaseVersion != 2 || ops[1].Source != "c" || ops[1].ID.Source != "c" || ops[1].ID.Sequence != 3 || !sameJSON(t, ops[1].Op, third) || !sameJSON(t, ops[1].SubmittedOp, third) {
		t.Fatalf("unexpected second history op: %+v", ops[1])
	}
}

func TestSubmitWithRequestDeduplicatesClientSequence(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-dedupe", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	req := SubmitRequest{
		DocumentID:  "doc-dedupe",
		BaseVersion: 0,
		Operation:   json.RawMessage(`[{"p":["counter"],"na":1}]`),
		Source:      "client-a",
		Sequence:    7,
	}

	first, err := store.SubmitWithRequest(ctx, req)
	if err != nil {
		t.Fatalf("first submit failed: %v", err)
	}
	if first.Version != 1 || first.Duplicate {
		t.Fatalf("unexpected first result: %+v", first)
	}

	if _, err := store.Submit(ctx, "doc-dedupe", 1, json.RawMessage(`[{"p":["counter"],"na":2}]`), "client-b"); err != nil {
		t.Fatalf("interleaved submit failed: %v", err)
	}

	retry, err := store.SubmitWithRequest(ctx, req)
	if err != nil {
		t.Fatalf("retry submit failed: %v", err)
	}
	if retry.Version != 2 || !retry.Duplicate {
		t.Fatalf("unexpected retry result: %+v", retry)
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-dedupe")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if got := string(snapshot.Document); got != `{"counter":3}` {
		t.Fatalf("duplicate submit should not apply twice: got %s", got)
	}
	if !sameJSON(t, retry.Document, snapshot.Document) {
		t.Fatalf("duplicate result should return the current snapshot document")
	}
}

func TestSubmitWithRequestRejectsSequenceReuseWithDifferentOperation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-seq-conflict", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	first := SubmitRequest{
		DocumentID:  "doc-seq-conflict",
		BaseVersion: 0,
		Operation:   json.RawMessage(`[{"p":["counter"],"na":1}]`),
		Source:      "client-a",
		Sequence:    7,
	}
	if _, err := store.SubmitWithRequest(ctx, first); err != nil {
		t.Fatalf("first submit failed: %v", err)
	}

	conflict := first
	conflict.Operation = json.RawMessage(`[{"p":["counter"],"na":2}]`)
	_, err = store.SubmitWithRequest(ctx, conflict)
	if err == nil {
		t.Fatalf("expected sequence conflict error")
	}
}

func TestSubmitWithRequestPublishesSequence(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-event-seq", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	events, cancel, err := store.Subscribe(ctx, "doc-event-seq", 1)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer cancel()

	_, err = store.SubmitWithRequest(ctx, SubmitRequest{
		DocumentID:  "doc-event-seq",
		BaseVersion: 0,
		Operation:   json.RawMessage(`[{"p":["counter"],"na":1}]`),
		Source:      "client-a",
		Sequence:    42,
	})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	select {
	case event := <-events:
		if event.Source != "client-a" || event.Sequence != 42 || event.ID.Source != "client-a" || event.ID.Sequence != 42 {
			t.Fatalf("unexpected event identity: %+v", event)
		}
	default:
		t.Fatalf("expected one event")
	}
}

func TestSubmitMiddlewareCanRejectBeforeCommit(t *testing.T) {
	ctx := context.Background()
	rejected := errors.New("reject submit")
	store := NewServer(
		NewMemoryBackend(),
		NewMemoryLocker(),
		WithSubmitMiddleware(func(next SubmitHandler) SubmitHandler {
			return func(ctx context.Context, req SubmitRequest) (SubmitResult, error) {
				return SubmitResult{}, rejected
			}
		}),
	)

	_, err := store.CreateDocument(ctx, "doc-middleware-reject", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	_, err = store.Submit(ctx, "doc-middleware-reject", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "client-a")
	if !errors.Is(err, rejected) {
		t.Fatalf("expected rejection error, got %v", err)
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-middleware-reject")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if snapshot.Version != 0 || string(snapshot.Document) != `{"counter":0}` {
		t.Fatalf("submit should not commit after middleware rejection: %+v", snapshot)
	}

	ops, err := store.GetOperations(ctx, "doc-middleware-reject", 0, 0)
	if err != nil {
		t.Fatalf("get operations failed: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("expected no committed ops after rejection, got %d", len(ops))
	}
}

func TestNewServerPanicsWhenSubmitMiddlewareReturnsNilHandler(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected NewServer to panic when middleware returns nil handler")
		}
		if msg := r.(string); msg != "sharedb: submit middleware returned nil handler" {
			t.Fatalf("unexpected panic message: %v", r)
		}
	}()

	_ = NewServer(
		NewMemoryBackend(),
		NewMemoryLocker(),
		WithSubmitMiddleware(func(next SubmitHandler) SubmitHandler {
			return nil
		}),
	)
}

func TestSubmitMiddlewareCanRewriteRequestAndObserveResult(t *testing.T) {
	ctx := context.Background()
	var calls []string
	store := NewServer(
		NewMemoryBackend(),
		NewMemoryLocker(),
		WithSubmitMiddleware(
			func(next SubmitHandler) SubmitHandler {
				return func(ctx context.Context, req SubmitRequest) (SubmitResult, error) {
					calls = append(calls, "outer-before")
					result, err := next(ctx, req)
					calls = append(calls, "outer-after")
					return result, err
				}
			},
			func(next SubmitHandler) SubmitHandler {
				return func(ctx context.Context, req SubmitRequest) (SubmitResult, error) {
					calls = append(calls, "inner-before")
					req.Source = "middleware-client"
					result, err := next(ctx, req)
					if err == nil {
						calls = append(calls, "inner-after")
					}
					return result, err
				}
			},
		),
	)

	_, err := store.CreateDocument(ctx, "doc-middleware-wrap", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	ch, cancel, err := store.Subscribe(ctx, "doc-middleware-wrap", 1)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer cancel()

	result, err := store.Submit(ctx, "doc-middleware-wrap", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "client-a")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	if result.Version != 1 || string(result.Document) != `{"counter":1}` {
		t.Fatalf("unexpected submit result: %+v", result)
	}
	wantCalls := []string{"outer-before", "inner-before", "inner-after", "outer-after"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected middleware call order: got %v want %v", calls, wantCalls)
	}

	ops, err := store.GetOperations(ctx, "doc-middleware-wrap", 0, 1)
	if err != nil {
		t.Fatalf("get operations failed: %v", err)
	}
	if len(ops) != 1 || ops[0].Source != "middleware-client" {
		t.Fatalf("middleware should rewrite request before commit, got %+v", ops)
	}

	select {
	case event := <-ch:
		if event.Source != "middleware-client" {
			t.Fatalf("middleware-rewritten source should be published, got %+v", event)
		}
	default:
		t.Fatalf("expected submit event")
	}
}

func TestGetOperationsRejectsInvalidRanges(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()
	_, err := store.CreateDocument(ctx, "doc-invalid-range", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	if _, err := store.Submit(ctx, "doc-invalid-range", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "client-a"); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	cases := []struct{ from, to int }{{-1, 0}, {1, 0}, {0, 2}}
	for _, tc := range cases {
		if _, err := store.GetOperations(ctx, "doc-invalid-range", tc.from, tc.to); err == nil {
			t.Fatalf("expected invalid range error for [%d,%d]", tc.from, tc.to)
		}
	}

	ops, err := store.GetOperations(ctx, "doc-invalid-range", 1, 1)
	if err != nil {
		t.Fatalf("empty range should be valid: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("empty range returned %d ops", len(ops))
	}
}

func TestIdentityOmittedFromJSONWhenUnset(t *testing.T) {
	eventRaw, err := json.Marshal(Event{DocumentID: "doc", Version: 1, Operation: json.RawMessage(`[]`), Document: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("marshal event failed: %v", err)
	}
	if strings.Contains(string(eventRaw), `"id"`) || strings.Contains(string(eventRaw), `"seq"`) || strings.Contains(string(eventRaw), `"source"`) {
		t.Fatalf("unset event identity should be omitted, got %s", eventRaw)
	}

	recordRaw, err := json.Marshal(OpRecord{DocumentID: "doc", Version: 1, Op: json.RawMessage(`[]`)})
	if err != nil {
		t.Fatalf("marshal op record failed: %v", err)
	}
	if strings.Contains(string(recordRaw), `"id"`) || strings.Contains(string(recordRaw), `"seq"`) || strings.Contains(string(recordRaw), `"source"`) {
		t.Fatalf("unset op identity should be omitted, got %s", recordRaw)
	}
}

func TestMemoryPublisherDeliversIndependentEventPayloads(t *testing.T) {
	ctx := context.Background()
	pub := NewMemoryPublisher()
	left, cancelLeft, err := pub.Subscribe(ctx, "doc-pub", 1)
	if err != nil {
		t.Fatalf("subscribe left failed: %v", err)
	}
	defer cancelLeft()
	right, cancelRight, err := pub.Subscribe(ctx, "doc-pub", 1)
	if err != nil {
		t.Fatalf("subscribe right failed: %v", err)
	}
	defer cancelRight()

	pub.Publish(ctx, Event{DocumentID: "doc-pub", Version: 1, Operation: json.RawMessage(`[{"p":["x"],"na":1}]`), Document: json.RawMessage(`{"x":1}`)})
	leftEvent := <-left
	leftEvent.Operation[0] = 'X'
	leftEvent.Document[0] = 'X'
	rightEvent := <-right
	if string(rightEvent.Operation) != `[{"p":["x"],"na":1}]` || string(rightEvent.Document) != `{"x":1}` {
		t.Fatalf("subscriber payloads should be independent, got op=%s doc=%s", rightEvent.Operation, rightEvent.Document)
	}
}
