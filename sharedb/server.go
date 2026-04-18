package sharedb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/edocevol/jsonot"
)

var (
	// ErrDocumentNotFound means the requested document does not exist.
	ErrDocumentNotFound = errors.New("sharedb: document not found")
	// ErrDocumentExists means a document with the same ID already exists.
	ErrDocumentExists = errors.New("sharedb: document already exists")
	// ErrInvalidVersion means the supplied base version is out of range.
	ErrInvalidVersion = errors.New("sharedb: invalid version")
)

// Snapshot is the latest immutable view of a document.
type Snapshot struct {
	DocumentID string          `json:"documentId"`
	Version    int             `json:"version"`
	Document   json.RawMessage `json:"document"`
}

// Event describes one operation that was accepted and committed.
type Event struct {
	DocumentID string          `json:"documentId"`
	Version    int             `json:"version"`
	Source     string          `json:"source,omitempty"`
	Operation  json.RawMessage `json:"op"`
	Document   json.RawMessage `json:"document"`
}

// SubmitResult is returned by Server.Submit.
type SubmitResult struct {
	// Version is the new document version after the op was committed.
	Version int `json:"version"`
	// Rebased is true when the op was transformed against concurrent ops
	// before being applied (i.e. baseVersion < server version at submit time).
	Rebased bool `json:"rebased"`
	// Operation is the (possibly transformed) op that was actually applied.
	Operation json.RawMessage `json:"op"`
	// Document is the document state after the op.
	Document json.RawMessage `json:"document"`
}

// ServerOption is a functional option for NewServer.
type ServerOption func(*Server)

// WithPublisher overrides the default in-memory Publisher.
func WithPublisher(pub Publisher) ServerOption {
	return func(s *Server) { s.pub = pub }
}

// Server is the central coordinator for collaborative editing.
//
// Version number flow
// -------------------
// Every Submit first acquires a per-document lock (Locker), reads the
// current version from Backend, transforms the incoming op against any
// history ops since baseVersion, applies the result, then atomically saves
// the new snapshot and op entry before releasing the lock. This makes the
// version number "centralized": even when Server runs on multiple nodes,
// only one Submit can advance the version at a time for a given document,
// provided all nodes share the same Locker (e.g. Redis Redlock) and
// Backend (e.g. Redis).
type Server struct {
	backend Backend
	locker  Locker
	pub     Publisher
	ot      *jsonot.JSONOperationTransformer
}

// NewServer creates a Server with the given backend and locker.
// Pass WithPublisher to use a custom publisher (e.g. Redis Pub/Sub).
// When no publisher is provided, events are delivered only within the
// current process via the in-memory default publisher created alongside
// a MemoryBackend by NewMemoryServer.
func NewServer(backend Backend, locker Locker, opts ...ServerOption) *Server {
	s := &Server{
		backend: backend,
		locker:  locker,
		ot:      jsonot.NewJSONOperationTransformer(),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.pub == nil {
		s.pub = NewMemoryPublisher()
	}
	return s
}

// CreateDocument initializes a new document with optional initial content.
// initial may be nil or an empty slice, in which case an empty object {} is used.
func (s *Server) CreateDocument(ctx context.Context, documentID string, initial json.RawMessage) (Snapshot, error) {
	unlock, err := s.locker.Lock(ctx, documentID)
	if err != nil {
		return Snapshot{}, err
	}
	defer unlock()

	payload := initial
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	if err := s.backend.CreateDoc(ctx, documentID, payload); err != nil {
		return Snapshot{}, err
	}

	return Snapshot{DocumentID: documentID, Version: 0, Document: append(json.RawMessage(nil), payload...)}, nil
}

// GetSnapshot returns the latest snapshot of a document.
func (s *Server) GetSnapshot(ctx context.Context, documentID string) (Snapshot, error) {
	rec, err := s.backend.GetDoc(ctx, documentID)
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		DocumentID: documentID,
		Version:    rec.Version,
		Document:   append(json.RawMessage(nil), rec.Doc...),
	}, nil
}

// Submit accepts a client operation based on baseVersion.
//
// Centralized version protocol:
//  1. Acquire per-document lock → ensures serial version advancement.
//  2. Read current version V from backend.
//  3. If baseVersion < V, fetch ops [baseVersion+1..V] and OT-transform
//     the incoming op against each of them in order.
//  4. Apply the (possibly transformed) op to the snapshot.
//  5. Persist new snapshot at version V+1 and append the op record.
//  6. Release lock, publish Event to subscribers.
func (s *Server) Submit(
	ctx context.Context,
	documentID string,
	baseVersion int,
	rawOperation json.RawMessage,
	source string,
) (SubmitResult, error) {
	// Step 1: acquire lock
	unlock, err := s.locker.Lock(ctx, documentID)
	if err != nil {
		return SubmitResult{}, err
	}
	defer unlock()

	// Step 2: read current state
	rec, err := s.backend.GetDoc(ctx, documentID)
	if err != nil {
		return SubmitResult{}, err
	}

	if baseVersion < 0 || baseVersion > rec.Version {
		return SubmitResult{}, fmt.Errorf("%w: expected 0-%d, got %d", ErrInvalidVersion, rec.Version, baseVersion)
	}

	op, err := s.parseOperation(rawOperation)
	if err != nil {
		return SubmitResult{}, err
	}

	// Step 3: transform against concurrent ops
	transformed := op
	rebased := false
	if baseVersion < rec.Version {
		concurrentOps, err := s.backend.GetOps(ctx, documentID, baseVersion, rec.Version)
		if err != nil {
			return SubmitResult{}, err
		}

		for _, opRec := range concurrentOps {
			concurrent, err := s.parseOperation(opRec.Op)
			if err != nil {
				return SubmitResult{}, fmt.Errorf("failed to parse concurrent op at version %d: %w", opRec.Version, err)
			}
			transformed, _, err = s.ot.Transform(ctx, transformed, concurrent)
			if err != nil {
				return SubmitResult{}, err
			}
		}
		rebased = true
	}

	// Empty op after transform → nothing to commit
	if transformed.IsEmpty() {
		return SubmitResult{
			Version:   rec.Version,
			Rebased:   rebased,
			Operation: json.RawMessage("[]"),
			Document:  append(json.RawMessage(nil), rec.Doc...),
		}, nil
	}

	// Step 4: apply
	docValue, err := jsonot.UnmarshalValue(rec.Doc)
	if err != nil {
		return SubmitResult{}, err
	}
	applied := s.ot.Apply(ctx, docValue, transformed)
	if applied.IsError() {
		return SubmitResult{}, applied.Error()
	}

	newDoc := append(json.RawMessage(nil), applied.MustGet().RawMessage()...)
	newVersion := rec.Version + 1
	serializedOp := append(json.RawMessage(nil), transformed.ToValue().RawMessage()...)

	// Step 5: persist snapshot + op log
	if err := s.backend.SaveDoc(ctx, DocRecord{
		DocumentID: documentID,
		Version:    newVersion,
		Doc:        newDoc,
	}); err != nil {
		return SubmitResult{}, err
	}
	if err := s.backend.AppendOp(ctx, OpRecord{
		DocumentID: documentID,
		Version:    newVersion,
		Source:     source,
		Op:         serializedOp,
	}); err != nil {
		return SubmitResult{}, err
	}

	result := SubmitResult{
		Version:   newVersion,
		Rebased:   rebased,
		Operation: serializedOp,
		Document:  newDoc,
	}

	// Step 6: publish event (lock already released via defer, but publish while we have data)
	s.pub.Publish(ctx, Event{
		DocumentID: documentID,
		Version:    newVersion,
		Source:     source,
		Operation:  append(json.RawMessage(nil), serializedOp...),
		Document:   append(json.RawMessage(nil), newDoc...),
	})

	return result, nil
}

// Subscribe registers a subscriber for committed operations on documentID.
// buffer controls the Event channel capacity. Returns a cancel func that
// must be called to unsubscribe and release resources.
func (s *Server) Subscribe(ctx context.Context, documentID string, buffer int) (<-chan Event, func(), error) {
	// Make sure the document exists before subscribing.
	if _, err := s.backend.GetDoc(ctx, documentID); err != nil {
		return nil, nil, err
	}
	return s.pub.Subscribe(ctx, documentID, buffer)
}

func (s *Server) parseOperation(raw json.RawMessage) (*jsonot.Operation, error) {
	payload := raw
	if len(payload) == 0 {
		payload = json.RawMessage("[]")
	}

	node, err := jsonot.UnmarshalValue(payload)
	if err != nil {
		return nil, err
	}

	components := s.ot.OperationComponentsFromValue(node)
	if components.IsError() {
		return nil, components.Error()
	}

	op := jsonot.NewOperation(components.MustGet())
	if err := op.Validation(); err != nil {
		return nil, err
	}

	return op, nil
}
