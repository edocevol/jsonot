package sharedb

import (
	"context"
	"errors"
	"sync"

	"github.com/gorilla/websocket"
)

// ErrInvalidSession means a session manager call received empty or malformed input.
var ErrInvalidSession = errors.New("sharedb: invalid session")

// InboundHandler processes one inbound websocket message for a client connection.
type InboundHandler func(context.Context, *ClientConn, []byte) error

// ClientConn owns one websocket connection and the goroutines attached to it.
type ClientConn struct {
	conn *websocket.Conn
	send chan []byte

	ctx    context.Context
	cancel context.CancelFunc

	closeOnce sync.Once
	startOnce sync.Once
	wg        sync.WaitGroup

	hooksMu    sync.Mutex
	closed     bool
	closeHooks []func()

	liveMu   sync.Mutex
	liveKeys map[string]struct{}
}

// NewClientConn creates a managed websocket connection with a bounded send queue.
func NewClientConn(conn *websocket.Conn, sendBuffer int) *ClientConn {
	if sendBuffer <= 0 {
		sendBuffer = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ClientConn{
		conn:     conn,
		send:     make(chan []byte, sendBuffer),
		ctx:      ctx,
		cancel:   cancel,
		liveKeys: make(map[string]struct{}),
	}
}

// Context returns the lifecycle context for this connection.
func (c *ClientConn) Context() context.Context {
	return c.ctx
}

// Start launches the read and write goroutines exactly once.
func (c *ClientConn) Start(handler InboundHandler) {
	c.startOnce.Do(func() {
		c.wg.Add(2)
		go func() {
			defer c.wg.Done()
			c.readLoop(handler)
		}()
		go func() {
			defer c.wg.Done()
			c.writeLoop()
		}()
	})
}

// Enqueue queues one outbound websocket payload.
func (c *ClientConn) Enqueue(data []byte) error {
	select {
	case <-c.ctx.Done():
		return context.Canceled
	default:
	}

	payload := append([]byte(nil), data...)
	select {
	case <-c.ctx.Done():
		return context.Canceled
	case c.send <- payload:
		return nil
	}
}

// AddCloseHook registers a function that runs exactly once when the connection closes.
func (c *ClientConn) AddCloseHook(fn func()) {
	if fn == nil {
		return
	}
	c.hooksMu.Lock()
	if c.closed {
		c.hooksMu.Unlock()
		fn()
		return
	}
	c.closeHooks = append(c.closeHooks, fn)
	c.hooksMu.Unlock()
}

// Close cancels the connection lifecycle, closes the websocket, and runs close hooks.
func (c *ClientConn) Close() {
	c.closeOnce.Do(func() {
		c.cancel()
		if c.conn != nil {
			_ = c.conn.Close()
		}

		c.hooksMu.Lock()
		c.closed = true
		hooks := append([]func(){}, c.closeHooks...)
		c.closeHooks = nil
		c.hooksMu.Unlock()
		for _, hook := range hooks {
			hook()
		}
	})
}

// Wait blocks until all connection-bound goroutines exit.
func (c *ClientConn) Wait() {
	c.wg.Wait()
}

func (c *ClientConn) startTask(fn func(context.Context)) {
	if fn == nil || c.ctx.Err() != nil {
		return
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		fn(c.ctx)
	}()
}

func (c *ClientConn) claimLiveAttachment(sessionID, documentID string) bool {
	key := sessionID + "\x00" + documentID
	c.liveMu.Lock()
	defer c.liveMu.Unlock()
	if _, exists := c.liveKeys[key]; exists {
		return false
	}
	c.liveKeys[key] = struct{}{}
	return true
}

func (c *ClientConn) readLoop(handler InboundHandler) {
	for {
		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			c.Close()
			return
		}
		if handler == nil {
			continue
		}
		if err := handler(c.ctx, c, payload); err != nil {
			c.Close()
			return
		}
	}
}

func (c *ClientConn) writeLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case payload := <-c.send:
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				c.Close()
				return
			}
		}
	}
}

// SessionManager tracks the active websocket connection for each stable session ID.
type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]*ClientConn
}

// NewSessionManager creates an empty session manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{sessions: make(map[string]*ClientConn)}
}

// Register installs conn as the active connection for sessionID, replacing and closing any prior one.
func (m *SessionManager) Register(sessionID string, conn *ClientConn) error {
	if sessionID == "" || conn == nil || conn.Context().Err() != nil {
		return ErrInvalidSession
	}

	conn.AddCloseHook(func() {
		m.Unregister(sessionID, conn)
	})

	var old *ClientConn
	m.mu.Lock()
	old = m.sessions[sessionID]
	m.sessions[sessionID] = conn
	m.mu.Unlock()

	if old != nil && old != conn {
		old.Close()
	}
	return nil
}

// Active returns the active connection for sessionID, if any.
func (m *SessionManager) Active(sessionID string) (*ClientConn, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	conn, ok := m.sessions[sessionID]
	return conn, ok
}

// Unregister removes conn only if it is still the active connection for sessionID.
func (m *SessionManager) Unregister(sessionID string, conn *ClientConn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.sessions[sessionID]; ok && current == conn {
		delete(m.sessions, sessionID)
	}
}
