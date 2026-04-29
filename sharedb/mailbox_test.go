package sharedb

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestEnvelopeJSONShape(t *testing.T) {
	raw, err := json.Marshal(Envelope{
		ID:         "env-1",
		SessionID:  "sess-1",
		DocumentID: "doc-1",
		Kind:       EnvelopeKindEvent,
		Version:    2,
		Event: &Event{
			Type:       EventTypeOp,
			DocumentID: "doc-1",
			Version:    2,
			Operation:  json.RawMessage(`[{"p":["counter"],"na":1}]`),
			Document:   json.RawMessage(`{"counter":1}`),
		},
	})
	if err != nil {
		t.Fatalf("marshal envelope failed: %v", err)
	}
	text := string(raw)
	for _, want := range []string{`"id":"env-1"`, `"sessionId":"sess-1"`, `"documentId":"doc-1"`, `"kind":"event"`, `"version":2`, `"event":`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected envelope JSON to contain %s, got %s", want, text)
		}
	}
}

func TestEnvelopeJSONOmitsEventForNonEventKindsAndKeepsZeroVersion(t *testing.T) {
	raw, err := json.Marshal(Envelope{
		ID:         "env-ack-1",
		SessionID:  "sess-1",
		DocumentID: "doc-1",
		Kind:       EnvelopeKindAck,
		Version:    0,
	})
	if err != nil {
		t.Fatalf("marshal ack envelope failed: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"version":0`) {
		t.Fatalf("expected zero version to be preserved, got %s", text)
	}
	if strings.Contains(text, `"event"`) {
		t.Fatalf("non-event envelope should omit event payload, got %s", text)
	}
}

func TestMemoryMailboxStoreImplementsInterface(t *testing.T) {
	var _ MailboxStore = NewMemoryMailboxStore()
}

func TestMemoryMailboxReplayReturnsOrderedEnvelopes(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	first := testEnvelope("env-1", "sess-1", 1)
	second := testEnvelope("env-2", "sess-1", 2)
	third := testEnvelope("env-3", "sess-1", 3)
	for _, env := range []Envelope{first, second, third} {
		if err := store.Append(ctx, env); err != nil {
			t.Fatalf("append failed: %v", err)
		}
	}

	got, err := store.ReplayAfter(ctx, "sess-1", "", 0)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("unexpected replay count: got %d want 3", len(got))
	}
	for i, want := range []string{"env-1", "env-2", "env-3"} {
		if got[i].ID != want {
			t.Fatalf("unexpected envelope order at %d: got %s want %s", i, got[i].ID, want)
		}
	}
}

func TestMemoryMailboxReplayAfterCursorSkipsAckedPrefix(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	for i := 1; i <= 3; i++ {
		if err := store.Append(ctx, testEnvelope("env-"+string(rune('0'+i)), "sess-1", i)); err != nil {
			t.Fatalf("append failed: %v", err)
		}
	}

	got, err := store.ReplayAfter(ctx, "sess-1", "env-2", 0)
	if err != nil {
		t.Fatalf("replay after failed: %v", err)
	}
	if len(got) != 1 || got[0].ID != "env-3" {
		t.Fatalf("unexpected replay after cursor: %+v", got)
	}
}

func TestMemoryMailboxAckThroughIsMonotonic(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	for i := 1; i <= 3; i++ {
		if err := store.Append(ctx, testEnvelope("env-"+string(rune('0'+i)), "sess-1", i)); err != nil {
			t.Fatalf("append failed: %v", err)
		}
	}
	if err := store.AckThrough(ctx, "sess-1", "env-3"); err != nil {
		t.Fatalf("ack through latest failed: %v", err)
	}
	if err := store.AckThrough(ctx, "sess-1", "env-2"); err != nil {
		t.Fatalf("ack through older failed: %v", err)
	}
	acked, err := store.LastAcked(ctx, "sess-1")
	if err != nil {
		t.Fatalf("last acked failed: %v", err)
	}
	if acked != "env-3" {
		t.Fatalf("ack cursor moved backward: got %s want env-3", acked)
	}
}

func TestMemoryMailboxAckThroughUnknownCursorReturnsEnvelopeNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	if err := store.Append(ctx, testEnvelope("env-1", "sess-1", 1)); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := store.AckThrough(ctx, "sess-1", "env-missing"); !errors.Is(err, ErrEnvelopeNotFound) {
		t.Fatalf("expected envelope not found error, got %v", err)
	}
}

func TestMemoryMailboxReplayClonesPayloads(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	env := testEnvelope("env-1", "sess-1", 1)
	if err := store.Append(ctx, env); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	env.Event.Operation[0] = 'Y'
	env.Event.Document[0] = 'Y'

	got, err := store.ReplayAfter(ctx, "sess-1", "", 0)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	got[0].Event.Operation[0] = 'X'
	got[0].Event.Document[0] = 'X'

	again, err := store.ReplayAfter(ctx, "sess-1", "", 0)
	if err != nil {
		t.Fatalf("replay again failed: %v", err)
	}
	if string(again[0].Event.Operation) != `[{"p":["counter"],"na":1}]` || string(again[0].Event.Document) != `{"counter":1}` {
		t.Fatalf("replayed envelope payloads should be cloned, got op=%s doc=%s", again[0].Event.Operation, again[0].Event.Document)
	}
}

func TestMemoryMailboxReplayAfterUnknownCursorReturnsEnvelopeNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	if err := store.Append(ctx, testEnvelope("env-1", "sess-1", 1)); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	_, err := store.ReplayAfter(ctx, "sess-1", "env-missing", 0)
	if !errors.Is(err, ErrEnvelopeNotFound) {
		t.Fatalf("expected envelope not found error, got %v", err)
	}
}

func TestMemoryMailboxRejectsDuplicateEnvelopeIDsPerSession(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	first := testEnvelope("env-1", "sess-1", 1)
	if err := store.Append(ctx, first); err != nil {
		t.Fatalf("first append failed: %v", err)
	}
	duplicate := testEnvelope("env-1", "sess-1", 2)
	if err := store.Append(ctx, duplicate); !errors.Is(err, ErrDuplicateEnvelopeID) {
		t.Fatalf("expected duplicate envelope id error, got %v", err)
	}
}

func TestMemoryMailboxRejectsEmptyEnvelopeID(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	env := testEnvelope("", "sess-1", 1)
	if err := store.Append(ctx, env); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("expected invalid envelope error for empty id, got %v", err)
	}
}

func TestMemoryMailboxRejectsEmptySessionID(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	env := testEnvelope("env-1", "", 1)
	if err := store.Append(ctx, env); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("expected invalid envelope error for empty session id, got %v", err)
	}
}

func TestMemoryMailboxRejectsEventEnvelopeWithoutPayload(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	env := Envelope{
		ID:         "env-1",
		SessionID:  "sess-1",
		DocumentID: "doc-1",
		Kind:       EnvelopeKindEvent,
		Version:    1,
	}
	if err := store.Append(ctx, env); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("expected invalid envelope error for event without payload, got %v", err)
	}
}

func TestMemoryMailboxRejectsUnknownEnvelopeKind(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	env := testEnvelope("env-1", "sess-1", 1)
	env.Kind = EnvelopeKind("")
	if err := store.Append(ctx, env); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("expected invalid envelope error for unknown kind, got %v", err)
	}
}

func TestMemoryMailboxRejectsNonEventEnvelopeWithEventPayload(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	env := testEnvelope("env-1", "sess-1", 1)
	env.Kind = EnvelopeKindAck
	if err := store.Append(ctx, env); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("expected invalid envelope error for non-event payload, got %v", err)
	}
}

func testEnvelope(id, sessionID string, version int) Envelope {
	return Envelope{
		ID:         id,
		SessionID:  sessionID,
		DocumentID: "doc-1",
		Kind:       EnvelopeKindEvent,
		Version:    version,
		Event: &Event{
			Type:       EventTypeOp,
			DocumentID: "doc-1",
			Version:    version,
			Operation:  json.RawMessage(`[{"p":["counter"],"na":1}]`),
			Document:   json.RawMessage(`{"counter":1}`),
		},
	}
}
