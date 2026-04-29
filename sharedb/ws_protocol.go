package sharedb

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrInvalidClientMessage means a websocket client message is malformed.
var ErrInvalidClientMessage = errors.New("sharedb: invalid client message")

const (
	MessageTypeConnect        = "connect"
	MessageTypeConnected      = "connected"
	MessageTypeReplay         = "replay"
	MessageTypeResyncRequired = "resync_required"
	MessageTypeEvent          = "event"
	MessageTypeAckEnvelope    = "ack_envelope"
)

const defaultReplayLimit = 256
const liveSubscribeBuffer = 128

// ConnectRequest is the first transport-level handshake from a client session.
type ConnectRequest struct {
	Type           string `json:"type"`
	SessionID      string `json:"sessionId"`
	DocumentID     string `json:"documentId"`
	LastEnvelopeID string `json:"lastEnvelopeId,omitempty"`
}

// ConnectedMessage acknowledges a successful websocket/session handshake.
type ConnectedMessage struct {
	Type                string `json:"type"`
	SessionID           string `json:"sessionId"`
	DocumentID          string `json:"documentId"`
	LastAckedEnvelopeID string `json:"lastAckedEnvelopeId,omitempty"`
}

// ReplayMessage delivers replayed envelopes after a reconnect.
type ReplayMessage struct {
	Type       string     `json:"type"`
	SessionID  string     `json:"sessionId"`
	DocumentID string     `json:"documentId"`
	Envelopes  []Envelope `json:"envelopes"`
}

// ResyncRequiredMessage tells the client mailbox replay cannot continue from its cursor.
type ResyncRequiredMessage struct {
	Type                string `json:"type"`
	SessionID           string `json:"sessionId"`
	DocumentID          string `json:"documentId"`
	LastAckedEnvelopeID string `json:"lastAckedEnvelopeId,omitempty"`
}

// EventMessage delivers one live session envelope over the websocket transport.
type EventMessage struct {
	Type       string   `json:"type"`
	SessionID  string   `json:"sessionId"`
	DocumentID string   `json:"documentId"`
	Envelope   Envelope `json:"envelope"`
}

// AckEnvelopeRequest tells the server the client processed envelopes through EnvelopeID.
type AckEnvelopeRequest struct {
	Type       string `json:"type"`
	SessionID  string `json:"sessionId"`
	EnvelopeID string `json:"envelopeId"`
}

// HandleConnect registers the websocket connection for the session and sends replay state.
func HandleConnect(ctx context.Context, conn *ClientConn, sessions *SessionManager, mailbox MailboxStore, req ConnectRequest) error {
	if err := sendConnectHandshake(ctx, conn, mailbox, req); err != nil {
		return err
	}
	if sessions == nil {
		return ErrInvalidClientMessage
	}
	return sessions.Register(req.SessionID, conn)
}

// HandleConnectAndAttachLive reduces the replay/live gap by subscribing before replay is computed.
func HandleConnectAndAttachLive(ctx context.Context, conn *ClientConn, server *Server, sessions *SessionManager, mailbox MailboxStore, req ConnectRequest, gen EnvelopeIDGenerator) error {
	if err := validateConnectRequest(req); err != nil {
		return err
	}
	if conn == nil || server == nil || sessions == nil || mailbox == nil || gen == nil || conn.Context().Err() != nil {
		return ErrInvalidClientMessage
	}
	if !conn.claimLiveAttachment(req.SessionID, req.DocumentID) {
		return ErrInvalidClientMessage
	}

	events, cancel, err := server.Subscribe(conn.Context(), req.DocumentID, liveSubscribeBuffer)
	if err != nil {
		return err
	}
	if err := sendConnectHandshake(ctx, conn, mailbox, req); err != nil {
		cancel()
		return err
	}
	maxReplayVersion, err := maxReplayEnvelopeVersion(ctx, mailbox, req.SessionID, req.LastEnvelopeID, defaultReplayLimit)
	if err != nil {
		cancel()
		return err
	}
	if err := sessions.Register(req.SessionID, conn); err != nil {
		cancel()
		return err
	}
	startLiveRelay(conn, mailbox, req.SessionID, req.DocumentID, gen, events, cancel, maxReplayVersion)
	return nil
}

func sendConnectHandshake(ctx context.Context, conn *ClientConn, mailbox MailboxStore, req ConnectRequest) error {
	if err := validateConnectRequest(req); err != nil {
		return err
	}
	if conn == nil || mailbox == nil {
		return ErrInvalidClientMessage
	}

	replay, err := ReplaySessionMailbox(ctx, mailbox, req.SessionID, req.LastEnvelopeID, defaultReplayLimit)
	if err != nil {
		return err
	}
	if err := conn.sendJSON(ConnectedMessage{
		Type:                MessageTypeConnected,
		SessionID:           req.SessionID,
		DocumentID:          req.DocumentID,
		LastAckedEnvelopeID: replay.LastAcked,
	}); err != nil {
		return err
	}
	if replay.RequiresResync {
		return conn.sendJSON(ResyncRequiredMessage{
			Type:                MessageTypeResyncRequired,
			SessionID:           req.SessionID,
			DocumentID:          req.DocumentID,
			LastAckedEnvelopeID: replay.LastAcked,
		})
	}
	if len(replay.Envelopes) == 0 {
		return nil
	}
	return conn.sendJSON(ReplayMessage{
		Type:       MessageTypeReplay,
		SessionID:  req.SessionID,
		DocumentID: req.DocumentID,
		Envelopes:  replay.Envelopes,
	})
}

func validateConnectRequest(req ConnectRequest) error {
	if req.Type != MessageTypeConnect || req.SessionID == "" || req.DocumentID == "" {
		return ErrInvalidClientMessage
	}
	return nil
}

func validateAckEnvelopeRequest(req AckEnvelopeRequest) error {
	if req.Type != MessageTypeAckEnvelope || req.SessionID == "" || req.EnvelopeID == "" {
		return ErrInvalidClientMessage
	}
	return nil
}

// HandleAckEnvelope advances the session mailbox ack cursor through the provided envelope.
func HandleAckEnvelope(ctx context.Context, mailbox MailboxStore, req AckEnvelopeRequest) error {
	if err := validateAckEnvelopeRequest(req); err != nil {
		return err
	}
	if mailbox == nil {
		return ErrInvalidClientMessage
	}
	return mailbox.AckThrough(ctx, req.SessionID, req.EnvelopeID)
}

// AttachLiveSession subscribes the active websocket connection to live document events.
func AttachLiveSession(ctx context.Context, conn *ClientConn, server *Server, mailbox MailboxStore, sessionID, documentID string, gen EnvelopeIDGenerator) error {
	if conn == nil || server == nil || mailbox == nil || gen == nil || sessionID == "" || documentID == "" || conn.Context().Err() != nil {
		return ErrInvalidClientMessage
	}
	if !conn.claimLiveAttachment(sessionID, documentID) {
		return ErrInvalidClientMessage
	}
	events, cancel, err := server.Subscribe(conn.Context(), documentID, liveSubscribeBuffer)
	if err != nil {
		return err
	}
	startLiveRelay(conn, mailbox, sessionID, documentID, gen, events, cancel, 0)
	return nil
}

func maxReplayEnvelopeVersion(ctx context.Context, mailbox MailboxStore, sessionID, afterEnvelopeID string, limit int) (int, error) {
	replay, err := ReplaySessionMailbox(ctx, mailbox, sessionID, afterEnvelopeID, limit)
	if err != nil {
		return 0, err
	}
	maxVersion := 0
	for _, env := range replay.Envelopes {
		if env.Version > maxVersion {
			maxVersion = env.Version
		}
	}
	return maxVersion, nil
}

func startLiveRelay(conn *ClientConn, mailbox MailboxStore, sessionID, documentID string, gen EnvelopeIDGenerator, events <-chan Event, cancel func(), skipThroughVersion int) {
	conn.startTask(func(connCtx context.Context) {
		defer cancel()
		for {
			select {
			case <-connCtx.Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				if event.Version <= skipThroughVersion {
					continue
				}
				env, err := EnvelopeForSession(sessionID, event, gen)
				if err != nil {
					conn.Close()
					return
				}
				if err := mailbox.Append(connCtx, env); err != nil {
					conn.Close()
					return
				}
				if err := conn.sendJSON(EventMessage{
					Type:       MessageTypeEvent,
					SessionID:  sessionID,
					DocumentID: documentID,
					Envelope:   env,
				}); err != nil {
					conn.Close()
					return
				}
			}
		}
	})
}

type inboundMessageEnvelope struct {
	Type string `json:"type"`
}

// NewSessionInboundHandler returns a websocket message dispatcher for session mailbox transport.
func NewSessionInboundHandler(server *Server, mailbox MailboxStore, sessions *SessionManager, gen EnvelopeIDGenerator) InboundHandler {
	return func(ctx context.Context, conn *ClientConn, payload []byte) error {
		var env inboundMessageEnvelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return ErrInvalidClientMessage
		}
		switch env.Type {
		case MessageTypeConnect:
			var req ConnectRequest
			if err := json.Unmarshal(payload, &req); err != nil {
				return ErrInvalidClientMessage
			}
			return HandleConnectAndAttachLive(ctx, conn, server, sessions, mailbox, req, gen)
		case MessageTypeAckEnvelope:
			var req AckEnvelopeRequest
			if err := json.Unmarshal(payload, &req); err != nil {
				return ErrInvalidClientMessage
			}
			if sessions == nil {
				return ErrInvalidClientMessage
			}
			active, ok := sessions.Active(req.SessionID)
			if !ok || active != conn {
				return ErrInvalidClientMessage
			}
			return HandleAckEnvelope(ctx, mailbox, req)
		default:
			return ErrInvalidClientMessage
		}
	}
}

func (c *ClientConn) sendJSON(msg any) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.Enqueue(payload)
}
