# SharedDB Session Mailbox / Envelope Replay Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add a session-scoped mailbox / envelope replay layer for `jsonot/sharedb` so WebSocket or Socket.IO clients can reconnect and fetch unacknowledged envelopes after disconnects.

**Architecture:** Keep `sharedb` core focused on document truth (`snapshot`, `version`, `op history`, `submit`, `subscribe`). Add a new mailbox abstraction beside the existing realtime `Publisher`, not inside it. The mailbox is keyed by `sessionID` and stores ordered envelopes plus an ack cursor so transports can replay missed messages after reconnect.

**Tech Stack:** Go, `jsonot/sharedb`, in-memory mailbox store for MVP, existing `Event` / `SubmitResult` semantics, later-compatible with Redis-backed mailbox storage.

---

## Scope of MVP

The first deliverable should support only these capabilities:

1. session-scoped ordered envelopes
2. append envelopes for a session
3. replay envelopes after a given cursor
4. ack-through semantics (`ack envelope X` means all envelopes up to X are confirmed)
5. enough API surface to wire a future WebSocket / Socket.IO reconnect flow

### Explicitly out of scope for MVP

- presence / awareness envelopes
- TTL cleanup policies
- Redis mailbox implementation
- cross-document global inbox
- exactly-once delivery guarantees
- changing the existing `Publisher` interface semantics
- replacing `GetOperations` / snapshot-based recovery

---

## Design constraints

### Constraint 1: mailbox key is `sessionID`, not websocket connection ID

A websocket connection is ephemeral and changes after reconnect. The mailbox must survive reconnects, browser refreshes, and network flaps. Use a transport-level `sessionID` as the stable key.

### Constraint 2: `Envelope.ID` is not the same as document version

A session mailbox can contain different kinds of messages:

- document event envelope
- submit ack envelope
- resync-required envelope
- error envelope

So replay ordering must use an envelope cursor (`Envelope.ID`), not just `Event.Version`.

### Constraint 3: do not overload `Publisher`

The current `Publisher` is a best-effort realtime fanout primitive. It should remain lightweight. Mailbox behavior belongs in a new abstraction and can be composed with the realtime publisher later.

### Constraint 4: preserve future compatibility with Redis / multi-node transport

The in-memory mailbox store should be simple but the interfaces must not assume single-process delivery forever.

---

## Proposed types

### Task 1: Add envelope model types

**Objective:** Introduce the domain types for mailbox messages without wiring them into transport yet.

**Files:**
- Modify: `sharedb/server.go` or create `sharedb/mailbox.go`
- Test: `sharedb/store_test.go` or new `sharedb/mailbox_test.go`

**Step 1: Write failing test**

Add a serialization/shape test that asserts an envelope can hold:

- `ID`
- `SessionID`
- `DocumentID`
- `Kind`
- `Version`
- `Event`

Expected envelope kinds:

```go
type EnvelopeKind string

const (
    EnvelopeKindEvent  EnvelopeKind = "event"
    EnvelopeKindAck    EnvelopeKind = "ack"
    EnvelopeKindResync EnvelopeKind = "resync"
    EnvelopeKindError  EnvelopeKind = "error"
)
```

**Step 2: Run test to verify failure**

Run: `cd sharedb && go test ./... -run TestEnvelope -count=1`
Expected: FAIL because the types do not exist yet.

**Step 3: Write minimal implementation**

Add:

```go
type Envelope struct {
    ID         string       `json:"id"`
    SessionID  string       `json:"sessionId"`
    DocumentID string       `json:"documentId"`
    Kind       EnvelopeKind `json:"kind"`
    Version    int          `json:"version,omitempty"`
    Event      Event        `json:"event"`
}
```

Prefer placing this in `sharedb/mailbox.go` to keep `server.go` from growing too much.

**Step 4: Run test to verify pass**

Run: `cd sharedb && go test ./... -run TestEnvelope -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add sharedb/mailbox.go sharedb/*test.go
git commit -m "feat(sharedb): add mailbox envelope types"
```

---

### Task 2: Add mailbox store interface

**Objective:** Define the storage contract for session mailbox replay.

**Files:**
- Modify: `sharedb/backend.go` or create `sharedb/mailbox.go`
- Test: `sharedb/mailbox_test.go`

**Step 1: Write failing test**

Add compile-time assertions that the memory implementation satisfies the mailbox interface.

**Step 2: Run test to verify failure**

Run: `cd sharedb && go test ./... -run TestMemoryMailboxStoreImplementsInterface -count=1`
Expected: FAIL because the interface or implementation is missing.

**Step 3: Write minimal implementation**

Add:

```go
type MailboxStore interface {
    Append(ctx context.Context, env Envelope) error
    ReplayAfter(ctx context.Context, sessionID, afterEnvelopeID string, limit int) ([]Envelope, error)
    AckThrough(ctx context.Context, sessionID, envelopeID string) error
    LastAcked(ctx context.Context, sessionID string) (string, error)
}
```

Semantics to document now:

- `ReplayAfter(sessionID, "", limit)` returns from the beginning
- results are ordered oldest → newest
- `AckThrough` is monotonic; acking an older envelope after a newer one must not move the cursor backward
- `limit <= 0` should mean “no artificial limit” for the in-memory implementation

**Step 4: Run test to verify pass**

Run: `cd sharedb && go test ./... -run TestMemoryMailboxStoreImplementsInterface -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add sharedb/mailbox.go sharedb/*test.go
git commit -m "feat(sharedb): add mailbox store interface"
```

---

### Task 3: Implement `MemoryMailboxStore`

**Objective:** Add an in-process mailbox store suitable for tests and demos.

**Files:**
- Modify: `sharedb/memory.go` or create `sharedb/memory_mailbox.go`
- Test: `sharedb/mailbox_test.go`

**Step 1: Write failing test**

Add these tests:

1. `TestMemoryMailboxReplayReturnsOrderedEnvelopes`
2. `TestMemoryMailboxReplayAfterCursorSkipsAckedPrefix`
3. `TestMemoryMailboxAckThroughIsMonotonic`
4. `TestMemoryMailboxReplayClonesPayloads`

The payload cloning test matters because `Envelope.Event` contains `json.RawMessage`.

**Step 2: Run test to verify failure**

Run: `cd sharedb && go test ./... -run 'TestMemoryMailbox' -count=1`
Expected: FAIL because the implementation does not exist.

**Step 3: Write minimal implementation**

Suggested structure:

```go
type MemoryMailboxStore struct {
    mu        sync.Mutex
    bySession map[string][]Envelope
    acked     map[string]string
}
```

Implementation notes:

- clone every stored envelope before saving
- clone every returned envelope before replaying
- keep append order stable
- `ReplayAfter` must find the envelope whose `ID == afterEnvelopeID` and return strictly later envelopes
- if `afterEnvelopeID` is unknown, return a sentinel error like `ErrEnvelopeNotFound` or treat it as a replay-gap/resync signal; define this explicitly in the test first

**Recommended MVP behavior:**
If `afterEnvelopeID` is unknown for that session and non-empty, return a clear error such as:

```go
var ErrEnvelopeNotFound = errors.New("sharedb: envelope not found")
```

This makes reconnect fallback explicit.

**Step 4: Run tests to verify pass**

Run:

```bash
cd sharedb && go test ./... -run 'TestMemoryMailbox' -count=1
cd sharedb && go test ./...
```

Expected: PASS.

**Step 5: Commit**

```bash
git add sharedb/memory*.go sharedb/*test.go
git commit -m "feat(sharedb): add in-memory mailbox store"
```

---

### Task 4: Add replay result helper for transports

**Objective:** Expose a transport-friendly API that converts mailbox state into a reconnect decision.

**Files:**
- Create: `sharedb/reconnect.go`
- Test: `sharedb/reconnect_test.go`

**Step 1: Write failing test**

Add tests for a helper like:

```go
type ReplayResult struct {
    SessionID      string
    Envelopes      []Envelope
    LastAcked      string
    RequiresResync bool
}
```

and:

```go
func ReplaySessionMailbox(ctx context.Context, store MailboxStore, sessionID, afterEnvelopeID string, limit int) (ReplayResult, error)
```

Required semantics:

- valid cursor → replay remaining envelopes
- empty cursor → replay from start
- unknown cursor → `RequiresResync=true` OR explicit `ErrEnvelopeNotFound`

**Pick one and keep it consistent.**

**Recommended MVP choice:** helper converts `ErrEnvelopeNotFound` into `RequiresResync=true` with no hard failure.

**Step 2: Run test to verify failure**

Run: `cd sharedb && go test ./... -run TestReplaySessionMailbox -count=1`
Expected: FAIL.

**Step 3: Write minimal implementation**

Implement the helper with minimal logic:

- call `LastAcked`
- call `ReplayAfter`
- translate missing cursor into `RequiresResync=true`

**Step 4: Run test to verify pass**

Run: `cd sharedb && go test ./... -run TestReplaySessionMailbox -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add sharedb/reconnect.go sharedb/reconnect_test.go
git commit -m "feat(sharedb): add mailbox replay helper"
```

---

### Task 5: Define mailbox-aware transport composition contract

**Objective:** Add the composition points needed to combine realtime publisher + mailbox store later, without implementing the full websocket transport in this PR.

**Files:**
- Create: `sharedb/mailbox_dispatch.go`
- Test: `sharedb/mailbox_dispatch_test.go`
- Docs: `sharedb/README.md`

**Step 1: Write failing test**

Add a test for a helper function that turns a document `Event` into session envelopes, e.g.:

```go
func EnvelopeForSession(sessionID string, event Event) Envelope
```

and/or a dispatcher contract:

```go
type SessionResolver interface {
    SessionsForDocument(ctx context.Context, documentID string) ([]string, error)
}
```

This task does not need to implement a full dispatcher yet. It should only nail down how events become session-scoped envelopes.

**Step 2: Run test to verify failure**

Run: `cd sharedb && go test ./... -run TestEnvelopeForSession -count=1`
Expected: FAIL.

**Step 3: Write minimal implementation**

Create a helper that builds envelopes from an event and a session ID.

For MVP envelope ID generation, use a simple injectable ID generator:

```go
type EnvelopeIDGenerator func() string
```

Do not hardcode time-based logic deep inside the helper. Keep it testable.

**Step 4: Run test to verify pass**

Run: `cd sharedb && go test ./... -run TestEnvelopeForSession -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add sharedb/mailbox_dispatch*.go sharedb/*test.go sharedb/README.md
git commit -m "feat(sharedb): define session envelope dispatch helpers"
```

---

### Task 6: Update documentation

**Objective:** Document mailbox semantics and how they differ from realtime publish/subscribe.

**Files:**
- Modify: `sharedb/README.md`
- Modify: `docs/sharedb-style-backend.md`

**Step 1: Write failing doc check**

Not a code test. Instead, identify missing README sections before editing:

- what mailbox solves
- why session ID is the mailbox key
- difference between `version` and `Envelope.ID`
- reconnect / replay flow
- why this does not replace `GetOperations`

**Step 2: Update docs**

Add a new section:

- `## Session mailbox / envelope replay`

It should explain:

1. `Publisher` remains realtime best-effort
2. mailbox is for reconnect replay
3. mailbox is keyed by session, not connection
4. unknown cursor means resync path

**Step 3: Verify docs are coherent**

Read the edited sections and check they match the implementation.

**Step 4: Commit**

```bash
git add sharedb/README.md docs/sharedb-style-backend.md
git commit -m "docs: describe sharedb session mailbox design"
```

---

## Suggested PR breakdown

### PR 1 — Mailbox foundation (recommended first)

Include only:

- `EnvelopeKind`
- `Envelope`
- `MailboxStore`
- `ErrEnvelopeNotFound`
- `MemoryMailboxStore`
- tests for append / replay / ack / clone safety

**Why first:** smallest coherent slice, no transport entanglement.

### PR 2 — Replay helper

Include:

- `ReplayResult`
- `ReplaySessionMailbox(...)`
- reconnect decision tests

### PR 3 — Envelope dispatch helpers

Include:

- session envelope creation helpers
- ID generator contract
- session resolver interface

### PR 4 — WebSocket / Socket.IO demo integration

Include:

- session-aware reconnect handshake
- replay-before-live-subscribe flow
- demo docs and manual verification steps

---

## Testing matrix

Run after each code task:

```bash
cd /home/ubuntu/jsonot/sharedb && go test ./...
cd /home/ubuntu/jsonot/sharedb && go vet ./...
cd /home/ubuntu/jsonot/sharedb && go test -race ./...
cd /home/ubuntu/jsonot && go test ./...
```

---

## Important implementation notes

### Note 1: clone raw payloads

`Envelope` contains `Event`, which contains `json.RawMessage`. Store and replay cloned payloads so test callers cannot mutate mailbox state accidentally.

### Note 2: ack cursor must be monotonic

If a client already acked `env-10`, then a later `AckThrough(session, "env-5")` must not move the cursor backward.

### Note 3: unknown cursor should not be silently ignored

If replay starts after a cursor that the mailbox does not know, that means replay continuity is broken. The caller must be told to resync.

### Note 4: do not promise exactly-once delivery

The mailbox provides replayable ordered envelopes and monotonic ack cursors. That is enough for robust reconnect UX. Do not claim exactly-once semantics in docs or APIs.

---

## Final handoff

The next implementation step should be:

> Start with **PR 1 — Mailbox foundation**.

That means the immediate coding target is:

- `sharedb/mailbox.go`
- `sharedb/memory_mailbox.go` (or `memory.go` if you prefer colocating)
- `sharedb/mailbox_test.go`

with strict TDD and no websocket integration yet.
