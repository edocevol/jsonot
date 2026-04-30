package sharedb

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHandleConnectRegistersSessionAndReplaysMissedEnvelopes(t *testing.T) {
	ctx := context.Background()
	mailbox := NewMemoryMailboxStore()
	for _, env := range []Envelope{
		testEnvelope("env-1", "sess-1", 1),
		testEnvelope("env-2", "sess-1", 2),
	} {
		if err := mailbox.Append(ctx, env); err != nil {
			t.Fatalf("append envelope failed: %v", err)
		}
	}
	if err := mailbox.AckThrough(ctx, "sess-1", "env-1"); err != nil {
		t.Fatalf("ack envelope failed: %v", err)
	}

	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 4)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	sessions := NewSessionManager()
	if err := HandleConnect(ctx, conn, sessions, mailbox, ConnectRequest{
		Type:           MessageTypeConnect,
		SessionID:      "sess-1",
		DocumentID:     "doc-1",
		LastEnvelopeID: "env-1",
	}); err != nil {
		t.Fatalf("handle connect failed: %v", err)
	}

	var connected ConnectedMessage
	readClientJSON(t, clientWS, &connected)
	if connected.Type != MessageTypeConnected {
		t.Fatalf("unexpected connected type: %s", connected.Type)
	}
	if connected.SessionID != "sess-1" || connected.DocumentID != "doc-1" || connected.LastAckedEnvelopeID != "env-1" {
		t.Fatalf("unexpected connected payload: %+v", connected)
	}

	var replay ReplayMessage
	readClientJSON(t, clientWS, &replay)
	if replay.Type != MessageTypeReplay {
		t.Fatalf("unexpected replay type: %s", replay.Type)
	}
	if len(replay.Envelopes) != 1 || replay.Envelopes[0].ID != "env-2" {
		t.Fatalf("unexpected replay payload: %+v", replay)
	}

	active, ok := sessions.Active("sess-1")
	if !ok || active != conn {
		t.Fatalf("expected active session to point at conn, got ok=%v active=%p", ok, active)
	}
}

func TestHandleConnectUnknownCursorRequestsResync(t *testing.T) {
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

	sessions := NewSessionManager()
	if err := HandleConnect(ctx, conn, sessions, mailbox, ConnectRequest{
		Type:           MessageTypeConnect,
		SessionID:      "sess-1",
		DocumentID:     "doc-1",
		LastEnvelopeID: "env-missing",
	}); err != nil {
		t.Fatalf("handle connect failed: %v", err)
	}

	var connected ConnectedMessage
	readClientJSON(t, clientWS, &connected)
	if connected.Type != MessageTypeConnected {
		t.Fatalf("unexpected connected type: %s", connected.Type)
	}

	var resync ResyncRequiredMessage
	readClientJSON(t, clientWS, &resync)
	if resync.Type != MessageTypeResyncRequired {
		t.Fatalf("unexpected resync type: %s", resync.Type)
	}
	if resync.SessionID != "sess-1" || resync.DocumentID != "doc-1" || resync.LastAckedEnvelopeID != "" {
		t.Fatalf("unexpected resync payload: %+v", resync)
	}
}

func TestHandleConnectRejectsInvalidRequest(t *testing.T) {
	ctx := context.Background()
	mailbox := NewMemoryMailboxStore()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 1)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	err := HandleConnect(ctx, conn, NewSessionManager(), mailbox, ConnectRequest{
		Type:       MessageTypeConnect,
		SessionID:  "",
		DocumentID: "doc-1",
	})
	if !errors.Is(err, ErrInvalidClientMessage) {
		t.Fatalf("expected invalid client message error, got %v", err)
	}
}

func TestHandleConnectDoesNotActivateSessionWhenReplayFails(t *testing.T) {
	ctx := context.Background()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 1)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	sessions := NewSessionManager()
	errReplay := errors.New("replay failed")
	err := HandleConnect(ctx, conn, sessions, replayFailingMailboxStore{err: errReplay}, ConnectRequest{
		Type:       MessageTypeConnect,
		SessionID:  "sess-1",
		DocumentID: "doc-1",
	})
	if !errors.Is(err, errReplay) {
		t.Fatalf("expected replay error, got %v", err)
	}
	if _, ok := sessions.Active("sess-1"); ok {
		t.Fatal("failed connect should not activate session")
	}
}

func TestAttachLiveSessionRelaysEventsToMailboxAndWebsocket(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()
	_, err := store.CreateDocument(ctx, "doc-live", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	mailbox := NewMemoryMailboxStore()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 8)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	if err := AttachLiveSession(ctx, conn, store, mailbox, "sess-live", "doc-live", func() string { return "env-live-1" }); err != nil {
		t.Fatalf("attach live session failed: %v", err)
	}

	_, err = store.Submit(ctx, "doc-live", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "client-a")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	var live EventMessage
	readClientJSON(t, clientWS, &live)
	if live.Type != MessageTypeEvent {
		t.Fatalf("unexpected event message type: %s", live.Type)
	}
	if live.SessionID != "sess-live" || live.Envelope.ID != "env-live-1" || live.Envelope.DocumentID != "doc-live" {
		t.Fatalf("unexpected live event payload: %+v", live)
	}

	replayed, err := mailbox.ReplayAfter(ctx, "sess-live", "", 0)
	if err != nil {
		t.Fatalf("replay mailbox failed: %v", err)
	}
	if len(replayed) != 1 || replayed[0].ID != "env-live-1" || replayed[0].Version != 1 {
		t.Fatalf("unexpected mailbox replay result: %+v", replayed)
	}
}

func TestAttachLiveSessionRejectsInvalidArguments(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()
	mailbox := NewMemoryMailboxStore()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 1)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	err := AttachLiveSession(ctx, conn, store, mailbox, "", "doc-live", func() string { return "env-1" })
	if !errors.Is(err, ErrInvalidClientMessage) {
		t.Fatalf("expected invalid client message error, got %v", err)
	}
}

func TestAttachLiveSessionRejectsClosedOrDuplicateAttachment(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryServer()
	_, err := store.CreateDocument(ctx, "doc-live-dup", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	mailbox := NewMemoryMailboxStore()

	closedServerWS, closedClientWS := newWebSocketPair(t)
	defer closedClientWS.Close()
	closedConn := NewClientConn(closedServerWS, 1)
	closedConn.Start(nil)
	closedConn.Close()
	closedConn.Wait()
	if err := AttachLiveSession(ctx, closedConn, store, mailbox, "sess-closed", "doc-live-dup", func() string { return "env-closed" }); !errors.Is(err, ErrInvalidClientMessage) {
		t.Fatalf("expected invalid client message for closed conn, got %v", err)
	}

	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 1)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	if err := AttachLiveSession(ctx, conn, store, mailbox, "sess-dup", "doc-live-dup", func() string { return "env-1" }); err != nil {
		t.Fatalf("first attach failed: %v", err)
	}
	if err := AttachLiveSession(ctx, conn, store, mailbox, "sess-dup", "doc-live-dup", func() string { return "env-2" }); !errors.Is(err, ErrInvalidClientMessage) {
		t.Fatalf("expected invalid client message for duplicate attach, got %v", err)
	}
}

func TestHandleAckEnvelopeAdvancesMailboxCursor(t *testing.T) {
	ctx := context.Background()
	mailbox := NewMemoryMailboxStore()
	for _, env := range []Envelope{
		testEnvelope("env-1", "sess-ack", 1),
		testEnvelope("env-2", "sess-ack", 2),
	} {
		if err := mailbox.Append(ctx, env); err != nil {
			t.Fatalf("append envelope failed: %v", err)
		}
	}

	if err := HandleAckEnvelope(ctx, mailbox, AckEnvelopeRequest{
		Type:       MessageTypeAckEnvelope,
		SessionID:  "sess-ack",
		EnvelopeID: "env-2",
	}); err != nil {
		t.Fatalf("handle ack envelope failed: %v", err)
	}

	acked, err := mailbox.LastAcked(ctx, "sess-ack")
	if err != nil {
		t.Fatalf("last acked failed: %v", err)
	}
	if acked != "env-2" {
		t.Fatalf("unexpected ack cursor: got %s want env-2", acked)
	}
}

func TestHandleAckEnvelopeRejectsInvalidRequest(t *testing.T) {
	err := HandleAckEnvelope(context.Background(), NewMemoryMailboxStore(), AckEnvelopeRequest{
		Type:       MessageTypeAckEnvelope,
		SessionID:  "",
		EnvelopeID: "env-1",
	})
	if !errors.Is(err, ErrInvalidClientMessage) {
		t.Fatalf("expected invalid client message error, got %v", err)
	}
}

func TestHandleAckEnvelopeReturnsEnvelopeNotFoundForUnknownCursor(t *testing.T) {
	ctx := context.Background()
	mailbox := NewMemoryMailboxStore()
	if err := mailbox.Append(ctx, testEnvelope("env-1", "sess-ack", 1)); err != nil {
		t.Fatalf("append envelope failed: %v", err)
	}

	err := HandleAckEnvelope(ctx, mailbox, AckEnvelopeRequest{
		Type:       MessageTypeAckEnvelope,
		SessionID:  "sess-ack",
		EnvelopeID: "env-missing",
	})
	if !errors.Is(err, ErrEnvelopeNotFound) {
		t.Fatalf("expected envelope not found error, got %v", err)
	}
}

func TestHandleSubmitSendsSubmitAckAndAppendsMailboxEnvelope(t *testing.T) {
	ctx := context.Background()
	store := NewServer(NewMemoryBackend(), NewMemoryLocker(), WithMaxRebaseGap(2))
	_, err := store.CreateDocument(ctx, "doc-submit", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	mailbox := NewMemoryMailboxStore()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 8)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	err = HandleSubmit(ctx, conn, store, mailbox, SubmitMessage{
		Type:       MessageTypeSubmit,
		SessionID:  "sess-submit",
		DocumentID: "doc-submit",
		Request: SubmitRequest{
			DocumentID:  "doc-submit",
			BaseVersion: 0,
			Operation:   json.RawMessage(`[{"p":["counter"],"na":1}]`),
			Source:      "client-a",
			Sequence:    1,
		},
	}, func() string { return "env-submit-1" })
	if err != nil {
		t.Fatalf("handle submit failed: %v", err)
	}

	var ack SubmitAckMessage
	readClientJSON(t, clientWS, &ack)
	if ack.Type != MessageTypeSubmitAck || ack.SessionID != "sess-submit" || ack.DocumentID != "doc-submit" {
		t.Fatalf("unexpected submit ack envelope: %+v", ack)
	}
	if ack.Result.Version != 1 || ack.Result.Rebased || ack.Envelope.ID != "env-submit-1" || ack.Envelope.Version != 1 {
		t.Fatalf("unexpected submit ack payload: %+v", ack)
	}

	replayed, err := mailbox.ReplayAfter(ctx, "sess-submit", "", 0)
	if err != nil {
		t.Fatalf("replay mailbox failed: %v", err)
	}
	if len(replayed) != 1 || replayed[0].ID != "env-submit-1" || replayed[0].Version != 1 {
		t.Fatalf("unexpected mailbox replay result: %+v", replayed)
	}
}

func TestHandleSubmitSendsSubmitErrorForStaleResyncRequired(t *testing.T) {
	ctx := context.Background()
	store := NewServer(NewMemoryBackend(), NewMemoryLocker(), WithMaxRebaseGap(1))
	_, err := store.CreateDocument(ctx, "doc-submit-stale", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := store.Submit(ctx, "doc-submit-stale", i, json.RawMessage(`[{"p":["counter"],"na":1}]`), "writer"); err != nil {
			t.Fatalf("seed submit %d failed: %v", i+1, err)
		}
	}
	mailbox := NewMemoryMailboxStore()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 8)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	err = HandleSubmit(ctx, conn, store, mailbox, SubmitMessage{
		Type:       MessageTypeSubmit,
		SessionID:  "sess-submit",
		DocumentID: "doc-submit-stale",
		Request: SubmitRequest{
			DocumentID:  "doc-submit-stale",
			BaseVersion: 0,
			Operation:   json.RawMessage(`[{"p":["counter"],"na":10}]`),
			Source:      "stale-client",
			Sequence:    9,
		},
	}, func() string { return "env-submit-stale" })
	if err != nil {
		t.Fatalf("handle submit should return nil after sending protocol error, got %v", err)
	}

	var msg SubmitErrorMessage
	readClientJSON(t, clientWS, &msg)
	if msg.Type != MessageTypeSubmitError || msg.Reason != StaleReasonVersionBehindWindow {
		t.Fatalf("unexpected submit error payload: %+v", msg)
	}
	if msg.CurrentVersion != 2 || msg.MinSupportedVersion != 1 || msg.MaxRebaseGap != 1 {
		t.Fatalf("unexpected submit error metadata: %+v", msg)
	}
}

func TestSessionInboundHandlerRoutesSubmitMessage(t *testing.T) {
	ctx := context.Background()
	store := NewServer(NewMemoryBackend(), NewMemoryLocker(), WithMaxRebaseGap(2))
	_, err := store.CreateDocument(ctx, "doc-dispatch-submit", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	mailbox := NewMemoryMailboxStore()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 8)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	sessions := NewSessionManager()
	if err := sessions.Register("sess-dispatch", conn); err != nil {
		t.Fatalf("register session failed: %v", err)
	}
	if err := AttachLiveSession(ctx, conn, store, mailbox, "sess-dispatch", "doc-dispatch-submit", func() string { return "env-live-unused" }); err != nil {
		t.Fatalf("attach live session failed: %v", err)
	}
	handler := NewSessionInboundHandler(store, mailbox, sessions, func() string { return "env-dispatch-1" })
	payload := []byte(`{"type":"submit","sessionId":"sess-dispatch","documentId":"doc-dispatch-submit","request":{"documentId":"doc-dispatch-submit","baseVersion":0,"op":[{"p":["counter"],"na":2}],"source":"client-a","seq":2}}`)
	if err := handler(ctx, conn, payload); err != nil {
		t.Fatalf("dispatcher submit failed: %v", err)
	}

	var ack SubmitAckMessage
	readClientJSON(t, clientWS, &ack)
	if ack.Type != MessageTypeSubmitAck || ack.Result.Version != 1 || ack.Envelope == nil || ack.Envelope.ID != "env-dispatch-1" {
		t.Fatalf("unexpected dispatcher submit ack: %+v", ack)
	}
}

func TestSessionInboundHandlerRejectsSubmitFromNonActiveSession(t *testing.T) {
	ctx := context.Background()
	store := NewServer(NewMemoryBackend(), NewMemoryLocker(), WithMaxRebaseGap(2))
	_, err := store.CreateDocument(ctx, "doc-dispatch-reject", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	mailbox := NewMemoryMailboxStore()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 8)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	handler := NewSessionInboundHandler(store, mailbox, NewSessionManager(), func() string { return "env-unused" })
	payload := []byte(`{"type":"submit","sessionId":"sess-other","documentId":"doc-dispatch-reject","request":{"documentId":"doc-dispatch-reject","baseVersion":0,"op":[{"p":["counter"],"na":2}],"source":"client-a","seq":2}}`)
	if err := handler(ctx, conn, payload); !errors.Is(err, ErrInvalidClientMessage) {
		t.Fatalf("expected invalid client message, got %v", err)
	}
}

func TestSessionInboundHandlerRejectsSubmitForUnattachedDocument(t *testing.T) {
	ctx := context.Background()
	store := NewServer(NewMemoryBackend(), NewMemoryLocker(), WithMaxRebaseGap(2))
	_, err := store.CreateDocument(ctx, "doc-dispatch-a", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document A failed: %v", err)
	}
	_, err = store.CreateDocument(ctx, "doc-dispatch-b", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document B failed: %v", err)
	}
	mailbox := NewMemoryMailboxStore()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 8)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	sessions := NewSessionManager()
	if err := sessions.Register("sess-attach", conn); err != nil {
		t.Fatalf("register session failed: %v", err)
	}
	if err := AttachLiveSession(ctx, conn, store, mailbox, "sess-attach", "doc-dispatch-a", func() string { return "env-a" }); err != nil {
		t.Fatalf("attach live session failed: %v", err)
	}
	handler := NewSessionInboundHandler(store, mailbox, sessions, func() string { return "env-unused" })
	payload := []byte(`{"type":"submit","sessionId":"sess-attach","documentId":"doc-dispatch-b","request":{"documentId":"doc-dispatch-b","baseVersion":0,"op":[{"p":["counter"],"na":2}],"source":"client-a","seq":2}}`)
	if err := handler(ctx, conn, payload); !errors.Is(err, ErrInvalidClientMessage) {
		t.Fatalf("expected unattached-document submit to be rejected, got %v", err)
	}
}

func TestHandleSubmitSkipsSyntheticEnvelopeForDuplicateRetry(t *testing.T) {
	ctx := context.Background()
	store := NewServer(NewMemoryBackend(), NewMemoryLocker())
	_, err := store.CreateDocument(ctx, "doc-submit-dup-ack", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	first := SubmitRequest{
		DocumentID:  "doc-submit-dup-ack",
		BaseVersion: 0,
		Operation:   json.RawMessage(`[{"p":["counter"],"na":1}]`),
		Source:      "sess-submit",
		Sequence:    3,
	}
	if _, err := store.SubmitWithRequest(ctx, first); err != nil {
		t.Fatalf("seed submit failed: %v", err)
	}
	mailbox := NewMemoryMailboxStore()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 8)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	err = HandleSubmit(ctx, conn, store, mailbox, SubmitMessage{
		Type:       MessageTypeSubmit,
		SessionID:  "sess-submit",
		DocumentID: "doc-submit-dup-ack",
		Request:    first,
	}, func() string { return "env-unused" })
	if err != nil {
		t.Fatalf("handle duplicate submit failed: %v", err)
	}
	var ack SubmitAckMessage
	readClientJSON(t, clientWS, &ack)
	if !ack.Result.Duplicate {
		t.Fatalf("expected duplicate submit ack, got %+v", ack)
	}
	if ack.Envelope != nil {
		t.Fatalf("duplicate retry should not emit synthetic or durable envelope: %+v", ack)
	}
	if replayed, err := mailbox.ReplayAfter(ctx, "sess-submit", "", 0); err != nil {
		t.Fatalf("replay mailbox failed: %v", err)
	} else if len(replayed) != 0 {
		t.Fatalf("duplicate retry should not append mailbox envelope: %+v", replayed)
	}
}

func TestHandleSubmitCanonicalizesSourceToSessionID(t *testing.T) {
	ctx := context.Background()
	store := NewServer(NewMemoryBackend(), NewMemoryLocker())
	_, err := store.CreateDocument(ctx, "doc-submit-source", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	mailbox := NewMemoryMailboxStore()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 8)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	err = HandleSubmit(ctx, conn, store, mailbox, SubmitMessage{
		Type:       MessageTypeSubmit,
		SessionID:  "sess-canonical",
		DocumentID: "doc-submit-source",
		Request: SubmitRequest{
			DocumentID:  "doc-submit-source",
			BaseVersion: 0,
			Operation:   json.RawMessage(`[{"p":["counter"],"na":1}]`),
			Source:      "spoofed-source",
			Sequence:    11,
		},
	}, func() string { return "env-canonical-1" })
	if err != nil {
		t.Fatalf("handle submit failed: %v", err)
	}
	var ack SubmitAckMessage
	readClientJSON(t, clientWS, &ack)
	ops, err := store.GetOperations(ctx, "doc-submit-source", 0, 1)
	if err != nil {
		t.Fatalf("get operations failed: %v", err)
	}
	if len(ops) != 1 || ops[0].Source != "sess-canonical" {
		t.Fatalf("expected committed source to be canonicalized to session ID, got %+v", ops)
	}
	if ack.Envelope == nil || ack.Envelope.Kind != EnvelopeKindAck || ack.Envelope.SessionID != "sess-canonical" {
		t.Fatalf("expected ack envelope to be session-bound ack envelope, got %+v", ack)
	}
}

func TestHandleSubmitOnLiveAttachmentSendsAckWithoutSelfEvent(t *testing.T) {
	ctx := context.Background()
	store := NewServer(NewMemoryBackend(), NewMemoryLocker())
	_, err := store.CreateDocument(ctx, "doc-submit-live", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	mailbox := NewMemoryMailboxStore()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 8)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	if err := AttachLiveSession(ctx, conn, store, mailbox, "sess-live-submit", "doc-submit-live", func() string { return "env-live-event" }); err != nil {
		t.Fatalf("attach live session failed: %v", err)
	}
	if err := HandleSubmit(ctx, conn, store, mailbox, SubmitMessage{
		Type:       MessageTypeSubmit,
		SessionID:  "sess-live-submit",
		DocumentID: "doc-submit-live",
		Request: SubmitRequest{
			DocumentID:  "doc-submit-live",
			BaseVersion: 0,
			Operation:   json.RawMessage(`[{"p":["counter"],"na":1}]`),
			Source:      "spoofed-source",
			Sequence:    1,
		},
	}, func() string { return "env-submit-ack" }); err != nil {
		t.Fatalf("handle submit failed: %v", err)
	}
	var ack SubmitAckMessage
	readClientJSON(t, clientWS, &ack)
	if ack.Type != MessageTypeSubmitAck || ack.Envelope == nil || ack.Envelope.Kind != EnvelopeKindAck {
		t.Fatalf("expected ack with durable ack envelope for live submit, got %+v", ack)
	}
	_ = clientWS.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = clientWS.ReadMessage()
	if !errors.Is(err, os.ErrDeadlineExceeded) && !websocket.IsUnexpectedCloseError(err) {
		if nerr, ok := err.(interface{ Timeout() bool }); !ok || !nerr.Timeout() {
			t.Fatalf("expected no self event after ack, got err=%v", err)
		}
	}
}

func TestHandleSubmitSendsSubmitErrorForInvalidVersion(t *testing.T) {
	ctx := context.Background()
	store := NewServer(NewMemoryBackend(), NewMemoryLocker())
	_, err := store.CreateDocument(ctx, "doc-submit-invalid-version", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	mailbox := NewMemoryMailboxStore()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 8)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	err = HandleSubmit(ctx, conn, store, mailbox, SubmitMessage{
		Type:       MessageTypeSubmit,
		SessionID:  "sess-submit",
		DocumentID: "doc-submit-invalid-version",
		Request: SubmitRequest{
			DocumentID:  "doc-submit-invalid-version",
			BaseVersion: 9,
			Operation:   json.RawMessage(`[{"p":["counter"],"na":1}]`),
			Source:      "client-a",
			Sequence:    1,
		},
	}, func() string { return "env-unused" })
	if err != nil {
		t.Fatalf("handle submit should return nil after sending protocol error, got %v", err)
	}

	var msg SubmitErrorMessage
	readClientJSON(t, clientWS, &msg)
	if msg.Type != MessageTypeSubmitError || msg.Code != SubmitErrorCodeInvalidVersion {
		t.Fatalf("unexpected invalid-version submit error payload: %+v", msg)
	}
}

func TestHandleSubmitSendsSubmitErrorForDuplicateSequenceConflict(t *testing.T) {
	ctx := context.Background()
	store := NewServer(NewMemoryBackend(), NewMemoryLocker())
	_, err := store.CreateDocument(ctx, "doc-submit-dup", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}
	first := SubmitRequest{
		DocumentID:  "doc-submit-dup",
		BaseVersion: 0,
		Operation:   json.RawMessage(`[{"p":["counter"],"na":1}]`),
		Source:      "sess-submit",
		Sequence:    7,
	}
	if _, err := store.SubmitWithRequest(ctx, first); err != nil {
		t.Fatalf("seed submit failed: %v", err)
	}
	mailbox := NewMemoryMailboxStore()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 8)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	err = HandleSubmit(ctx, conn, store, mailbox, SubmitMessage{
		Type:       MessageTypeSubmit,
		SessionID:  "sess-submit",
		DocumentID: "doc-submit-dup",
		Request: SubmitRequest{
			DocumentID:  "doc-submit-dup",
			BaseVersion: 0,
			Operation:   json.RawMessage(`[{"p":["counter"],"na":2}]`),
			Source:      "client-a",
			Sequence:    7,
		},
	}, func() string { return "env-unused" })
	if err != nil {
		t.Fatalf("handle submit should return nil after sending protocol error, got %v", err)
	}

	var msg SubmitErrorMessage
	readClientJSON(t, clientWS, &msg)
	if msg.Type != MessageTypeSubmitError || msg.Code != SubmitErrorCodeDuplicateSequenceConflict {
		t.Fatalf("unexpected duplicate-sequence submit error payload: %+v", msg)
	}
}

func readClientJSON(t *testing.T, conn *websocket.Conn, target any) {
	t.Helper()
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read websocket message failed: %v", err)
	}
	if err := json.Unmarshal(payload, target); err != nil {
		t.Fatalf("unmarshal websocket message failed: %v; payload=%s", err, payload)
	}
}

type replayFailingMailboxStore struct {
	err error
}

func (s replayFailingMailboxStore) Append(context.Context, Envelope) error {
	return nil
}

func (s replayFailingMailboxStore) ReplayAfter(context.Context, string, string, int) ([]Envelope, error) {
	return nil, s.err
}

func (s replayFailingMailboxStore) AckThrough(context.Context, string, string) error {
	return nil
}

func (s replayFailingMailboxStore) LastAcked(context.Context, string) (string, error) {
	return "", nil
}
