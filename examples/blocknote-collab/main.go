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
	Type     string          `json:"type"`
	Version  int             `json:"version"`
	Document json.RawMessage `json:"document,omitempty"`
}

type serverMessage struct {
	Type     string          `json:"type"`
	ClientID string          `json:"clientId,omitempty"`
	Version  int             `json:"version"`
	Document json.RawMessage `json:"document,omitempty"`
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
	mu        sync.Mutex
	ot        *jsonot.JSONOperationTransformer
	document  jsonot.Value
	history   []*jsonot.Operation
	snapshots []jsonot.Value
	clients   map[*client]struct{}
}

func newCollabServer() *collabServer {
	initialDoc := jsonot.ValueFromAny(map[string]any{
		"blocks": []any{
			map[string]any{
				"id":   "welcome-block",
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type":   "text",
						"text":   "欢迎来到 jsonot + BlockNote 协同编辑示例",
						"styles": map[string]any{},
					},
				},
			},
		},
	})

	s := &collabServer{
		ot:      jsonot.NewJSONOperationTransformer(),
		document: initialDoc,
		clients: make(map[*client]struct{}),
	}

	clone, err := cloneValue(initialDoc)
	if err != nil {
		panic(fmt.Errorf("clone initial document failed: %w", err))
	}
	s.snapshots = []jsonot.Value{clone}
	return s
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

func (s *collabServer) snapshot() (json.RawMessage, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw := append(json.RawMessage(nil), s.document.RawMessage()...)
	return raw, len(s.history), nil
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

	docRaw, version, err := s.snapshot()
	if err != nil {
		log.Printf("snapshot failed: %v", err)
		return
	}

	if err := c.writeJSON(serverMessage{Type: "init", ClientID: c.id, Version: version, Document: docRaw}); err != nil {
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

		if msg.Type != "sync" {
			_ = c.writeJSON(serverMessage{Type: "error", Version: version, Message: "unsupported message type"})
			continue
		}

		update, ack, err := s.applyClientSync(c, msg.Version, msg.Document)
		if err != nil {
			_ = c.writeJSON(serverMessage{Type: "error", Version: version, Message: err.Error()})
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

func (s *collabServer) applyClientSync(sender *client, baseVersion int, rawDocument json.RawMessage) (serverMessage, serverMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if baseVersion < 0 || baseVersion >= len(s.snapshots) {
		return serverMessage{}, serverMessage{}, fmt.Errorf("invalid base version: expected 0-%d, got %d", len(s.snapshots)-1, baseVersion)
	}

	clientDoc, err := jsonot.UnmarshalValue(rawDocument)
	if err != nil {
		return serverMessage{}, serverMessage{}, fmt.Errorf("parse document failed: %w", err)
	}
	if err := validateClientDocument(clientDoc); err != nil {
		return serverMessage{}, serverMessage{}, err
	}

	baseDoc, err := cloneValue(s.snapshots[baseVersion])
	if err != nil {
		return serverMessage{}, serverMessage{}, fmt.Errorf("load base snapshot failed: %w", err)
	}

	diffResult := s.ot.Diff(context.Background(), baseDoc, clientDoc)
	if diffResult.IsError() {
		return serverMessage{}, serverMessage{}, fmt.Errorf("diff document failed: %w", diffResult.Error())
	}

	transformed := diffResult.MustGet()
	for _, concurrent := range s.history[baseVersion:] {
		transformed, _, err = s.ot.Transform(context.Background(), transformed, concurrent)
		if err != nil {
			return serverMessage{}, serverMessage{}, fmt.Errorf("transform failed: %w", err)
		}
	}

	if transformed.IsEmpty() {
		raw := append(json.RawMessage(nil), s.document.RawMessage()...)
		ack := serverMessage{Type: "ack", Version: len(s.history), Document: raw}
		return serverMessage{}, ack, nil
	}

	current, err := cloneValue(s.document)
	if err != nil {
		return serverMessage{}, serverMessage{}, fmt.Errorf("clone current document failed: %w", err)
	}
	applyResult := s.ot.Apply(context.Background(), current, transformed)
	if applyResult.IsError() {
		return serverMessage{}, serverMessage{}, fmt.Errorf("apply operation failed: %w", applyResult.Error())
	}

	nextDoc := applyResult.MustGet()
	if err := validateClientDocument(nextDoc); err != nil {
		return serverMessage{}, serverMessage{}, fmt.Errorf("invalid document after apply: %w", err)
	}

	s.document = nextDoc
	s.history = append(s.history, transformed)
	clonedSnapshot, err := cloneValue(nextDoc)
	if err != nil {
		return serverMessage{}, serverMessage{}, fmt.Errorf("clone next snapshot failed: %w", err)
	}
	s.snapshots = append(s.snapshots, clonedSnapshot)

	version := len(s.history)
	raw := append(json.RawMessage(nil), s.document.RawMessage()...)

	update := serverMessage{Type: "update", ClientID: sender.id, Version: version, Document: raw}
	ack := serverMessage{Type: "ack", Version: version, Document: raw}
	return update, ack, nil
}

func validateClientDocument(document jsonot.Value) error {
	if !document.IsObject() {
		return errors.New("document must be an object")
	}

	blocks := document.GetKey("blocks")
	if blocks.IsAbsent() {
		return errors.New("document.blocks is required")
	}

	arr := blocks.MustGet().GetArray()
	if arr.IsError() {
		return errors.New("document.blocks must be an array")
	}

	if len(arr.MustGet()) == 0 {
		return errors.New("document.blocks must not be empty")
	}
	return nil
}

func cloneValue(value jsonot.Value) (jsonot.Value, error) {
	return jsonot.UnmarshalValue(value.RawMessage())
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

func newClientID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		log.Printf("generate client ID failed: %v", err)
		return "anonymous"
	}
	return hex.EncodeToString(buf[:])
}

func main() {
	addr := flag.String("addr", ":8080", "http service address")
	flag.Parse()

	staticFS, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatalf("load static assets failed: %v", err)
	}

	server := newCollabServer()
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServerFS(staticFS))
	mux.HandleFunc("/ws", server.handleWS)

	log.Printf("blocknote collab demo listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
