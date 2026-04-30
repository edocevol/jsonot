package sharedb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/edocevol/jsonot"
)

var (
	// ErrDocumentNotFound means the requested document does not exist.
	ErrDocumentNotFound = errors.New("sharedb: document not found")
	// ErrDocumentExists means a document with the same ID already exists.
	ErrDocumentExists = errors.New("sharedb: document already exists")
	// ErrInvalidVersion means the supplied base version is out of range.
	ErrInvalidVersion = errors.New("sharedb: invalid version")
	// ErrDuplicateSequenceConflict means a client reused Source+Sequence for a different operation.
	ErrDuplicateSequenceConflict = errors.New("sharedb: duplicate source sequence conflicts with original operation")
	// ErrStaleResyncRequired means the client is too far behind for bounded server-side rebase.
	ErrStaleResyncRequired = errors.New("sharedb: stale submit requires resync")
)

// Snapshot is the latest immutable view of a document.
type Snapshot struct {
	DocumentID string          `json:"documentId"`
	Version    int             `json:"version"`
	Document   json.RawMessage `json:"document"`
}

// EventType classifies lifecycle and operation events sent to subscribers.
type EventType string

const (
	EventTypeCreate EventType = "create"
	EventTypeOp     EventType = "op"
	EventTypeDelete EventType = "delete"
)

// Event describes one operation that was accepted and committed.
type Event struct {
	Type       EventType       `json:"type,omitempty"`
	DocumentID string          `json:"documentId"`
	Version    int             `json:"version"`
	ID         OpID            `json:"id,omitempty"`
	Source     string          `json:"source,omitempty"` // deprecated: use ID.Source
	Sequence   int             `json:"seq,omitempty"`    // deprecated: use ID.Sequence
	Operation  json.RawMessage `json:"op"`
	Document   json.RawMessage `json:"document"`
}

// SubmitRequest is the structured form of a client operation submission.
// Source+Sequence are optional, but when both are supplied the server treats
// retries of the same client sequence as idempotent and returns the already
// committed result without applying the operation again.
type SubmitRequest struct {
	DocumentID  string          `json:"documentId"`
	BaseVersion int             `json:"baseVersion"`
	Operation   json.RawMessage `json:"op"`
	ID          OpID            `json:"id,omitempty"`
	Source      string          `json:"source,omitempty"` // deprecated: use ID.Source
	Sequence    int             `json:"seq,omitempty"`    // deprecated: use ID.Sequence
}

// SubmitResult is returned by Server.Submit.
type SubmitResult struct {
	// Version is the new document version after the op was committed.
	Version int `json:"version"`
	// Rebased is true when the op was transformed against concurrent ops
	// before being applied (i.e. baseVersion < server version at submit time).
	Rebased bool `json:"rebased"`
	// Duplicate is true when SubmitWithRequest detected a retry for the same
	// Source+Sequence and did not apply the operation again.
	Duplicate bool `json:"duplicate,omitempty"`
	// Operation is the (possibly transformed) op that was actually applied.
	Operation json.RawMessage `json:"op"`
	// Document is the document state after the op.
	Document json.RawMessage `json:"document"`
}

// SubmitHandler handles a structured submit request.
type SubmitHandler func(context.Context, SubmitRequest) (SubmitResult, error)

// StaleReason classifies why the server requires a client resync instead of
// continuing with replay or server-side OT rebase.
type StaleReason string

const (
	StaleReasonVersionBehindWindow  StaleReason = "version_behind_window"
	StaleReasonReplayCursorNotFound StaleReason = "replay_cursor_not_found"
)

// StaleSubmitError describes a submit that the server rejected because the
// client's base version fell outside the configured bounded rebase window.
type StaleSubmitError struct {
	Reason              StaleReason `json:"reason,omitempty"`
	DocumentID          string      `json:"documentId,omitempty"`
	BaseVersion         int         `json:"baseVersion,omitempty"`
	CurrentVersion      int         `json:"currentVersion,omitempty"`
	MinSupportedVersion int         `json:"minSupportedVersion,omitempty"`
	MaxRebaseGap        int         `json:"maxRebaseGap,omitempty"`
}

func (e *StaleSubmitError) Error() string {
	if e == nil {
		return ErrStaleResyncRequired.Error()
	}
	return fmt.Sprintf("%s: reason=%s document=%s base=%d current=%d minSupported=%d maxRebaseGap=%d", ErrStaleResyncRequired.Error(), e.Reason, e.DocumentID, e.BaseVersion, e.CurrentVersion, e.MinSupportedVersion, e.MaxRebaseGap)
}

func (e *StaleSubmitError) Unwrap() error {
	return ErrStaleResyncRequired
}

// SubmitMiddleware wraps submit handling so callers can validate, reject,
// enrich, or observe submit requests/results.
type SubmitMiddleware func(SubmitHandler) SubmitHandler

// ServerOption is a functional option for NewServer.
type ServerOption func(*Server)

// WithPublisher overrides the default in-memory Publisher.
func WithPublisher(pub Publisher) ServerOption {
	return func(s *Server) { s.pub = pub }
}

// WithSubmitMiddleware appends middleware around SubmitWithRequest/Submit.
// Middleware are applied in declaration order: the first middleware is the
// outermost wrapper, and the last middleware runs closest to the core submit.
func WithSubmitMiddleware(middleware ...SubmitMiddleware) ServerOption {
	return func(s *Server) {
		for _, mw := range middleware {
			if mw != nil {
				s.submitMiddleware = append(s.submitMiddleware, mw)
			}
		}
	}
}

// WithMaxRebaseGap bounds how many committed versions a stale submit may be
// rebased across on the server. A value <= 0 disables the limit.
func WithMaxRebaseGap(maxGap int) ServerOption {
	return func(s *Server) {
		s.maxRebaseGap = maxGap
	}
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

	parsedOps    parsedOpCache
	maxRebaseGap int

	submitMiddleware []SubmitMiddleware
	submitHandler    SubmitHandler
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
		parsedOps: parsedOpCache{
			docs: make(map[string]map[int]*jsonot.Operation),
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.pub == nil {
		s.pub = NewMemoryPublisher()
	}
	s.submitHandler = s.buildSubmitHandler()
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

// GetSnapshotAt reconstructs the document snapshot at a historical version by
// applying inverse OT operations from the current snapshot backwards.
func (s *Server) GetSnapshotAt(ctx context.Context, documentID string, version int) (Snapshot, error) {
	rec, err := s.backend.GetDoc(ctx, documentID)
	if err != nil {
		return Snapshot{}, err
	}
	if version < 0 || version > rec.Version {
		return Snapshot{}, fmt.Errorf("%w: expected 0-%d, got %d", ErrInvalidVersion, rec.Version, version)
	}
	if version == rec.Version {
		return Snapshot{DocumentID: documentID, Version: rec.Version, Document: append(json.RawMessage(nil), rec.Doc...)}, nil
	}

	document, err := s.reconstructSnapshotAt(ctx, documentID, rec.Doc, version, rec.Version)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{DocumentID: documentID, Version: version, Document: document}, nil
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
	return s.SubmitWithRequest(ctx, SubmitRequest{
		DocumentID:  documentID,
		BaseVersion: baseVersion,
		Operation:   rawOperation,
		Source:      source,
	})
}

// SubmitWithRequest accepts a structured client operation submission. In
// addition to Submit's versioned rebase semantics, Source+Sequence provide
// ShareDB-style idempotency for clients that retry after transport failures.
func (s *Server) SubmitWithRequest(ctx context.Context, req SubmitRequest) (SubmitResult, error) {
	return s.submitHandler(ctx, req)
}

func (s *Server) submitCore(ctx context.Context, req SubmitRequest) (SubmitResult, error) {
	opID := req.opID()
	// Step 1: acquire lock
	unlock, err := s.locker.Lock(ctx, req.DocumentID)
	if err != nil {
		return SubmitResult{}, err
	}
	defer unlock()

	// Step 2: read current state
	rec, err := s.backend.GetDoc(ctx, req.DocumentID)
	if err != nil {
		return SubmitResult{}, err
	}

	if req.BaseVersion < 0 || req.BaseVersion > rec.Version {
		return SubmitResult{}, fmt.Errorf("%w: expected 0-%d, got %d", ErrInvalidVersion, rec.Version, req.BaseVersion)
	}

	if opID.Source != "" && opID.Sequence > 0 && rec.Version > 0 {
		ops, err := s.backend.GetOps(ctx, req.DocumentID, 0, rec.Version)
		if err != nil {
			return SubmitResult{}, err
		}
		for _, opRec := range ops {
			recID := opRec.opID()
			if recID.Source == opID.Source && recID.Sequence == opID.Sequence {
				if len(opRec.SubmittedOp) > 0 && !jsonRawEqual(req.Operation, opRec.SubmittedOp) {
					return SubmitResult{}, ErrDuplicateSequenceConflict
				}
				return SubmitResult{
					Version:   rec.Version,
					Duplicate: true,
					Operation: append(json.RawMessage(nil), opRec.Op...),
					Document:  append(json.RawMessage(nil), rec.Doc...),
				}, nil
			}
		}
	}

	op, err := s.parseOperation(req.Operation)
	if err != nil {
		return SubmitResult{}, err
	}

	// Step 3: transform against concurrent ops
	transformed := op
	rebased := false
	if req.BaseVersion < rec.Version {
		gap := rec.Version - req.BaseVersion
		if s.maxRebaseGap > 0 && gap > s.maxRebaseGap {
			return SubmitResult{}, &StaleSubmitError{
				Reason:              StaleReasonVersionBehindWindow,
				DocumentID:          req.DocumentID,
				BaseVersion:         req.BaseVersion,
				CurrentVersion:      rec.Version,
				MinSupportedVersion: rec.Version - s.maxRebaseGap,
				MaxRebaseGap:        s.maxRebaseGap,
			}
		}
		concurrentOps, err := s.backend.GetOps(ctx, req.DocumentID, req.BaseVersion, rec.Version)
		if err != nil {
			return SubmitResult{}, err
		}

		for _, opRec := range concurrentOps {
			concurrent, err := s.getOrParseCommittedOperation(opRec)
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

	// Step 5: persist snapshot + op log atomically
	if err := s.backend.CommitOp(ctx, DocRecord{
		DocumentID: req.DocumentID,
		Version:    newVersion,
		Doc:        newDoc,
	}, OpRecord{
		DocumentID:  req.DocumentID,
		Version:     newVersion,
		BaseVersion: req.BaseVersion,
		ID:          opID,
		Source:      opID.Source,
		Sequence:    opID.Sequence,
		SubmittedOp: append(json.RawMessage(nil), req.Operation...),
		Op:          serializedOp,
	}); err != nil {
		return SubmitResult{}, err
	}

	result := SubmitResult{
		Version:   newVersion,
		Rebased:   rebased,
		Operation: serializedOp,
		Document:  newDoc,
	}
	s.cacheCommittedOperation(OpRecord{DocumentID: req.DocumentID, Version: newVersion, Op: serializedOp}, transformed)

	// Step 6: publish event (lock already released via defer, but publish while we have data)
	s.pub.Publish(ctx, Event{
		Type:       EventTypeOp,
		DocumentID: req.DocumentID,
		Version:    newVersion,
		ID:         opID,
		Source:     opID.Source,
		Sequence:   opID.Sequence,
		Operation:  append(json.RawMessage(nil), serializedOp...),
		Document:   append(json.RawMessage(nil), newDoc...),
	})

	return result, nil
}

func (s *Server) buildSubmitHandler() SubmitHandler {
	handler := s.submitCore
	for i := len(s.submitMiddleware) - 1; i >= 0; i-- {
		handler = s.submitMiddleware[i](handler)
		if handler == nil {
			panic("sharedb: submit middleware returned nil handler")
		}
	}
	return handler
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

// DeleteDocument removes a document snapshot after validating that baseVersion
// matches the latest server version. Existing subscribers receive a delete event.
func (s *Server) DeleteDocument(ctx context.Context, documentID string, baseVersion int, source string) error {
	unlock, err := s.locker.Lock(ctx, documentID)
	if err != nil {
		return err
	}
	defer unlock()

	rec, err := s.backend.GetDoc(ctx, documentID)
	if err != nil {
		return err
	}
	if baseVersion != rec.Version {
		return fmt.Errorf("%w: expected %d, got %d", ErrInvalidVersion, rec.Version, baseVersion)
	}
	if err := s.backend.DeleteDoc(ctx, documentID); err != nil {
		return err
	}
	s.parsedOps.DeleteDocument(documentID)

	s.pub.Publish(ctx, Event{
		Type:       EventTypeDelete,
		DocumentID: documentID,
		Version:    rec.Version,
		Source:     source,
		Document:   append(json.RawMessage(nil), rec.Doc...),
	})
	return nil
}

// RollbackToVersion computes an inverse OT operation from the current version
// back to targetVersion and commits it as a new versioned operation.
func (s *Server) RollbackToVersion(ctx context.Context, documentID string, targetVersion int, source string) (SubmitResult, error) {
	unlock, err := s.locker.Lock(ctx, documentID)
	if err != nil {
		return SubmitResult{}, err
	}
	defer unlock()

	rec, err := s.backend.GetDoc(ctx, documentID)
	if err != nil {
		return SubmitResult{}, err
	}
	if targetVersion < 0 || targetVersion > rec.Version {
		return SubmitResult{}, fmt.Errorf("%w: expected 0-%d, got %d", ErrInvalidVersion, rec.Version, targetVersion)
	}
	if targetVersion == rec.Version {
		return SubmitResult{Version: rec.Version, Operation: json.RawMessage("[]"), Document: append(json.RawMessage(nil), rec.Doc...)}, nil
	}

	rollbackOp, rollbackDoc, err := s.buildRollback(ctx, documentID, rec.Doc, targetVersion, rec.Version)
	if err != nil {
		return SubmitResult{}, err
	}
	serializedOp := append(json.RawMessage(nil), rollbackOp.ToValue().RawMessage()...)
	newVersion := rec.Version + 1
	if err := s.backend.CommitOp(ctx, DocRecord{DocumentID: documentID, Version: newVersion, Doc: rollbackDoc}, OpRecord{DocumentID: documentID, Version: newVersion, BaseVersion: rec.Version, Source: source, SubmittedOp: serializedOp, Op: serializedOp}); err != nil {
		return SubmitResult{}, err
	}
	s.cacheCommittedOperation(OpRecord{DocumentID: documentID, Version: newVersion, Op: serializedOp}, rollbackOp)

	result := SubmitResult{Version: newVersion, Operation: serializedOp, Document: append(json.RawMessage(nil), rollbackDoc...)}
	s.pub.Publish(ctx, Event{Type: EventTypeOp, DocumentID: documentID, Version: newVersion, Source: source, Operation: append(json.RawMessage(nil), serializedOp...), Document: append(json.RawMessage(nil), rollbackDoc...)})
	return result, nil
}

// GetOperations returns committed operation records that produced versions in
// [fromVersion+1, toVersion]. It is useful for client catch-up, audit logs, and
// reconnect flows that need ShareDB-style op history.
func (s *Server) GetOperations(ctx context.Context, documentID string, fromVersion, toVersion int) ([]OpRecord, error) {
	rec, err := s.backend.GetDoc(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if fromVersion < 0 || toVersion < fromVersion || toVersion > rec.Version {
		return nil, fmt.Errorf("%w: expected 0-%d range, got [%d, %d]", ErrInvalidVersion, rec.Version, fromVersion, toVersion)
	}
	ops, err := s.backend.GetOps(ctx, documentID, fromVersion, toVersion)
	if err != nil {
		return nil, err
	}
	if len(ops) != toVersion-fromVersion {
		return nil, fmt.Errorf("sharedb: incomplete operation history for %s: got %d ops, want %d", documentID, len(ops), toVersion-fromVersion)
	}
	return ops, nil
}

func (r SubmitRequest) opID() OpID {
	id := r.ID
	if id.Source == "" {
		id.Source = r.Source
	}
	if id.Sequence == 0 {
		id.Sequence = r.Sequence
	}
	return id
}

func (r OpRecord) opID() OpID {
	id := r.ID
	if id.Source == "" {
		id.Source = r.Source
	}
	if id.Sequence == 0 {
		id.Sequence = r.Sequence
	}
	return id
}

func jsonRawEqual(a, b json.RawMessage) bool {
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

func cloneEvent(event Event) Event {
	cloned := event
	cloned.Operation = append(json.RawMessage(nil), event.Operation...)
	cloned.Document = append(json.RawMessage(nil), event.Document...)
	return cloned
}

func (id OpID) isZero() bool {
	return id.Source == "" && id.Sequence == 0
}

func (e Event) MarshalJSON() ([]byte, error) {
	type eventJSON struct {
		Type       EventType       `json:"type,omitempty"`
		DocumentID string          `json:"documentId"`
		Version    int             `json:"version"`
		ID         *OpID           `json:"id,omitempty"`
		Source     string          `json:"source,omitempty"`
		Sequence   int             `json:"seq,omitempty"`
		Operation  json.RawMessage `json:"op"`
		Document   json.RawMessage `json:"document"`
	}
	var id *OpID
	if !e.ID.isZero() {
		idValue := e.ID
		id = &idValue
	}
	return json.Marshal(eventJSON{
		Type:       e.Type,
		DocumentID: e.DocumentID,
		Version:    e.Version,
		ID:         id,
		Source:     e.Source,
		Sequence:   e.Sequence,
		Operation:  e.Operation,
		Document:   e.Document,
	})
}

func (r OpRecord) MarshalJSON() ([]byte, error) {
	type opRecordJSON struct {
		DocumentID  string          `json:"documentId"`
		Version     int             `json:"version"`
		BaseVersion int             `json:"baseVersion"`
		ID          *OpID           `json:"id,omitempty"`
		Source      string          `json:"source,omitempty"`
		Sequence    int             `json:"seq,omitempty"`
		SubmittedOp json.RawMessage `json:"submittedOp,omitempty"`
		Op          json.RawMessage `json:"op"`
	}
	var id *OpID
	if !r.ID.isZero() {
		idValue := r.ID
		id = &idValue
	}
	return json.Marshal(opRecordJSON{
		DocumentID:  r.DocumentID,
		Version:     r.Version,
		BaseVersion: r.BaseVersion,
		ID:          id,
		Source:      r.Source,
		Sequence:    r.Sequence,
		SubmittedOp: r.SubmittedOp,
		Op:          r.Op,
	})
}

func (s *Server) reconstructSnapshotAt(ctx context.Context, documentID string, currentDocument json.RawMessage, targetVersion, currentVersion int) (json.RawMessage, error) {
	ops, err := s.GetOperations(ctx, documentID, targetVersion, currentVersion)
	if err != nil {
		return nil, err
	}
	docValue, err := jsonot.UnmarshalValue(currentDocument)
	if err != nil {
		return nil, err
	}
	for i := len(ops) - 1; i >= 0; i-- {
		inverse, err := s.invertCommittedOperation(ops[i])
		if err != nil {
			return nil, err
		}
		applied := s.ot.Apply(ctx, docValue, inverse)
		if applied.IsError() {
			return nil, applied.Error()
		}
		docValue = applied.MustGet()
	}
	return append(json.RawMessage(nil), docValue.RawMessage()...), nil
}

func (s *Server) buildRollback(ctx context.Context, documentID string, currentDocument json.RawMessage, targetVersion, currentVersion int) (*jsonot.Operation, json.RawMessage, error) {
	ops, err := s.GetOperations(ctx, documentID, targetVersion, currentVersion)
	if err != nil {
		return nil, nil, err
	}
	docValue, err := jsonot.UnmarshalValue(currentDocument)
	if err != nil {
		return nil, nil, err
	}
	rollback := jsonot.NewOperation([]*jsonot.OperationComponent{})
	for i := len(ops) - 1; i >= 0; i-- {
		inverse, err := s.invertCommittedOperation(ops[i])
		if err != nil {
			return nil, nil, err
		}
		rollback.Compose(inverse)
		applied := s.ot.Apply(ctx, docValue, inverse)
		if applied.IsError() {
			return nil, nil, applied.Error()
		}
		docValue = applied.MustGet()
	}
	if err := rollback.Validation(); err != nil {
		return nil, nil, err
	}
	return rollback, append(json.RawMessage(nil), docValue.RawMessage()...), nil
}

func (s *Server) invertCommittedOperation(opRec OpRecord) (*jsonot.Operation, error) {
	op, err := s.getOrParseCommittedOperation(opRec)
	if err != nil {
		return nil, err
	}
	components := op.Array()
	inverted := jsonot.NewOperation([]*jsonot.OperationComponent{})
	for i := len(components) - 1; i >= 0; i-- {
		inverse := components[i].Invert()
		if inverse.IsError() {
			return nil, inverse.Error()
		}
		inverted.Append(inverse.MustGet())
	}
	return s.parseOperation(append(json.RawMessage(nil), inverted.ToValue().RawMessage()...))
}

func (s *Server) getOrParseCommittedOperation(opRec OpRecord) (*jsonot.Operation, error) {
	if op, ok := s.parsedOps.Get(opRec.DocumentID, opRec.Version); ok {
		return op, nil
	}
	op, err := s.parseOperation(opRec.Op)
	if err != nil {
		return nil, err
	}
	s.cacheCommittedOperation(opRec, op)
	return cloneOperation(op), nil
}

func (s *Server) cacheCommittedOperation(opRec OpRecord, op *jsonot.Operation) {
	if opRec.DocumentID == "" || opRec.Version <= 0 || op == nil {
		return
	}
	s.parsedOps.Store(opRec.DocumentID, opRec.Version, op)
}

func cloneOperation(op *jsonot.Operation) *jsonot.Operation {
	if op == nil {
		return nil
	}
	components := op.Array()
	clones := make([]*jsonot.OperationComponent, 0, len(components))
	for _, component := range components {
		clones = append(clones, component.Clone())
	}
	return jsonot.NewOperation(clones)
}

type parsedOpCache struct {
	mu   sync.RWMutex
	docs map[string]map[int]*jsonot.Operation
}

func (c *parsedOpCache) Get(documentID string, version int) (*jsonot.Operation, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	versions, ok := c.docs[documentID]
	if !ok {
		return nil, false
	}
	op, ok := versions[version]
	if !ok {
		return nil, false
	}
	return cloneOperation(op), true
}

func (c *parsedOpCache) Store(documentID string, version int, op *jsonot.Operation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.docs == nil {
		c.docs = make(map[string]map[int]*jsonot.Operation)
	}
	versions, ok := c.docs[documentID]
	if !ok {
		versions = make(map[int]*jsonot.Operation)
		c.docs[documentID] = versions
	}
	versions[version] = cloneOperation(op)
}

func (c *parsedOpCache) DeleteDocument(documentID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.docs, documentID)
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
