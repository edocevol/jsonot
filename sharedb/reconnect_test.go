package sharedb

import (
	"context"
	"errors"
	"testing"
)

func TestReplaySessionMailboxReturnsRemainingEnvelopes(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	for i := 1; i <= 3; i++ {
		if err := store.Append(ctx, testEnvelope("env-"+string(rune('0'+i)), "sess-1", i)); err != nil {
			t.Fatalf("append failed: %v", err)
		}
	}
	if err := store.AckThrough(ctx, "sess-1", "env-1"); err != nil {
		t.Fatalf("ack failed: %v", err)
	}

	result, err := ReplaySessionMailbox(ctx, store, "sess-1", "env-1", 0)
	if err != nil {
		t.Fatalf("replay helper failed: %v", err)
	}
	if result.SessionID != "sess-1" {
		t.Fatalf("unexpected session id: %s", result.SessionID)
	}
	if result.LastAcked != "env-1" {
		t.Fatalf("unexpected last acked: %s", result.LastAcked)
	}
	if result.RequiresResync {
		t.Fatalf("valid cursor should not require resync")
	}
	if len(result.Envelopes) != 2 || result.Envelopes[0].ID != "env-2" || result.Envelopes[1].ID != "env-3" {
		t.Fatalf("unexpected replay result: %+v", result.Envelopes)
	}
}

func TestReplaySessionMailboxReplaysFromStartOnEmptyCursor(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	for i := 1; i <= 2; i++ {
		if err := store.Append(ctx, testEnvelope("env-"+string(rune('0'+i)), "sess-1", i)); err != nil {
			t.Fatalf("append failed: %v", err)
		}
	}

	result, err := ReplaySessionMailbox(ctx, store, "sess-1", "", 0)
	if err != nil {
		t.Fatalf("replay helper failed: %v", err)
	}
	if result.RequiresResync {
		t.Fatalf("empty cursor should replay from start, not require resync")
	}
	if len(result.Envelopes) != 2 || result.Envelopes[0].ID != "env-1" || result.Envelopes[1].ID != "env-2" {
		t.Fatalf("unexpected replay result from start: %+v", result.Envelopes)
	}
}

func TestReplaySessionMailboxMarksUnknownCursorAsResync(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	if err := store.Append(ctx, testEnvelope("env-1", "sess-1", 1)); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := store.AckThrough(ctx, "sess-1", "env-1"); err != nil {
		t.Fatalf("ack failed: %v", err)
	}

	result, err := ReplaySessionMailbox(ctx, store, "sess-1", "env-missing", 0)
	if err != nil {
		t.Fatalf("unknown cursor should convert to resync, got err=%v", err)
	}
	if !result.RequiresResync {
		t.Fatalf("unknown cursor should require resync")
	}
	if len(result.Envelopes) != 0 {
		t.Fatalf("resync result should not include replay envelopes, got %+v", result.Envelopes)
	}
	if result.LastAcked != "env-1" {
		t.Fatalf("unexpected last acked on resync result: %s", result.LastAcked)
	}
}

func TestReplaySessionMailboxAppliesLimit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMailboxStore()
	for i := 1; i <= 3; i++ {
		if err := store.Append(ctx, testEnvelope("env-"+string(rune('0'+i)), "sess-1", i)); err != nil {
			t.Fatalf("append failed: %v", err)
		}
	}

	result, err := ReplaySessionMailbox(ctx, store, "sess-1", "", 1)
	if err != nil {
		t.Fatalf("replay helper failed: %v", err)
	}
	if len(result.Envelopes) != 1 || result.Envelopes[0].ID != "env-1" {
		t.Fatalf("unexpected limited replay result: %+v", result.Envelopes)
	}
}

func TestReplaySessionMailboxPropagatesEnvelopeNotFoundForEmptyCursor(t *testing.T) {
	ctx := context.Background()
	store := &stubMailboxStore{
		replayErr: ErrEnvelopeNotFound,
	}
	_, err := ReplaySessionMailbox(ctx, store, "sess-1", "", 0)
	if !errors.Is(err, ErrEnvelopeNotFound) {
		t.Fatalf("expected envelope not found to propagate for empty cursor, got %v", err)
	}
}

type stubMailboxStore struct {
	lastAcked string
	replay    []Envelope
	replayErr error
	lastErr   error
}

func (s *stubMailboxStore) Append(context.Context, Envelope) error { return nil }
func (s *stubMailboxStore) ReplayAfter(context.Context, string, string, int) ([]Envelope, error) {
	if s.replayErr != nil {
		return nil, s.replayErr
	}
	return s.replay, nil
}
func (s *stubMailboxStore) AckThrough(context.Context, string, string) error { return nil }
func (s *stubMailboxStore) LastAcked(context.Context, string) (string, error) {
	if s.lastErr != nil {
		return "", s.lastErr
	}
	return s.lastAcked, nil
}
