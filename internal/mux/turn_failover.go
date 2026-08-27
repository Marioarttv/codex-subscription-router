package mux

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/b-nnett/codex-subscription-router/internal/protocol"
)

const automaticContinuationText = "Continue."

type activeTurnRoute struct {
	route     externalRoute
	accountID string
	turnID    string
}

type turnCompletion struct {
	ThreadID     string
	TurnID       string
	UsageLimited bool
}

func (m *Multiplexer) rememberActiveTurn(route externalRoute, accountID, turnID string) {
	threadID := threadIDFromParams(route.message.Params)
	if threadID == "" {
		return
	}
	m.activeTurnsMu.Lock()
	m.activeTurns[threadID] = activeTurnRoute{
		route:     route,
		accountID: accountID,
		turnID:    turnID,
	}
	m.activeTurnsMu.Unlock()
}

func (m *Multiplexer) bindActiveTurnID(threadID, accountID, turnID string) {
	if threadID == "" || turnID == "" {
		return
	}
	m.activeTurnsMu.Lock()
	active, ok := m.activeTurns[threadID]
	if ok && active.accountID == accountID {
		active.turnID = turnID
		m.activeTurns[threadID] = active
	}
	m.activeTurnsMu.Unlock()
}

func (m *Multiplexer) takeActiveTurn(threadID, accountID, turnID string) (activeTurnRoute, bool) {
	if threadID == "" {
		return activeTurnRoute{}, false
	}
	m.activeTurnsMu.Lock()
	defer m.activeTurnsMu.Unlock()
	active, ok := m.activeTurns[threadID]
	if !ok || active.accountID != accountID ||
		(turnID != "" && active.turnID != "" && active.turnID != turnID) {
		return activeTurnRoute{}, false
	}
	delete(m.activeTurns, threadID)
	return active, true
}

func parseTurnCompletion(params json.RawMessage) (turnCompletion, bool) {
	var decoded struct {
		ThreadID string `json:"threadId"`
		Turn     struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  *struct {
				Message        string `json:"message"`
				CodexErrorInfo any    `json:"codexErrorInfo"`
			} `json:"error"`
		} `json:"turn"`
	}
	if json.Unmarshal(params, &decoded) != nil || decoded.ThreadID == "" || decoded.Turn.ID == "" {
		return turnCompletion{}, false
	}
	completion := turnCompletion{ThreadID: decoded.ThreadID, TurnID: decoded.Turn.ID}
	if decoded.Turn.Status == "failed" && decoded.Turn.Error != nil {
		completion.UsageLimited = isUsageLimitTurnError(
			decoded.Turn.Error.CodexErrorInfo,
			decoded.Turn.Error.Message,
		)
	}
	return completion, true
}

func isUsageLimitTurnError(info any, message string) bool {
	if value, ok := info.(string); ok {
		normalized := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(value))
		if normalized == "usagelimitexceeded" {
			return true
		}
	}
	text := strings.ToLower(message)
	return strings.Contains(text, "usage limit") || strings.Contains(text, "rate limit")
}

func turnIDFromResult(result json.RawMessage) string {
	var decoded struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if json.Unmarshal(result, &decoded) != nil {
		return ""
	}
	return decoded.Turn.ID
}

func automaticContinuationParams(original json.RawMessage) (json.RawMessage, error) {
	var params map[string]any
	if err := json.Unmarshal(original, &params); err != nil {
		return nil, fmt.Errorf("decode original turn: %w", err)
	}
	if _, ok := params["threadId"].(string); !ok {
		return nil, fmt.Errorf("original turn has no thread ID")
	}
	params["input"] = []map[string]any{{"type": "text", "text": automaticContinuationText}}
	delete(params, "clientUserMessageId")
	delete(params, "responsesapiClientMetadata")
	delete(params, "additionalContext")
	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("encode automatic continuation: %w", err)
	}
	return encoded, nil
}

func (m *Multiplexer) continueTurnAfterUsageLimit(active activeTurnRoute, exhaustedAccountID string) {
	threadID := threadIDFromParams(active.route.message.Params)
	if threadID == "" {
		return
	}
	excluded := cloneAccountSet(active.route.excluded)
	if excluded == nil {
		excluded = make(map[string]struct{})
	}
	excluded[exhaustedAccountID] = struct{}{}

	ctx, cancel := context.WithTimeout(context.Background(), 3*requestTimeout)
	defer cancel()
	fallback, _, err := m.chooseAccountExcluding(ctx, excluded)
	if err != nil {
		m.publish(Event{
			Type:      "thread-failover-unavailable",
			AccountID: exhaustedAccountID,
			Message:   "No other subscription currently has five-hour and weekly capacity",
			Data:      map[string]any{"threadId": threadID},
		})
		return
	}
	if err := m.handoffThread(ctx, threadID, exhaustedAccountID, fallback.ID); err != nil {
		m.publish(Event{
			Type:      "thread-failover-failed",
			AccountID: exhaustedAccountID,
			Message:   fmt.Sprintf("Could not move the chat to %s: %v", fallback.Label, err),
			Data:      map[string]any{"threadId": threadID},
		})
		return
	}
	params, err := automaticContinuationParams(active.route.message.Params)
	if err != nil {
		m.publish(Event{
			Type:      "thread-failover-failed",
			AccountID: fallback.ID,
			Message:   fmt.Sprintf("Chat moved to %s, but automatic Continue failed: %v", fallback.Label, err),
			Data:      map[string]any{"threadId": threadID},
		})
		return
	}
	child, ok := m.child(fallback.ID)
	if !ok {
		return
	}
	continuation := protocol.Message{Method: "turn/start", Params: params}
	route := externalRoute{
		accountID: fallback.ID,
		method:    "turn/start",
		message:   continuation,
		excluded:  cloneAccountSet(excluded),
	}
	m.rememberActiveTurn(route, fallback.ID, "")
	response, err := child.Request(ctx, "turn/start", params)
	if err != nil {
		m.takeActiveTurn(threadID, fallback.ID, "")
		m.publish(Event{
			Type:      "thread-failover-failed",
			AccountID: fallback.ID,
			Message:   fmt.Sprintf("Chat moved to %s, but automatic Continue failed: %v", fallback.Label, err),
			Data:      map[string]any{"threadId": threadID},
		})
		return
	}
	m.bindActiveTurnID(threadID, fallback.ID, turnIDFromResult(response.Result))
	m.publish(Event{
		Type:      "thread-failed-over",
		AccountID: fallback.ID,
		Message:   fmt.Sprintf("Moved to %s and sent Continue automatically", fallback.Label),
		Data: map[string]any{
			"threadId":          threadID,
			"previousAccountId": exhaustedAccountID,
			"automaticContinue": true,
		},
	})
}
