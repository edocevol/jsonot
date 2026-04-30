package sharedb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func sameJSON(t *testing.T, got, want json.RawMessage) bool {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("got invalid JSON %q: %v", got, err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("want invalid JSON %q: %v", want, err)
	}
	return reflect.DeepEqual(gotValue, wantValue)
}

func TestSubmitDoesNotAdvanceSnapshotWhenCommitFails(t *testing.T) {
	ctx := context.Background()
	backend := &failingAppendBackend{MemoryBackend: NewMemoryBackend(), err: errors.New("append failed")}
	store := NewServer(backend, NewMemoryLocker())

	_, err := store.CreateDocument(ctx, "doc-atomic", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	_, err = store.Submit(ctx, "doc-atomic", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "client-a")
	if err == nil {
		t.Fatalf("expected submit to fail")
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-atomic")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if snapshot.Version != 0 {
		t.Fatalf("snapshot version advanced despite failed commit: got %d want 0", snapshot.Version)
	}
	if got := string(snapshot.Document); got != `{"counter":0}` {
		t.Fatalf("snapshot document changed despite failed commit: got %s", got)
	}

	ops, err := store.GetOperations(ctx, "doc-atomic", 0, 0)
	if err != nil {
		t.Fatalf("get operations failed: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("unexpected committed ops after failed commit: got %d", len(ops))
	}
}

type failingAppendBackend struct {
	*MemoryBackend
	err error
}

func (b *failingAppendBackend) AppendOp(context.Context, OpRecord) error {
	return b.err
}

func (b *failingAppendBackend) CommitOp(context.Context, DocRecord, OpRecord) error {
	return b.err
}

func TestStoreSequentialAndRebasedSubmit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-1", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	left := json.RawMessage(`[{"p":["counter"],"na":1}]`)
	right := json.RawMessage(`[{"p":["counter"],"na":2}]`)

	leftResult, err := store.Submit(ctx, "doc-1", 0, left, "left-client")
	if err != nil {
		t.Fatalf("submit left failed: %v", err)
	}
	if leftResult.Version != 1 {
		t.Fatalf("unexpected left version: got %d want 1", leftResult.Version)
	}
	if leftResult.Rebased {
		t.Fatalf("left operation should not be rebased")
	}

	rightResult, err := store.Submit(ctx, "doc-1", 0, right, "right-client")
	if err != nil {
		t.Fatalf("submit right failed: %v", err)
	}
	if rightResult.Version != 2 {
		t.Fatalf("unexpected right version: got %d want 2", rightResult.Version)
	}
	if !rightResult.Rebased {
		t.Fatalf("right operation should be rebased")
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-1")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}

	if got := string(snapshot.Document); got != `{"counter":3}` {
		t.Fatalf("unexpected final document: got %s want %s", got, `{"counter":3}`)
	}
}

func TestStorePageBlocksConcurrentInsertions(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	base := pageDocument(
		block("welcome-block", "欢迎来到 jsonot + BlockNote 协同编辑示例"),
	)
	_, err := store.CreateDocument(ctx, "doc-page-blocks", base)
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	leftOp := json.RawMessage(`[{"p":["blocks",1],"li":{"id":"left-block","type":"paragraph","content":[{"type":"text","text":"左侧新增段落","styles":{}}]}}]`)
	rightOp := json.RawMessage(`[{"p":["blocks",1],"li":{"id":"right-block","type":"paragraph","content":[{"type":"text","text":"右侧新增段落","styles":{}}]}}]`)

	leftResult, err := store.Submit(ctx, "doc-page-blocks", 0, leftOp, "left-client")
	if err != nil {
		t.Fatalf("submit left failed: %v", err)
	}
	if leftResult.Version != 1 || leftResult.Rebased {
		t.Fatalf("unexpected left submit result: %+v", leftResult)
	}

	rightResult, err := store.Submit(ctx, "doc-page-blocks", 0, rightOp, "right-client")
	if err != nil {
		t.Fatalf("submit right failed: %v", err)
	}
	if rightResult.Version != 2 || !rightResult.Rebased {
		t.Fatalf("unexpected right submit result: %+v", rightResult)
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-page-blocks")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}

	assertPageBlockOrder(t, snapshot.Document, "welcome-block", "right-block", "left-block")
	assertPageBlockText(t, snapshot.Document, "left-block", "左侧新增段落")
	assertPageBlockText(t, snapshot.Document, "right-block", "右侧新增段落")
}

func TestStorePageBlocksConcurrentInsertAndModify(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	base := pageDocument(
		block("welcome-block", "欢迎来到 jsonot + BlockNote 协同编辑示例"),
	)
	_, err := store.CreateDocument(ctx, "doc-page-mixed", base)
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	insertOp := json.RawMessage(`[{"p":["blocks",1],"li":{"id":"appendix-block","type":"paragraph","content":[{"type":"text","text":"追加说明","styles":{}}]}}]`)
	modifyOp := json.RawMessage(`[{"p":["blocks",0,"content",0,"text"],"od":"欢迎来到 jsonot + BlockNote 协同编辑示例","oi":"欢迎来到 jsonot + BlockNote 协同编辑示例（已修改）"}]`)

	insertResult, err := store.Submit(ctx, "doc-page-mixed", 0, insertOp, "insert-client")
	if err != nil {
		t.Fatalf("submit insert failed: %v", err)
	}
	if insertResult.Version != 1 || insertResult.Rebased {
		t.Fatalf("unexpected insert submit result: %+v", insertResult)
	}

	modifyResult, err := store.Submit(ctx, "doc-page-mixed", 0, modifyOp, "modify-client")
	if err != nil {
		t.Fatalf("submit modify failed: %v", err)
	}
	if modifyResult.Version != 2 || !modifyResult.Rebased {
		t.Fatalf("unexpected modify submit result: %+v", modifyResult)
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-page-mixed")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}

	assertPageBlockOrder(t, snapshot.Document, "welcome-block", "appendix-block")
	assertPageBlockText(t, snapshot.Document, "welcome-block", "欢迎来到 jsonot + BlockNote 协同编辑示例（已修改）")
	assertPageBlockText(t, snapshot.Document, "appendix-block", "追加说明")
}

func TestStorePageBlocksConcurrentModifySameBlock(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	base := pageDocument(
		block("welcome-block", "hello"),
	)
	_, err := store.CreateDocument(ctx, "doc-page-same-block", base)
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	leftOp := json.RawMessage(`[{"p":["blocks",0,"content",0,"text"],"od":"hello","oi":"hello left"}]`)
	rightOp := json.RawMessage(`[{"p":["blocks",0,"content",0,"text"],"od":"hello","oi":"hello right"}]`)

	if _, err := store.Submit(ctx, "doc-page-same-block", 0, leftOp, "left-client"); err != nil {
		t.Fatalf("submit left failed: %v", err)
	}
	rightResult, err := store.Submit(ctx, "doc-page-same-block", 0, rightOp, "right-client")
	if err != nil {
		t.Fatalf("submit right failed: %v", err)
	}
	if !rightResult.Rebased || !sameJSON(t, rightResult.Operation, json.RawMessage(`[{"p":["blocks",0,"content",0,"text"],"od":"hello left","oi":"hello right"}]`)) {
		t.Fatalf("unexpected rebased right operation: %+v", rightResult)
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-page-same-block")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	assertPageBlockText(t, snapshot.Document, "welcome-block", "hello right")
}

func TestSubmitRebasesAcrossMultipleMissedOps(t *testing.T) {
	ctx := context.Background()
	backend := &countingBackend{MemoryBackend: NewMemoryBackend()}
	store := NewServer(backend, NewMemoryLocker())

	_, err := store.CreateDocument(ctx, "doc-stale-gap", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	for i, delta := range []int{1, 2, 3} {
		if _, err := store.Submit(ctx, "doc-stale-gap", i, json.RawMessage(fmt.Sprintf(`[{"p":["counter"],"na":%d}]`, delta)), fmt.Sprintf("writer-%d", i+1)); err != nil {
			t.Fatalf("seed submit %d failed: %v", i+1, err)
		}
	}

	result, err := store.Submit(ctx, "doc-stale-gap", 0, json.RawMessage(`[{"p":["counter"],"na":10}]`), "stale-client")
	if err != nil {
		t.Fatalf("submit stale op failed: %v", err)
	}
	if !result.Rebased || result.Version != 4 {
		t.Fatalf("unexpected stale submit result: %+v", result)
	}
	if backend.getOpsCalls != 1 || backend.lastGetOpsFrom != 0 || backend.lastGetOpsTo != 3 {
		t.Fatalf("unexpected GetOps usage: calls=%d from=%d to=%d", backend.getOpsCalls, backend.lastGetOpsFrom, backend.lastGetOpsTo)
	}
	if !sameJSON(t, result.Operation, json.RawMessage(`[{"p":["counter"],"na":10}]`)) {
		t.Fatalf("unexpected transformed op: %s", result.Operation)
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-stale-gap")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if got := string(snapshot.Document); got != `{"counter":16}` {
		t.Fatalf("unexpected final document: %s", snapshot.Document)
	}

	ops, err := store.GetOperations(ctx, "doc-stale-gap", 3, 4)
	if err != nil {
		t.Fatalf("get operations failed: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("unexpected op count: got %d want 1", len(ops))
	}
	if ops[0].BaseVersion != 0 || !sameJSON(t, ops[0].SubmittedOp, json.RawMessage(`[{"p":["counter"],"na":10}]`)) || !sameJSON(t, ops[0].Op, result.Operation) {
		t.Fatalf("unexpected stored stale op record: %+v", ops[0])
	}
}

func TestSubmitStaleDeleteBecomesNoopAfterConcurrentDelete(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	base := pageDocument(
		block("welcome-block", "hello"),
		block("obsolete-block", "bye"),
	)
	_, err := store.CreateDocument(ctx, "doc-stale-noop", base)
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	deleteOp := json.RawMessage(`[{"p":["blocks",1],"ld":{"id":"obsolete-block","type":"paragraph","content":[{"type":"text","text":"bye","styles":{}}]}}]`)
	if _, err := store.Submit(ctx, "doc-stale-noop", 0, deleteOp, "deleter-a"); err != nil {
		t.Fatalf("first delete failed: %v", err)
	}

	result, err := store.Submit(ctx, "doc-stale-noop", 0, deleteOp, "deleter-b")
	if err != nil {
		t.Fatalf("second stale delete failed: %v", err)
	}
	if !result.Rebased || result.Version != 1 || !sameJSON(t, result.Operation, json.RawMessage(`[]`)) {
		t.Fatalf("unexpected noop stale delete result: %+v", result)
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-stale-noop")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	assertPageBlockOrder(t, snapshot.Document, "welcome-block")

	ops, err := store.GetOperations(ctx, "doc-stale-noop", 0, 1)
	if err != nil {
		t.Fatalf("get operations failed: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("noop stale delete should not append op, got %d committed ops", len(ops))
	}
}

func TestSubmitRejectsStaleBasePastRebaseWindow(t *testing.T) {
	ctx := context.Background()
	backend := &countingBackend{MemoryBackend: NewMemoryBackend()}
	store := NewServer(backend, NewMemoryLocker(), WithMaxRebaseGap(2))

	_, err := store.CreateDocument(ctx, "doc-stale-window", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := store.Submit(ctx, "doc-stale-window", i, json.RawMessage(`[{"p":["counter"],"na":1}]`), fmt.Sprintf("writer-%d", i+1)); err != nil {
			t.Fatalf("seed submit %d failed: %v", i+1, err)
		}
	}

	_, err = store.Submit(ctx, "doc-stale-window", 0, json.RawMessage(`[{"p":["counter"],"na":10}]`), "stale-client")
	if err == nil {
		t.Fatalf("expected stale submit to require resync")
	}
	if !errors.Is(err, ErrStaleResyncRequired) {
		t.Fatalf("expected ErrStaleResyncRequired, got %v", err)
	}
	var staleErr *StaleSubmitError
	if !errors.As(err, &staleErr) {
		t.Fatalf("expected StaleSubmitError, got %T", err)
	}
	if staleErr.BaseVersion != 0 || staleErr.CurrentVersion != 3 || staleErr.MinSupportedVersion != 1 || staleErr.MaxRebaseGap != 2 {
		t.Fatalf("unexpected stale error details: %+v", staleErr)
	}
	if backend.getOpsCalls != 0 {
		t.Fatalf("GetOps should not run when rebase window is exceeded, got %d calls", backend.getOpsCalls)
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-stale-window")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if got := string(snapshot.Document); got != `{"counter":3}` {
		t.Fatalf("unexpected document after rejected stale submit: %s", snapshot.Document)
	}
	ops, err := store.GetOperations(ctx, "doc-stale-window", 0, 3)
	if err != nil {
		t.Fatalf("get operations failed: %v", err)
	}
	if len(ops) != 3 {
		t.Fatalf("rejected stale submit should not append op, got %d committed ops", len(ops))
	}
}

func TestSubmitAllowsStaleBaseAtRebaseWindowBoundary(t *testing.T) {
	ctx := context.Background()
	backend := &countingBackend{MemoryBackend: NewMemoryBackend()}
	store := NewServer(backend, NewMemoryLocker(), WithMaxRebaseGap(2))

	_, err := store.CreateDocument(ctx, "doc-stale-boundary", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := store.Submit(ctx, "doc-stale-boundary", i, json.RawMessage(`[{"p":["counter"],"na":1}]`), fmt.Sprintf("writer-%d", i+1)); err != nil {
			t.Fatalf("seed submit %d failed: %v", i+1, err)
		}
	}

	result, err := store.Submit(ctx, "doc-stale-boundary", 0, json.RawMessage(`[{"p":["counter"],"na":10}]`), "stale-client")
	if err != nil {
		t.Fatalf("boundary stale submit failed: %v", err)
	}
	if !result.Rebased || result.Version != 3 {
		t.Fatalf("unexpected boundary stale submit result: %+v", result)
	}
	if backend.getOpsCalls != 1 || backend.lastGetOpsFrom != 0 || backend.lastGetOpsTo != 2 {
		t.Fatalf("unexpected GetOps usage at boundary: calls=%d from=%d to=%d", backend.getOpsCalls, backend.lastGetOpsFrom, backend.lastGetOpsTo)
	}
}

func TestStaleSubmitErrorExposesReason(t *testing.T) {
	err := &StaleSubmitError{Reason: StaleReasonVersionBehindWindow, DocumentID: "doc-1", BaseVersion: 1, CurrentVersion: 5, MinSupportedVersion: 3, MaxRebaseGap: 2}
	if err.Reason != StaleReasonVersionBehindWindow {
		t.Fatalf("unexpected stale reason: got %q want %q", err.Reason, StaleReasonVersionBehindWindow)
	}
}

func TestResyncRequiredMessageIncludesReasonAndVersionMetadata(t *testing.T) {
	ctx := context.Background()
	mailbox := NewMemoryMailboxStore()
	if err := mailbox.Append(ctx, testEnvelope("env-1", "sess-1", 1)); err != nil {
		t.Fatalf("append envelope failed: %v", err)
	}

	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 4)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	if err := sendConnectHandshake(ctx, conn, mailbox, ConnectRequest{
		Type:           MessageTypeConnect,
		SessionID:      "sess-1",
		DocumentID:     "doc-1",
		LastEnvelopeID: "env-missing",
	}); err != nil {
		t.Fatalf("send connect handshake failed: %v", err)
	}

	var connected ConnectedMessage
	readClientJSON(t, clientWS, &connected)
	var resync ResyncRequiredMessage
	readClientJSON(t, clientWS, &resync)
	if resync.Type != MessageTypeResyncRequired {
		t.Fatalf("unexpected resync type: %s", resync.Type)
	}
	if resync.Reason != StaleReasonReplayCursorNotFound {
		t.Fatalf("unexpected resync reason: got %q want %q", resync.Reason, StaleReasonReplayCursorNotFound)
	}
	if resync.CurrentVersion != 0 || resync.MinSupportedVersion != 0 || resync.MaxRebaseGap != 0 {
		t.Fatalf("unexpected resync metadata: %+v", resync)
	}
}

func TestGetSnapshotAtReturnsHistoricalVersion(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()
	_, err := store.CreateDocument(ctx, "doc-snapshot-at", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	for version, delta := range []int{1, 2, 3} {
		op := json.RawMessage(fmt.Sprintf(`[{"p":["counter"],"na":%d}]`, delta))
		if _, err := store.Submit(ctx, "doc-snapshot-at", version, op, fmt.Sprintf("writer-%d", version+1)); err != nil {
			t.Fatalf("seed submit %d failed: %v", version+1, err)
		}
	}

	snapshot, err := store.GetSnapshotAt(ctx, "doc-snapshot-at", 2)
	if err != nil {
		t.Fatalf("GetSnapshotAt failed: %v", err)
	}
	if snapshot.Version != 2 {
		t.Fatalf("unexpected snapshot version: got %d want 2", snapshot.Version)
	}
	if got := string(snapshot.Document); got != `{"counter":3}` {
		t.Fatalf("unexpected historical snapshot: %s", snapshot.Document)
	}
}

func TestRollbackToVersionCommitsInverseOperation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()
	_, err := store.CreateDocument(ctx, "doc-rollback", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	if _, err := store.Submit(ctx, "doc-rollback", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "writer-1"); err != nil {
		t.Fatalf("submit first failed: %v", err)
	}
	if _, err := store.Submit(ctx, "doc-rollback", 1, json.RawMessage(`[{"p":["counter"],"na":2}]`), "writer-2"); err != nil {
		t.Fatalf("submit second failed: %v", err)
	}

	result, err := store.RollbackToVersion(ctx, "doc-rollback", 1, "rollback-bot")
	if err != nil {
		t.Fatalf("RollbackToVersion failed: %v", err)
	}
	if result.Version != 3 {
		t.Fatalf("unexpected rollback version: got %d want 3", result.Version)
	}
	if !sameJSON(t, result.Operation, json.RawMessage(`[{"p":["counter"],"na":-2}]`)) {
		t.Fatalf("unexpected rollback operation: %s", result.Operation)
	}
	if got := string(result.Document); got != `{"counter":1}` {
		t.Fatalf("unexpected rollback document: %s", result.Document)
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-rollback")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if got := string(snapshot.Document); got != `{"counter":1}` {
		t.Fatalf("unexpected current snapshot after rollback: %s", snapshot.Document)
	}

	historical, err := store.GetSnapshotAt(ctx, "doc-rollback", 1)
	if err != nil {
		t.Fatalf("GetSnapshotAt after rollback failed: %v", err)
	}
	if !sameJSON(t, historical.Document, snapshot.Document) {
		t.Fatalf("rollback target snapshot mismatch")
	}
}

func TestStoreSubscribe(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-2", json.RawMessage(`{"title":"hello"}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	events, cancel, err := store.Subscribe(ctx, "doc-2", 1)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer cancel()

	op := json.RawMessage(`[{"p":["title"],"od":"hello","oi":"world"}]`)
	_, err = store.Submit(ctx, "doc-2", 0, op, "client-a")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	select {
	case event := <-events:
		if event.DocumentID != "doc-2" {
			t.Fatalf("unexpected document id: %s", event.DocumentID)
		}
		if event.Type != EventTypeOp {
			t.Fatalf("unexpected event type: got %q want %q", event.Type, EventTypeOp)
		}
		if event.Version != 1 {
			t.Fatalf("unexpected version: got %d want 1", event.Version)
		}
		if event.Source != "client-a" {
			t.Fatalf("unexpected source: %s", event.Source)
		}
		if got := string(event.Document); got != `{"title":"world"}` {
			t.Fatalf("unexpected document: got %s", got)
		}
	default:
		t.Fatalf("expected one event")
	}
}

func TestStoreInvalidVersion(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-3", json.RawMessage(`{"ok":true}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	_, err = store.Submit(ctx, "doc-3", 5, json.RawMessage(`[]`), "")
	if err == nil {
		t.Fatalf("expected invalid version error")
	}
}

func TestDeleteDocumentRemovesSnapshotAndPublishesDelete(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-delete", json.RawMessage(`{"title":"bye"}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	events, cancel, err := store.Subscribe(ctx, "doc-delete", 1)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer cancel()

	if err := store.DeleteDocument(ctx, "doc-delete", 0, "client-a"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := store.GetSnapshot(ctx, "doc-delete"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("expected document not found after delete, got %v", err)
	}
	if _, _, err := store.Subscribe(ctx, "doc-delete", 1); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("expected subscribe on deleted doc to fail with not found, got %v", err)
	}

	select {
	case event := <-events:
		if event.Type != EventTypeDelete {
			t.Fatalf("unexpected event type: got %q want %q", event.Type, EventTypeDelete)
		}
		if event.DocumentID != "doc-delete" || event.Version != 0 || event.Source != "client-a" {
			t.Fatalf("unexpected delete event: %+v", event)
		}
	default:
		t.Fatalf("expected delete event")
	}
}

func TestDeleteDocumentRejectsStaleVersion(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-delete-version", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	if _, err := store.Submit(ctx, "doc-delete-version", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "client-a"); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	err = store.DeleteDocument(ctx, "doc-delete-version", 0, "client-a")
	if !errors.Is(err, ErrInvalidVersion) {
		t.Fatalf("expected invalid version error, got %v", err)
	}
	snapshot, err := store.GetSnapshot(ctx, "doc-delete-version")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if snapshot.Version != 1 {
		t.Fatalf("stale delete changed snapshot version: got %d", snapshot.Version)
	}
}

func TestStoreGetOperationsReturnsCommittedHistoryRange(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-history", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	first := json.RawMessage(`[{"p":["counter"],"na":1}]`)
	second := json.RawMessage(`[{"p":["counter"],"na":2}]`)
	third := json.RawMessage(`[{"p":["counter"],"na":3}]`)

	if _, err := store.SubmitWithRequest(ctx, SubmitRequest{DocumentID: "doc-history", BaseVersion: 0, Operation: first, Source: "a", Sequence: 1}); err != nil {
		t.Fatalf("submit first failed: %v", err)
	}
	if _, err := store.SubmitWithRequest(ctx, SubmitRequest{DocumentID: "doc-history", BaseVersion: 1, Operation: second, Source: "b", Sequence: 2}); err != nil {
		t.Fatalf("submit second failed: %v", err)
	}
	if _, err := store.SubmitWithRequest(ctx, SubmitRequest{DocumentID: "doc-history", BaseVersion: 2, Operation: third, Source: "c", Sequence: 3}); err != nil {
		t.Fatalf("submit third failed: %v", err)
	}

	ops, err := store.GetOperations(ctx, "doc-history", 1, 3)
	if err != nil {
		t.Fatalf("get operations failed: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("unexpected operation count: got %d want 2", len(ops))
	}
	if ops[0].Version != 2 || ops[0].BaseVersion != 1 || ops[0].Source != "b" || ops[0].ID.Source != "b" || ops[0].ID.Sequence != 2 || !sameJSON(t, ops[0].Op, second) || !sameJSON(t, ops[0].SubmittedOp, second) {
		t.Fatalf("unexpected first history op: %+v", ops[0])
	}
	if ops[1].Version != 3 || ops[1].BaseVersion != 2 || ops[1].Source != "c" || ops[1].ID.Source != "c" || ops[1].ID.Sequence != 3 || !sameJSON(t, ops[1].Op, third) || !sameJSON(t, ops[1].SubmittedOp, third) {
		t.Fatalf("unexpected second history op: %+v", ops[1])
	}
}

func TestSubmitWithRequestDeduplicatesClientSequence(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-dedupe", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	req := SubmitRequest{
		DocumentID:  "doc-dedupe",
		BaseVersion: 0,
		Operation:   json.RawMessage(`[{"p":["counter"],"na":1}]`),
		Source:      "client-a",
		Sequence:    7,
	}

	first, err := store.SubmitWithRequest(ctx, req)
	if err != nil {
		t.Fatalf("first submit failed: %v", err)
	}
	if first.Version != 1 || first.Duplicate {
		t.Fatalf("unexpected first result: %+v", first)
	}

	if _, err := store.Submit(ctx, "doc-dedupe", 1, json.RawMessage(`[{"p":["counter"],"na":2}]`), "client-b"); err != nil {
		t.Fatalf("interleaved submit failed: %v", err)
	}

	retry, err := store.SubmitWithRequest(ctx, req)
	if err != nil {
		t.Fatalf("retry submit failed: %v", err)
	}
	if retry.Version != 2 || !retry.Duplicate {
		t.Fatalf("unexpected retry result: %+v", retry)
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-dedupe")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if got := string(snapshot.Document); got != `{"counter":3}` {
		t.Fatalf("duplicate submit should not apply twice: got %s", got)
	}
	if !sameJSON(t, retry.Document, snapshot.Document) {
		t.Fatalf("duplicate result should return the current snapshot document")
	}
}

func TestSubmitWithRequestRejectsSequenceReuseWithDifferentOperation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-seq-conflict", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	first := SubmitRequest{
		DocumentID:  "doc-seq-conflict",
		BaseVersion: 0,
		Operation:   json.RawMessage(`[{"p":["counter"],"na":1}]`),
		Source:      "client-a",
		Sequence:    7,
	}
	if _, err := store.SubmitWithRequest(ctx, first); err != nil {
		t.Fatalf("first submit failed: %v", err)
	}

	conflict := first
	conflict.Operation = json.RawMessage(`[{"p":["counter"],"na":2}]`)
	_, err = store.SubmitWithRequest(ctx, conflict)
	if err == nil {
		t.Fatalf("expected sequence conflict error")
	}
}

func TestSubmitWithRequestPublishesSequence(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()

	_, err := store.CreateDocument(ctx, "doc-event-seq", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	events, cancel, err := store.Subscribe(ctx, "doc-event-seq", 1)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer cancel()

	_, err = store.SubmitWithRequest(ctx, SubmitRequest{
		DocumentID:  "doc-event-seq",
		BaseVersion: 0,
		Operation:   json.RawMessage(`[{"p":["counter"],"na":1}]`),
		Source:      "client-a",
		Sequence:    42,
	})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	select {
	case event := <-events:
		if event.Source != "client-a" || event.Sequence != 42 || event.ID.Source != "client-a" || event.ID.Sequence != 42 {
			t.Fatalf("unexpected event identity: %+v", event)
		}
	default:
		t.Fatalf("expected one event")
	}
}

func TestSubmitMiddlewareCanRejectBeforeCommit(t *testing.T) {
	ctx := context.Background()
	rejected := errors.New("reject submit")
	store := NewServer(
		NewMemoryBackend(),
		NewMemoryLocker(),
		WithSubmitMiddleware(func(next SubmitHandler) SubmitHandler {
			return func(ctx context.Context, req SubmitRequest) (SubmitResult, error) {
				return SubmitResult{}, rejected
			}
		}),
	)

	_, err := store.CreateDocument(ctx, "doc-middleware-reject", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	_, err = store.Submit(ctx, "doc-middleware-reject", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "client-a")
	if !errors.Is(err, rejected) {
		t.Fatalf("expected rejection error, got %v", err)
	}

	snapshot, err := store.GetSnapshot(ctx, "doc-middleware-reject")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if snapshot.Version != 0 || string(snapshot.Document) != `{"counter":0}` {
		t.Fatalf("submit should not commit after middleware rejection: %+v", snapshot)
	}

	ops, err := store.GetOperations(ctx, "doc-middleware-reject", 0, 0)
	if err != nil {
		t.Fatalf("get operations failed: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("expected no committed ops after rejection, got %d", len(ops))
	}
}

func TestNewServerPanicsWhenSubmitMiddlewareReturnsNilHandler(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected NewServer to panic when middleware returns nil handler")
		}
		if msg := r.(string); msg != "sharedb: submit middleware returned nil handler" {
			t.Fatalf("unexpected panic message: %v", r)
		}
	}()

	_ = NewServer(
		NewMemoryBackend(),
		NewMemoryLocker(),
		WithSubmitMiddleware(func(next SubmitHandler) SubmitHandler {
			return nil
		}),
	)
}

func TestSubmitMiddlewareCanRewriteRequestAndObserveResult(t *testing.T) {
	ctx := context.Background()
	var calls []string
	store := NewServer(
		NewMemoryBackend(),
		NewMemoryLocker(),
		WithSubmitMiddleware(
			func(next SubmitHandler) SubmitHandler {
				return func(ctx context.Context, req SubmitRequest) (SubmitResult, error) {
					calls = append(calls, "outer-before")
					result, err := next(ctx, req)
					calls = append(calls, "outer-after")
					return result, err
				}
			},
			func(next SubmitHandler) SubmitHandler {
				return func(ctx context.Context, req SubmitRequest) (SubmitResult, error) {
					calls = append(calls, "inner-before")
					req.Source = "middleware-client"
					result, err := next(ctx, req)
					if err == nil {
						calls = append(calls, "inner-after")
					}
					return result, err
				}
			},
		),
	)

	_, err := store.CreateDocument(ctx, "doc-middleware-wrap", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	ch, cancel, err := store.Subscribe(ctx, "doc-middleware-wrap", 1)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer cancel()

	result, err := store.Submit(ctx, "doc-middleware-wrap", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "client-a")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	if result.Version != 1 || string(result.Document) != `{"counter":1}` {
		t.Fatalf("unexpected submit result: %+v", result)
	}
	wantCalls := []string{"outer-before", "inner-before", "inner-after", "outer-after"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected middleware call order: got %v want %v", calls, wantCalls)
	}

	ops, err := store.GetOperations(ctx, "doc-middleware-wrap", 0, 1)
	if err != nil {
		t.Fatalf("get operations failed: %v", err)
	}
	if len(ops) != 1 || ops[0].Source != "middleware-client" {
		t.Fatalf("middleware should rewrite request before commit, got %+v", ops)
	}

	select {
	case event := <-ch:
		if event.Source != "middleware-client" {
			t.Fatalf("middleware-rewritten source should be published, got %+v", event)
		}
	default:
		t.Fatalf("expected submit event")
	}
}

func TestGetOperationsRejectsInvalidRanges(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()
	_, err := store.CreateDocument(ctx, "doc-invalid-range", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	if _, err := store.Submit(ctx, "doc-invalid-range", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "client-a"); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	cases := []struct{ from, to int }{{-1, 0}, {1, 0}, {0, 2}}
	for _, tc := range cases {
		if _, err := store.GetOperations(ctx, "doc-invalid-range", tc.from, tc.to); err == nil {
			t.Fatalf("expected invalid range error for [%d,%d]", tc.from, tc.to)
		}
	}

	ops, err := store.GetOperations(ctx, "doc-invalid-range", 1, 1)
	if err != nil {
		t.Fatalf("empty range should be valid: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("empty range returned %d ops", len(ops))
	}
}

func TestIdentityOmittedFromJSONWhenUnset(t *testing.T) {
	eventRaw, err := json.Marshal(Event{DocumentID: "doc", Version: 1, Operation: json.RawMessage(`[]`), Document: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("marshal event failed: %v", err)
	}
	if strings.Contains(string(eventRaw), `"id"`) || strings.Contains(string(eventRaw), `"seq"`) || strings.Contains(string(eventRaw), `"source"`) {
		t.Fatalf("unset event identity should be omitted, got %s", eventRaw)
	}

	recordRaw, err := json.Marshal(OpRecord{DocumentID: "doc", Version: 1, Op: json.RawMessage(`[]`)})
	if err != nil {
		t.Fatalf("marshal op record failed: %v", err)
	}
	if strings.Contains(string(recordRaw), `"id"`) || strings.Contains(string(recordRaw), `"seq"`) || strings.Contains(string(recordRaw), `"source"`) {
		t.Fatalf("unset op identity should be omitted, got %s", recordRaw)
	}
}

func TestMemoryPublisherDeliversIndependentEventPayloads(t *testing.T) {
	ctx := context.Background()
	pub := NewMemoryPublisher()
	left, cancelLeft, err := pub.Subscribe(ctx, "doc-pub", 1)
	if err != nil {
		t.Fatalf("subscribe left failed: %v", err)
	}
	defer cancelLeft()
	right, cancelRight, err := pub.Subscribe(ctx, "doc-pub", 1)
	if err != nil {
		t.Fatalf("subscribe right failed: %v", err)
	}
	defer cancelRight()

	pub.Publish(ctx, Event{DocumentID: "doc-pub", Version: 1, Operation: json.RawMessage(`[{"p":["x"],"na":1}]`), Document: json.RawMessage(`{"x":1}`)})
	leftEvent := <-left
	leftEvent.Operation[0] = 'X'
	leftEvent.Document[0] = 'X'
	rightEvent := <-right
	if string(rightEvent.Operation) != `[{"p":["x"],"na":1}]` || string(rightEvent.Document) != `{"x":1}` {
		t.Fatalf("subscriber payloads should be independent, got op=%s doc=%s", rightEvent.Operation, rightEvent.Document)
	}
}

func BenchmarkSubmitStaleBaseGap(b *testing.B) {
	for _, gap := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprintf("counter-gap-%d", gap), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ctx := context.Background()
				store := NewMemoryServer()
				docID := fmt.Sprintf("doc-counter-%d", i)
				if _, err := store.CreateDocument(ctx, docID, json.RawMessage(`{"counter":0}`)); err != nil {
					b.Fatalf("create document failed: %v", err)
				}
				for version := 0; version < gap; version++ {
					op := json.RawMessage(`[{"p":["counter"],"na":1}]`)
					if _, err := store.Submit(ctx, docID, version, op, fmt.Sprintf("writer-%d", version)); err != nil {
						b.Fatalf("seed submit failed: %v", err)
					}
				}
				b.StartTimer()
				_, err := store.Submit(ctx, docID, 0, json.RawMessage(`[{"p":["counter"],"na":10}]`), "stale-client")
				b.StopTimer()
				if err != nil {
					b.Fatalf("stale submit failed: %v", err)
				}
			}
		})

		b.Run(fmt.Sprintf("page-block-gap-%d", gap), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ctx := context.Background()
				store := NewMemoryServer()
				docID := fmt.Sprintf("doc-page-%d", i)
				if _, err := store.CreateDocument(ctx, docID, pageDocument(block("welcome-block", "hello"))); err != nil {
					b.Fatalf("create document failed: %v", err)
				}
				for version := 0; version < gap; version++ {
					op := json.RawMessage(fmt.Sprintf(`[{"p":["blocks",1],"li":{"id":"seed-%d","type":"paragraph","content":[{"type":"text","text":"seed-%d","styles":{}}]}}]`, version, version))
					if _, err := store.Submit(ctx, docID, version, op, fmt.Sprintf("writer-%d", version)); err != nil {
						b.Fatalf("seed submit failed: %v", err)
					}
				}
				staleOp := json.RawMessage(`[{"p":["blocks",0,"content",0,"text"],"od":"hello","oi":"hello stale"}]`)
				b.StartTimer()
				_, err := store.Submit(ctx, docID, 0, staleOp, "stale-client")
				b.StopTimer()
				if err != nil {
					b.Fatalf("stale submit failed: %v", err)
				}
			}
		})
	}
}

type countingBackend struct {
	*MemoryBackend
	getOpsCalls    int
	lastGetOpsFrom int
	lastGetOpsTo   int
}

func (b *countingBackend) GetOps(ctx context.Context, docID string, fromVersion, toVersion int) ([]OpRecord, error) {
	b.getOpsCalls++
	b.lastGetOpsFrom = fromVersion
	b.lastGetOpsTo = toVersion
	return b.MemoryBackend.GetOps(ctx, docID, fromVersion, toVersion)
}

func pageDocument(blocks ...map[string]any) json.RawMessage {
	payload, err := json.Marshal(map[string]any{"blocks": blocks})
	if err != nil {
		panic(err)
	}
	return payload
}

func block(id, text string) map[string]any {
	return map[string]any{
		"id":   id,
		"type": "paragraph",
		"content": []any{
			map[string]any{
				"type":   "text",
				"text":   text,
				"styles": map[string]any{},
			},
		},
	}
}

func assertPageBlockOrder(t *testing.T, document json.RawMessage, wantIDs ...string) {
	t.Helper()
	got := extractPageBlockIDs(t, document)
	if !reflect.DeepEqual(got, wantIDs) {
		t.Fatalf("unexpected block order: got %v want %v", got, wantIDs)
	}
}

func assertPageBlockText(t *testing.T, document json.RawMessage, blockID, want string) {
	t.Helper()
	got := extractPageBlockTexts(t, document)
	if got[blockID] != want {
		t.Fatalf("unexpected block text for %s: got %q want %q", blockID, got[blockID], want)
	}
}

func extractPageBlockIDs(t *testing.T, document json.RawMessage) []string {
	t.Helper()
	type pageBlock struct {
		ID string `json:"id"`
	}
	type page struct {
		Blocks []pageBlock `json:"blocks"`
	}
	var decoded page
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("unmarshal page document failed: %v", err)
	}
	ids := make([]string, 0, len(decoded.Blocks))
	for _, blk := range decoded.Blocks {
		ids = append(ids, blk.ID)
	}
	return ids
}

func extractPageBlockTexts(t *testing.T, document json.RawMessage) map[string]string {
	t.Helper()
	type textNode struct {
		Text string `json:"text"`
	}
	type pageBlock struct {
		ID      string     `json:"id"`
		Content []textNode `json:"content"`
	}
	type page struct {
		Blocks []pageBlock `json:"blocks"`
	}
	var decoded page
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("unmarshal page document failed: %v", err)
	}
	texts := make(map[string]string, len(decoded.Blocks))
	for _, blk := range decoded.Blocks {
		if len(blk.Content) > 0 {
			texts[blk.ID] = blk.Content[0].Text
		}
	}
	return texts
}
