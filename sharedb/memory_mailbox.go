package sharedb

import (
	"context"
	"sync"
)

// MemoryMailboxStore stores replayable envelopes in-process for tests and demos.
type MemoryMailboxStore struct {
	mu        sync.Mutex
	bySession map[string][]Envelope
	acked     map[string]string
}

var _ MailboxStore = (*MemoryMailboxStore)(nil)

func NewMemoryMailboxStore() *MemoryMailboxStore {
	return &MemoryMailboxStore{
		bySession: make(map[string][]Envelope),
		acked:     make(map[string]string),
	}
}

func (s *MemoryMailboxStore) Append(_ context.Context, env Envelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateEnvelope(env); err != nil {
		return err
	}
	for _, existing := range s.bySession[env.SessionID] {
		if existing.ID == env.ID {
			return ErrDuplicateEnvelopeID
		}
	}
	s.bySession[env.SessionID] = append(s.bySession[env.SessionID], cloneEnvelope(env))
	return nil
}

func (s *MemoryMailboxStore) ReplayAfter(_ context.Context, sessionID, afterEnvelopeID string, limit int) ([]Envelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	envelopes := s.bySession[sessionID]
	start := 0
	if afterEnvelopeID != "" {
		found := false
		for i, env := range envelopes {
			if env.ID == afterEnvelopeID {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return nil, ErrEnvelopeNotFound
		}
	}
	if start >= len(envelopes) {
		return nil, nil
	}
	end := len(envelopes)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	out := make([]Envelope, 0, end-start)
	for _, env := range envelopes[start:end] {
		out = append(out, cloneEnvelope(env))
	}
	return out, nil
}

func (s *MemoryMailboxStore) AckThrough(_ context.Context, sessionID, envelopeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if envelopeID == "" {
		return nil
	}
	envelopes := s.bySession[sessionID]
	candidateIndex := -1
	for i, env := range envelopes {
		if env.ID == envelopeID {
			candidateIndex = i
			break
		}
	}
	if candidateIndex == -1 {
		return ErrEnvelopeNotFound
	}
	current := s.acked[sessionID]
	if current == "" {
		s.acked[sessionID] = envelopeID
		return nil
	}
	currentIndex := -1
	for i, env := range envelopes {
		if env.ID == current {
			currentIndex = i
			break
		}
	}
	if currentIndex == -1 || candidateIndex > currentIndex {
		s.acked[sessionID] = envelopeID
	}
	return nil
}

func (s *MemoryMailboxStore) LastAcked(_ context.Context, sessionID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acked[sessionID], nil
}
