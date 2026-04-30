package sharedb

import (
	"context"
	"errors"
)

// ReplayResult is a transport-friendly mailbox replay decision.
type ReplayResult struct {
	SessionID           string      `json:"sessionId"`
	Envelopes           []Envelope  `json:"envelopes"`
	LastAcked           string      `json:"lastAcked,omitempty"`
	RequiresResync      bool        `json:"requiresResync,omitempty"`
	Reason              StaleReason `json:"reason,omitempty"`
	CurrentVersion      int         `json:"currentVersion,omitempty"`
	MinSupportedVersion int         `json:"minSupportedVersion,omitempty"`
	MaxRebaseGap        int         `json:"maxRebaseGap,omitempty"`
}

// ReplaySessionMailbox replays envelopes for a session after the provided cursor.
// Unknown non-empty cursors are translated into a resync decision instead of an error.
func ReplaySessionMailbox(ctx context.Context, store MailboxStore, sessionID, afterEnvelopeID string, limit int) (ReplayResult, error) {
	lastAcked, err := store.LastAcked(ctx, sessionID)
	if err != nil {
		return ReplayResult{}, err
	}
	envelopes, err := store.ReplayAfter(ctx, sessionID, afterEnvelopeID, limit)
	if err != nil {
		if afterEnvelopeID != "" && errors.Is(err, ErrEnvelopeNotFound) {
			return ReplayResult{
				SessionID:      sessionID,
				LastAcked:      lastAcked,
				RequiresResync: true,
				Reason:         StaleReasonReplayCursorNotFound,
			}, nil
		}
		return ReplayResult{}, err
	}
	return ReplayResult{
		SessionID: sessionID,
		Envelopes: envelopes,
		LastAcked: lastAcked,
	}, nil
}
