package sharedb

import (
	"context"
	"errors"
)

// EnvelopeKind classifies replayable session mailbox messages.
type EnvelopeKind string

const (
	EnvelopeKindEvent  EnvelopeKind = "event"
	EnvelopeKindAck    EnvelopeKind = "ack"
	EnvelopeKindResync EnvelopeKind = "resync"
	EnvelopeKindError  EnvelopeKind = "error"
)

// ErrEnvelopeNotFound means the requested envelope cursor is unknown.
var ErrEnvelopeNotFound = errors.New("sharedb: envelope not found")

// ErrDuplicateEnvelopeID means a session already has an envelope with the same ID.
var ErrDuplicateEnvelopeID = errors.New("sharedb: duplicate envelope id")

// ErrInvalidEnvelope means the envelope is missing required cursor or payload fields.
var ErrInvalidEnvelope = errors.New("sharedb: invalid envelope")

// Envelope is a replayable session-scoped transport message.
type Envelope struct {
	ID         string       `json:"id"`
	SessionID  string       `json:"sessionId"`
	DocumentID string       `json:"documentId"`
	Kind       EnvelopeKind `json:"kind"`
	Version    int          `json:"version"`
	Event      *Event       `json:"event,omitempty"`
}

// MailboxStore persists replayable envelopes for a stable client session.
type MailboxStore interface {
	Append(ctx context.Context, env Envelope) error
	ReplayAfter(ctx context.Context, sessionID, afterEnvelopeID string, limit int) ([]Envelope, error)
	AckThrough(ctx context.Context, sessionID, envelopeID string) error
	LastAcked(ctx context.Context, sessionID string) (string, error)
}

func cloneEnvelope(env Envelope) Envelope {
	cloned := env
	if env.Event != nil {
		event := cloneEvent(*env.Event)
		cloned.Event = &event
	}
	return cloned
}

func validateEnvelope(env Envelope) error {
	if env.ID == "" || env.SessionID == "" {
		return ErrInvalidEnvelope
	}
	switch env.Kind {
	case EnvelopeKindEvent:
		if env.Event == nil {
			return ErrInvalidEnvelope
		}
	case EnvelopeKindAck, EnvelopeKindResync, EnvelopeKindError:
		if env.Event != nil {
			return ErrInvalidEnvelope
		}
	default:
		return ErrInvalidEnvelope
	}
	return nil
}
