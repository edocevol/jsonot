package sharedb

import (
	"context"
)

// EnvelopeIDGenerator creates stable or testable envelope IDs.
type EnvelopeIDGenerator func() string

// SessionResolver returns subscribed session IDs for a document.
type SessionResolver interface {
	SessionsForDocument(ctx context.Context, documentID string) ([]string, error)
}

// StaticSessionResolver is a simple in-memory resolver for tests and demos.
type StaticSessionResolver map[string][]string

func (r StaticSessionResolver) SessionsForDocument(_ context.Context, documentID string) ([]string, error) {
	sessions := r[documentID]
	return append([]string(nil), sessions...), nil
}

// EnvelopeForSession converts a committed document event into a session-scoped envelope.
func EnvelopeForSession(sessionID string, event Event, gen EnvelopeIDGenerator) (Envelope, error) {
	if gen == nil {
		return Envelope{}, ErrInvalidEnvelope
	}
	if err := validateDispatchEvent(event); err != nil {
		return Envelope{}, ErrInvalidEnvelope
	}
	env := Envelope{
		ID:         gen(),
		SessionID:  sessionID,
		DocumentID: event.DocumentID,
		Kind:       EnvelopeKindEvent,
		Version:    event.Version,
		Event:      &event,
	}
	if err := validateEnvelope(env); err != nil {
		return Envelope{}, err
	}
	return cloneEnvelope(env), nil
}
