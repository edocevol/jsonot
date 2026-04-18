package sharedb

import (
	"context"
	"encoding/json"
)

// DocRecord holds the latest document snapshot stored in the backend.
type DocRecord struct {
	DocumentID string          `json:"documentId"`
	Version    int             `json:"version"` // version of last applied op (0 = initial)
	Doc        json.RawMessage `json:"doc"`
}

// OpRecord holds one committed operation entry in the op log.
type OpRecord struct {
	DocumentID string          `json:"documentId"`
	Version    int             `json:"version"` // version this op produced (1-based)
	Source     string          `json:"source,omitempty"`
	Op         json.RawMessage `json:"op"`
}

// Backend abstracts all durable storage for documents and op history.
// Implementations control where snapshots and ops are persisted.
//
// Concurrency model: the Server calls Backend methods while holding
// a per-document lock obtained from Locker. Backend implementations
// do NOT need to be internally serialized per document, but must be
// safe for concurrent calls across different documents.
type Backend interface {
	// CreateDoc initializes a document with the given initial content.
	// Returns ErrDocumentExists if a document with that ID already exists.
	CreateDoc(ctx context.Context, docID string, initial json.RawMessage) error

	// GetDoc returns the latest snapshot for docID.
	// Returns ErrDocumentNotFound when the document does not exist.
	GetDoc(ctx context.Context, docID string) (DocRecord, error)

	// SaveDoc persists an updated snapshot. Called after every successful op.
	SaveDoc(ctx context.Context, record DocRecord) error

	// AppendOp appends a committed op to the per-document op log.
	// The caller is responsible for setting record.Version correctly.
	AppendOp(ctx context.Context, record OpRecord) error

	// GetOps returns ops that produced versions in [fromVersion+1, toVersion].
	// The returned slice is ordered by version ascending.
	// An empty slice is returned when fromVersion == toVersion.
	GetOps(ctx context.Context, docID string, fromVersion, toVersion int) ([]OpRecord, error)
}

// Locker provides per-document mutual exclusion.
// The Server acquires a document lock before reading the current version,
// transforming, applying, and writing back. This is what makes the version
// number "centralized" — only one Submit can advance the version at a time,
// even across multiple server processes when backed by a distributed Locker
// (e.g. Redis SETNX / Redlock).
type Locker interface {
	// Lock acquires an exclusive lock for docID.
	// The returned unlock function must be called exactly once.
	Lock(ctx context.Context, docID string) (unlock func(), err error)
}

// Publisher broadcasts committed operation events to subscribers.
// The in-memory implementation delivers events directly through Go channels.
// A Redis implementation would use PUBLISH/SUBSCRIBE so events cross process
// boundaries (horizontal scaling).
type Publisher interface {
	// Publish sends the event to all current subscribers of the document.
	// Must not block; slow subscribers may miss events.
	Publish(ctx context.Context, event Event)

	// Subscribe registers a subscriber and returns a read-only channel and
	// a cancel function that must be called to release resources.
	Subscribe(ctx context.Context, docID string, buffer int) (<-chan Event, func(), error)
}
