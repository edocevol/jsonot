package sharedb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestClientConnWriteLoopDeliversQueuedMessages(t *testing.T) {
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()

	conn := NewClientConn(serverWS, 1)
	conn.Start(nil)

	if err := conn.Enqueue([]byte(`{"type":"hello"}`)); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	_, payload, err := clientWS.ReadMessage()
	if err != nil {
		t.Fatalf("read client message failed: %v", err)
	}
	if string(payload) != `{"type":"hello"}` {
		t.Fatalf("unexpected payload: %s", payload)
	}

	conn.Close()
	conn.Wait()
}

func TestClientConnReadLoopPassesMessagesToHandler(t *testing.T) {
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()

	got := make(chan []byte, 1)
	conn := NewClientConn(serverWS, 1)
	conn.Start(func(_ context.Context, _ *ClientConn, payload []byte) error {
		got <- append([]byte(nil), payload...)
		return nil
	})

	if err := clientWS.WriteMessage(websocket.TextMessage, []byte(`{"type":"connect"}`)); err != nil {
		t.Fatalf("write client message failed: %v", err)
	}

	select {
	case payload := <-got:
		if string(payload) != `{"type":"connect"}` {
			t.Fatalf("unexpected handler payload: %s", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler payload")
	}

	conn.Close()
	conn.Wait()
}

func TestClientConnEnqueueAfterCloseReturnsCanceled(t *testing.T) {
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()

	conn := NewClientConn(serverWS, 1)
	conn.Close()

	if err := conn.Enqueue([]byte(`{"type":"late"}`)); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestClientConnAddCloseHookAfterCloseRunsImmediately(t *testing.T) {
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()

	conn := NewClientConn(serverWS, 1)
	conn.Close()

	ran := make(chan struct{}, 1)
	conn.AddCloseHook(func() {
		ran <- struct{}{}
	})

	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for close hook")
	}
}

func TestSessionManagerRegisterReplacesOldConnectionWithoutDroppingNewOne(t *testing.T) {
	oldServerWS, oldClientWS := newWebSocketPair(t)
	defer oldClientWS.Close()
	newServerWS, newClientWS := newWebSocketPair(t)
	defer newClientWS.Close()

	manager := NewSessionManager()
	oldConn := NewClientConn(oldServerWS, 1)
	newConn := NewClientConn(newServerWS, 1)

	if err := manager.Register("sess-1", oldConn); err != nil {
		t.Fatalf("register old conn failed: %v", err)
	}
	if active, ok := manager.Active("sess-1"); !ok || active != oldConn {
		t.Fatalf("expected old conn to be active, got ok=%v active=%p", ok, active)
	}

	if err := manager.Register("sess-1", newConn); err != nil {
		t.Fatalf("register new conn failed: %v", err)
	}

	select {
	case <-oldConn.Context().Done():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for old conn to close")
	}
	oldConn.Wait()

	if active, ok := manager.Active("sess-1"); !ok || active != newConn {
		t.Fatalf("expected new conn to remain active, got ok=%v active=%p", ok, active)
	}

	newConn.Close()
	newConn.Wait()
	if _, ok := manager.Active("sess-1"); ok {
		t.Fatal("expected session to unregister when active conn closes")
	}
}

func TestSessionManagerRegisterRejectsClosedConnection(t *testing.T) {
	serverWS, clientWS := newWebSocketPair(t)
	defer clientWS.Close()

	manager := NewSessionManager()
	conn := NewClientConn(serverWS, 1)
	conn.Close()

	if err := manager.Register("sess-1", conn); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expected invalid session error, got %v", err)
	}
	if _, ok := manager.Active("sess-1"); ok {
		t.Fatal("closed connection should not become active")
	}
}

func newWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()

	serverConnCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		serverConnCh <- conn
	}))
	t.Cleanup(server.Close)

	dialer := websocket.Dialer{}
	clientConn, _, err := dialer.Dial("ws"+server.URL[len("http"):], nil)
	if err != nil {
		t.Fatalf("dial websocket failed: %v", err)
	}
	serverConn := <-serverConnCh
	t.Cleanup(func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
	})

	return serverConn, clientConn
}
