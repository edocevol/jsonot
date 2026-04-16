package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/edocevol/jsonot"
	"github.com/gorilla/websocket"
)

//go:embed web/*
var webFiles embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return sameOrigin(r)
	},
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}

	return parsed.Host == r.Host
}

type clientMessage struct {
	Type    string          `json:"type"`
	Version int             `json:"version"`
	Op      json.RawMessage `json:"op,omitempty"`
}

type serverMessage struct {
	Type     string          `json:"type"`
	ClientID string          `json:"clientId,omitempty"`
	Version  int             `json:"version"`
	Document string          `json:"document,omitempty"`
	Op       json.RawMessage `json:"op,omitempty"`
	Message  string          `json:"message,omitempty"`
}

type client struct {
	id   string
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *client) writeJSON(msg serverMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(msg)
}

type collabServer struct {
	mu       sync.Mutex
	ot       *jsonot.JSONOperationTransformer
	document string
	history  []*jsonot.Operation
	clients  map[*client]struct{}
}

func newCollabServer() *collabServer {
	return &collabServer{
		ot:      jsonot.NewJSONOperationTransformer(),
		clients: make(map[*client]struct{}),
	}
}

func (s *collabServer) addClient(c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[c] = struct{}{}
}

func (s *collabServer) removeClient(c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, c)
}

func (s *collabServer) snapshot() (string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.document, len(s.history)
}

func (s *collabServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade websocket failed: %v", err)
		return
	}
	defer conn.Close()

	c := &client{id: newClientID(), conn: conn}
	s.addClient(c)
	defer s.removeClient(c)

	document, version := s.snapshot()
	if err := c.writeJSON(serverMessage{Type: "init", ClientID: c.id, Version: version, Document: document}); err != nil {
		log.Printf("send init failed: %v", err)
		return
	}

	for {
		var msg clientMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) && !errors.Is(err, websocket.ErrCloseSent) {
				log.Printf("read websocket message failed: %v", err)
			}
			return
		}

		if msg.Type != "op" {
			_ = c.writeJSON(serverMessage{Type: "error", Message: "unsupported message type"})
			continue
		}

		update, ack, err := s.applyClientOperation(c, msg.Version, msg.Op)
		if err != nil {
			_ = c.writeJSON(serverMessage{Type: "error", Message: err.Error()})
			continue
		}

		if err := c.writeJSON(ack); err != nil {
			log.Printf("send ack failed: %v", err)
			return
		}

		if update.Type != "" {
			s.broadcastExcept(c, update)
		}
	}
}

func (s *collabServer) applyClientOperation(sender *client, baseVersion int, rawOp json.RawMessage) (serverMessage, serverMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if baseVersion < 0 || baseVersion > len(s.history) {
		return serverMessage{}, serverMessage{}, fmt.Errorf(
			"client version is invalid: expected 0-%d, got %d",
			len(s.history),
			baseVersion,
		)
	}

	op, err := s.parseOperation(rawOp)
	if err != nil {
		return serverMessage{}, serverMessage{}, err
	}

	transformed := op
	for _, concurrent := range s.history[baseVersion:] {
		transformed, _, err = s.ot.Transform(context.Background(), transformed, concurrent)
		if err != nil {
			return serverMessage{}, serverMessage{}, err
		}
	}

	if transformed.IsEmpty() {
		ack := serverMessage{Type: "ack", Version: len(s.history), Document: s.document}
		return serverMessage{}, ack, nil
	}

	nextDocument, err := s.applyOperation(transformed)
	if err != nil {
		return serverMessage{}, serverMessage{}, err
	}

	s.document = nextDocument
	s.history = append(s.history, transformed)
	version := len(s.history)
	serialized := append(json.RawMessage(nil), transformed.ToValue().RawMessage()...)

	update := serverMessage{
		Type:     "update",
		ClientID: sender.id,
		Version:  version,
		Document: s.document,
		Op:       serialized,
	}
	ack := serverMessage{Type: "ack", Version: version, Document: s.document}
	return update, ack, nil
}

func (s *collabServer) broadcastExcept(sender *client, msg serverMessage) {
	s.mu.Lock()
	clients := make([]*client, 0, len(s.clients))
	for c := range s.clients {
		if c != sender {
			clients = append(clients, c)
		}
	}
	s.mu.Unlock()

	for _, c := range clients {
		if err := c.writeJSON(msg); err != nil {
			log.Printf("broadcast update failed: %v", err)
		}
	}
}

func (s *collabServer) parseOperation(rawOp json.RawMessage) (*jsonot.Operation, error) {
	node, err := jsonot.UnmarshalValue(rawOp)
	if err != nil {
		return nil, err
	}

	components := s.ot.OperationComponentsFromValue(node)
	if components.IsError() {
		return nil, components.Error()
	}

	operation := jsonot.NewOperation(components.MustGet())
	if err := operation.Validation(); err != nil {
		return nil, err
	}
	return operation, nil
}

func (s *collabServer) applyOperation(op *jsonot.Operation) (string, error) {
	doc := jsonot.ValueFromAny(map[string]any{"content": s.document})
	result := s.ot.Apply(context.Background(), doc, op)
	if result.IsError() {
		return "", result.Error()
	}

	content := result.MustGet().GetStringKey("content")
	if content.IsAbsent() {
		return "", errors.New("document content is missing")
	}
	return content.MustGet(), nil
}

func newClientID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		log.Printf("failed to generate client ID: %v", err)
		return "anonymous"
	}
	return hex.EncodeToString(buf[:])
}

func main() {
	addr := flag.String("addr", ":8080", "http service address")
	flag.Parse()

	staticFS, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatalf("load embedded web assets failed: %v", err)
	}

	server := newCollabServer()
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServerFS(staticFS))
	mux.HandleFunc("/ws", server.handleWS)

	log.Printf("websocket example listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
