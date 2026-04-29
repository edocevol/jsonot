package sharedb

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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
