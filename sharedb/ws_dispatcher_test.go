package sharedb

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestSessionInboundHandlerConnectRegistersAndAttachesLiveSession(t *testing.T) {
	ctx := context.Background()
	server := NewMemoryServer()
	_, err := server.CreateDocument(ctx, "doc-dispatch", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	mailbox := NewMemoryMailboxStore()
	sessions := NewSessionManager()
	handler := NewSessionInboundHandler(server, mailbox, sessions, func() string { return "env-dispatch-1" })

	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 8)
	conn.Start(handler)
	defer conn.Wait()
	defer conn.Close()

	if err := clientWS.WriteJSON(ConnectRequest{
		Type:       MessageTypeConnect,
		SessionID:  "sess-dispatch",
		DocumentID: "doc-dispatch",
	}); err != nil {
		t.Fatalf("write connect request failed: %v", err)
	}

	var connected ConnectedMessage
	readClientJSON(t, clientWS, &connected)
	if connected.Type != MessageTypeConnected {
		t.Fatalf("unexpected connected type: %s", connected.Type)
	}

	_, err = server.Submit(ctx, "doc-dispatch", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "client-a")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	var eventMsg EventMessage
	readClientJSON(t, clientWS, &eventMsg)
	if eventMsg.Type != MessageTypeEvent || eventMsg.Envelope.ID != "env-dispatch-1" {
		t.Fatalf("unexpected event message: %+v", eventMsg)
	}

	active, ok := sessions.Active("sess-dispatch")
	if !ok || active != conn {
		t.Fatalf("expected active session to point at conn, got ok=%v active=%p", ok, active)
	}
}

func TestSessionInboundHandlerAckEnvelopeRoutesToMailbox(t *testing.T) {
	ctx := context.Background()
	mailbox := NewMemoryMailboxStore()
	for _, env := range []Envelope{
		testEnvelope("env-1", "sess-dispatch", 1),
		testEnvelope("env-2", "sess-dispatch", 2),
	} {
		if err := mailbox.Append(ctx, env); err != nil {
			t.Fatalf("append envelope failed: %v", err)
		}
	}

	sessions := NewSessionManager()
	handler := NewSessionInboundHandler(NewMemoryServer(), mailbox, sessions, func() string { return "unused" })
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 2)
	conn.Start(handler)
	defer conn.Wait()
	defer conn.Close()

	if err := sessions.Register("sess-dispatch", conn); err != nil {
		t.Fatalf("register session failed: %v", err)
	}

	if err := clientWS.WriteJSON(AckEnvelopeRequest{
		Type:       MessageTypeAckEnvelope,
		SessionID:  "sess-dispatch",
		EnvelopeID: "env-2",
	}); err != nil {
		t.Fatalf("write ack request failed: %v", err)
	}

	waitForAckCursor(t, ctx, mailbox, "sess-dispatch", "env-2")
}

func TestSessionInboundHandlerRejectsInvalidMessage(t *testing.T) {
	handler := NewSessionInboundHandler(NewMemoryServer(), NewMemoryMailboxStore(), NewSessionManager(), func() string { return "unused" })
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 1)
	conn.Start(handler)
	defer conn.Wait()
	defer conn.Close()

	if err := clientWS.WriteMessage(1, []byte(`{"type":"unknown"}`)); err != nil {
		t.Fatalf("write unknown message failed: %v", err)
	}

	<-conn.Context().Done()
	if !errors.Is(conn.Context().Err(), context.Canceled) {
		t.Fatalf("expected canceled connection context, got %v", conn.Context().Err())
	}
}

func TestSessionInboundHandlerDoesNotActivateSessionWhenLiveAttachFails(t *testing.T) {
	mailbox := NewMemoryMailboxStore()
	sessions := NewSessionManager()
	handler := NewSessionInboundHandler(nil, mailbox, sessions, func() string { return "unused" })
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 4)
	conn.Start(handler)
	defer conn.Wait()
	defer conn.Close()

	if err := clientWS.WriteJSON(ConnectRequest{
		Type:       MessageTypeConnect,
		SessionID:  "sess-fail",
		DocumentID: "doc-fail",
	}); err != nil {
		t.Fatalf("write connect request failed: %v", err)
	}

	<-conn.Context().Done()
	if _, ok := sessions.Active("sess-fail"); ok {
		t.Fatal("session should not become active when live attach fails")
	}
}

func TestSessionInboundHandlerRejectsAckFromNonActiveConnection(t *testing.T) {
	ctx := context.Background()
	mailbox := NewMemoryMailboxStore()
	for _, env := range []Envelope{
		testEnvelope("env-1", "sess-owner", 1),
		testEnvelope("env-2", "sess-owner", 2),
	} {
		if err := mailbox.Append(ctx, env); err != nil {
			t.Fatalf("append envelope failed: %v", err)
		}
	}

	sessions := NewSessionManager()
	handler := NewSessionInboundHandler(NewMemoryServer(), mailbox, sessions, func() string { return "unused" })
	activeServerWS, activeClientWS := newWebSocketPair(t)
	defer activeClientWS.Close()
	activeConn := NewClientConn(activeServerWS, 1)
	activeConn.Start(nil)
	defer activeConn.Wait()
	defer activeConn.Close()
	if err := sessions.Register("sess-owner", activeConn); err != nil {
		t.Fatalf("register active session failed: %v", err)
	}

	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 2)
	conn.Start(handler)
	defer conn.Wait()
	defer conn.Close()

	if err := clientWS.WriteJSON(AckEnvelopeRequest{
		Type:       MessageTypeAckEnvelope,
		SessionID:  "sess-owner",
		EnvelopeID: "env-2",
	}); err != nil {
		t.Fatalf("write ack request failed: %v", err)
	}

	<-conn.Context().Done()
	acked, err := mailbox.LastAcked(ctx, "sess-owner")
	if err != nil {
		t.Fatalf("last acked failed: %v", err)
	}
	if acked != "" {
		t.Fatalf("non-active connection should not ack mailbox, got %s", acked)
	}
}

func waitForAckCursor(t *testing.T, ctx context.Context, mailbox MailboxStore, sessionID, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, err := mailbox.LastAcked(ctx, sessionID)
		if err != nil {
			t.Fatalf("last acked failed: %v", err)
		}
		if got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	got, err := mailbox.LastAcked(ctx, sessionID)
	if err != nil {
		t.Fatalf("last acked failed after waiting: %v", err)
	}
	t.Fatalf("unexpected ack cursor after waiting: got %s want %s", got, want)
}
