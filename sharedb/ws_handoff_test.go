package sharedb

import (
	"context"
	"encoding/json"
	"testing"
)

func TestHandleConnectAndAttachLiveCapturesEventCommittedDuringReplayWindow(t *testing.T) {
	ctx := context.Background()
	server := NewMemoryServer()
	_, err := server.CreateDocument(ctx, "doc-handoff", json.RawMessage(`{"counter":0}`))
	if err != nil {
		t.Fatalf("create document failed: %v", err)
	}

	mailbox := &blockingReplayMailboxStore{
		MailboxStore:  NewMemoryMailboxStore(),
		replayStarted: make(chan struct{}, 1),
		releaseReplay: make(chan struct{}),
	}
	sessions := NewSessionManager()
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()
	conn := NewClientConn(serverWS, 8)
	conn.Start(nil)
	defer conn.Wait()
	defer conn.Close()

	done := make(chan error, 1)
	go func() {
		done <- HandleConnectAndAttachLive(ctx, conn, server, sessions, mailbox, ConnectRequest{
			Type:       MessageTypeConnect,
			SessionID:  "sess-handoff",
			DocumentID: "doc-handoff",
		}, func() string { return "env-handoff-1" })
	}()

	<-mailbox.replayStarted
	_, err = server.Submit(ctx, "doc-handoff", 0, json.RawMessage(`[{"p":["counter"],"na":1}]`), "client-a")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	close(mailbox.releaseReplay)

	if err := <-done; err != nil {
		t.Fatalf("handle connect and attach live failed: %v", err)
	}

	var connected ConnectedMessage
	readClientJSON(t, clientWS, &connected)
	if connected.Type != MessageTypeConnected {
		t.Fatalf("unexpected connected type: %s", connected.Type)
	}

	var eventMsg EventMessage
	readClientJSON(t, clientWS, &eventMsg)
	if eventMsg.Type != MessageTypeEvent || eventMsg.Envelope.ID != "env-handoff-1" || eventMsg.Envelope.Version != 1 {
		t.Fatalf("unexpected handoff event: %+v", eventMsg)
	}

	replayed, err := mailbox.ReplayAfter(ctx, "sess-handoff", "", 0)
	if err != nil {
		t.Fatalf("mailbox replay failed: %v", err)
	}
	if len(replayed) != 1 || replayed[0].ID != "env-handoff-1" {
		t.Fatalf("unexpected mailbox contents after handoff: %+v", replayed)
	}
}

type blockingReplayMailboxStore struct {
	MailboxStore
	replayStarted chan struct{}
	releaseReplay chan struct{}
}

func (s *blockingReplayMailboxStore) ReplayAfter(ctx context.Context, sessionID, afterEnvelopeID string, limit int) ([]Envelope, error) {
	select {
	case s.replayStarted <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.releaseReplay:
	}
	return s.MailboxStore.ReplayAfter(ctx, sessionID, afterEnvelopeID, limit)
}
