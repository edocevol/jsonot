package sharedb

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ─── MemoryBackend ─────────────────────────────────────────────────────────────

type memDoc struct {
	version int
	doc     json.RawMessage
	ops     []OpRecord // index i holds the op that produced version i+1
}

// MemoryBackend is a thread-safe, in-process Backend implementation.
// All state is lost when the process exits. Suitable for tests and demos.
type MemoryBackend struct {
	mu   sync.RWMutex
	docs map[string]*memDoc
}

// NewMemoryBackend creates an empty MemoryBackend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{docs: make(map[string]*memDoc)}
}

var _ Backend = (*MemoryBackend)(nil)

func (b *MemoryBackend) CreateDoc(_ context.Context, docID string, initial json.RawMessage) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.docs[docID]; ok {
		return ErrDocumentExists
	}

	b.docs[docID] = &memDoc{
		doc: append(json.RawMessage(nil), initial...),
		ops: make([]OpRecord, 0),
	}

	return nil
}

func (b *MemoryBackend) GetDoc(_ context.Context, docID string) (DocRecord, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	d, ok := b.docs[docID]
	if !ok {
		return DocRecord{}, ErrDocumentNotFound
	}

	return DocRecord{
		DocumentID: docID,
		Version:    d.version,
		Doc:        append(json.RawMessage(nil), d.doc...),
	}, nil
}

func (b *MemoryBackend) SaveDoc(_ context.Context, record DocRecord) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	d, ok := b.docs[record.DocumentID]
	if !ok {
		return ErrDocumentNotFound
	}

	d.version = record.Version
	d.doc = append(json.RawMessage(nil), record.Doc...)
	return nil
}

func (b *MemoryBackend) AppendOp(_ context.Context, record OpRecord) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	d, ok := b.docs[record.DocumentID]
	if !ok {
		return ErrDocumentNotFound
	}

	d.ops = append(d.ops, record)
	return nil
}

// GetOps returns ops that produced versions in [fromVersion+1, toVersion].
func (b *MemoryBackend) GetOps(_ context.Context, docID string, fromVersion, toVersion int) ([]OpRecord, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	d, ok := b.docs[docID]
	if !ok {
		return nil, ErrDocumentNotFound
	}

	if fromVersion == toVersion {
		return nil, nil
	}

	// ops[i] produced version i+1, so ops[fromVersion..toVersion-1] for versions fromVersion+1..toVersion.
	if fromVersion < 0 || toVersion > len(d.ops) {
		return nil, fmt.Errorf("sharedb: GetOps out of range [%d, %d] (len=%d)",
			fromVersion, toVersion, len(d.ops))
	}

	slice := d.ops[fromVersion:toVersion]
	result := make([]OpRecord, len(slice))
	copy(result, slice)
	return result, nil
}

// ─── MemoryLocker ──────────────────────────────────────────────────────────────

// MemoryLocker implements per-document mutual exclusion using Go mutexes.
// It is suitable for in-process use only.
type MemoryLocker struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewMemoryLocker creates a new MemoryLocker.
func NewMemoryLocker() *MemoryLocker {
	return &MemoryLocker{locks: make(map[string]*sync.Mutex)}
}

var _ Locker = (*MemoryLocker)(nil)

func (l *MemoryLocker) Lock(_ context.Context, docID string) (func(), error) {
	l.mu.Lock()
	m, ok := l.locks[docID]
	if !ok {
		m = new(sync.Mutex)
		l.locks[docID] = m
	}
	l.mu.Unlock()

	m.Lock()
	return m.Unlock, nil
}

// ─── MemoryPublisher ───────────────────────────────────────────────────────────

type memSubscriber struct {
	docID string
	ch    chan Event
}

// MemoryPublisher implements Publisher with in-process Go channels.
type MemoryPublisher struct {
	mu   sync.Mutex
	subs map[int]*memSubscriber
	next int
}

// NewMemoryPublisher creates a new MemoryPublisher.
func NewMemoryPublisher() *MemoryPublisher {
	return &MemoryPublisher{subs: make(map[int]*memSubscriber)}
}

var _ Publisher = (*MemoryPublisher)(nil)

func (p *MemoryPublisher) Publish(_ context.Context, event Event) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, sub := range p.subs {
		if sub.docID != event.DocumentID {
			continue
		}
		select {
		case sub.ch <- event:
		default:
			// slow subscriber — drop event
		}
	}
}

func (p *MemoryPublisher) Subscribe(_ context.Context, docID string, buffer int) (<-chan Event, func(), error) {
	if buffer < 0 {
		buffer = 0
	}

	p.mu.Lock()
	id := p.next
	p.next++
	sub := &memSubscriber{docID: docID, ch: make(chan Event, buffer)}
	p.subs[id] = sub
	p.mu.Unlock()

	cancel := func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if s, ok := p.subs[id]; ok {
			delete(p.subs, id)
			close(s.ch)
		}
	}

	return sub.ch, cancel, nil
}

// ─── NewMemoryServer convenience constructor ───────────────────────────────────

// NewMemoryServer creates a fully in-process Server backed by MemoryBackend,
// MemoryLocker and MemoryPublisher. Suitable for tests and single-node demos.
func NewMemoryServer() *Server {
	pub := NewMemoryPublisher()
	return NewServer(
		NewMemoryBackend(),
		NewMemoryLocker(),
		WithPublisher(pub),
	)
}
