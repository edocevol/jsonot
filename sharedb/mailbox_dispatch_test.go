package sharedb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestEnvelopeForSessionBuildsEventEnvelope(t *testing.T) {
	event := Event{
		Type:       EventTypeOp,
		DocumentID: "doc-1",
		Version:    7,
		Source:     "client-a",
		Operation:  json.RawMessage(`[{"p":["counter"],"na":1}]`),
		Document:   json.RawMessage(`{"counter":1}`),
	}
	env, err := EnvelopeForSession("sess-1", event, func() string { return "env-7" })
	if err != nil {
		t.Fatalf("envelope for session failed: %v", err)
	}
	if env.ID != "env-7" || env.SessionID != "sess-1" || env.DocumentID != "doc-1" || env.Kind != EnvelopeKindEvent || env.Version != 7 {
		t.Fatalf("unexpected envelope metadata: %+v", env)
	}
	if env.Event == nil || env.Event.Type != EventTypeOp || !sameJSON(t, env.Event.Operation, event.Operation) || !sameJSON(t, env.Event.Document, event.Document) {
		t.Fatalf("unexpected envelope event payload: %+v", env.Event)
	}
}

func TestEnvelopeForSessionClonesEventPayloads(t *testing.T) {
	event := Event{
		Type:       EventTypeOp,
		DocumentID: "doc-1",
		Version:    1,
		Operation:  json.RawMessage(`[{"p":["counter"],"na":1}]`),
		Document:   json.RawMessage(`{"counter":1}`),
	}
	env, err := EnvelopeForSession("sess-1", event, func() string { return "env-1" })
	if err != nil {
		t.Fatalf("envelope for session failed: %v", err)
	}
	event.Operation[0] = 'X'
	event.Document[0] = 'X'
	if string(env.Event.Operation) != `[{"p":["counter"],"na":1}]` || string(env.Event.Document) != `{"counter":1}` {
		t.Fatalf("envelope should clone event payloads, got op=%s doc=%s", env.Event.Operation, env.Event.Document)
	}
}

func TestEnvelopeForSessionRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name      string
		sessionID string
		event     Event
		gen       EnvelopeIDGenerator
	}{
		{name: "empty session", sessionID: "", event: Event{Type: EventTypeOp, DocumentID: "doc-1", Version: 1, Operation: json.RawMessage(`[]`), Document: json.RawMessage(`{}`)}, gen: func() string { return "env-1" }},
		{name: "nil generator", sessionID: "sess-1", event: Event{Type: EventTypeOp, DocumentID: "doc-1", Version: 1, Operation: json.RawMessage(`[]`), Document: json.RawMessage(`{}`)}, gen: nil},
		{name: "empty id", sessionID: "sess-1", event: Event{Type: EventTypeOp, DocumentID: "doc-1", Version: 1, Operation: json.RawMessage(`[]`), Document: json.RawMessage(`{}`)}, gen: func() string { return "" }},
		{name: "empty document", sessionID: "sess-1", event: Event{Type: EventTypeOp, Version: 1, Operation: json.RawMessage(`[]`), Document: json.RawMessage(`{}`)}, gen: func() string { return "env-1" }},
		{name: "empty event type", sessionID: "sess-1", event: Event{DocumentID: "doc-1", Version: 1, Operation: json.RawMessage(`[]`), Document: json.RawMessage(`{}`)}, gen: func() string { return "env-1" }},
		{name: "negative version", sessionID: "sess-1", event: Event{Type: EventTypeOp, DocumentID: "doc-1", Version: -1, Operation: json.RawMessage(`[]`), Document: json.RawMessage(`{}`)}, gen: func() string { return "env-1" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := EnvelopeForSession(tc.sessionID, tc.event, tc.gen)
			if !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("expected invalid envelope error, got %v", err)
			}
		})
	}
}

func TestStaticSessionResolverReturnsCopy(t *testing.T) {
	resolver := StaticSessionResolver{"doc-1": {"sess-1", "sess-2"}}
	got, err := resolver.SessionsForDocument(context.Background(), "doc-1")
	if err != nil {
		t.Fatalf("sessions for document failed: %v", err)
	}
	want := []string{"sess-1", "sess-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected sessions: got %v want %v", got, want)
	}
	got[0] = "mutated"
	again, err := resolver.SessionsForDocument(context.Background(), "doc-1")
	if err != nil {
		t.Fatalf("sessions for document second read failed: %v", err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatalf("resolver should return copy, got %v want %v", again, want)
	}
}
