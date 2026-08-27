package mux

import (
	"encoding/json"
	"testing"

	"github.com/b-nnett/codex-subscription-router/internal/protocol"
)

func TestParseTurnCompletionRecognizesStructuredUsageLimit(t *testing.T) {
	completion, ok := parseTurnCompletion(json.RawMessage(`{
		"threadId":"thread-1",
		"turn":{
			"id":"turn-1",
			"status":"failed",
			"items":[],
			"error":{
				"message":"You've hit your usage limit.",
				"codexErrorInfo":"usageLimitExceeded"
			}
		}
	}`))
	if !ok || !completion.UsageLimited || completion.ThreadID != "thread-1" || completion.TurnID != "turn-1" {
		t.Fatalf("unexpected completion: %#v ok=%v", completion, ok)
	}
}

func TestParseTurnCompletionDoesNotRetrySuccessfulOrUnrelatedFailures(t *testing.T) {
	for name, params := range map[string]string{
		"successful": `{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","error":null}}`,
		"unrelated":  `{"threadId":"thread-1","turn":{"id":"turn-1","status":"failed","error":{"message":"Workspace is unavailable","codexErrorInfo":"other"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			completion, ok := parseTurnCompletion(json.RawMessage(params))
			if !ok || completion.UsageLimited {
				t.Fatalf("unexpected completion: %#v ok=%v", completion, ok)
			}
		})
	}
}

func TestAutomaticContinuationParamsPreservesTurnConfiguration(t *testing.T) {
	original := json.RawMessage(`{
		"threadId":"thread-1",
		"clientUserMessageId":"client-1",
		"input":[{"type":"text","text":"Build the feature"}],
		"model":"gpt-5.6-sol",
		"effort":"high",
		"cwd":"/tmp/project",
		"responsesapiClientMetadata":{"source":"desktop"},
		"additionalContext":{"selection":{"value":"secret","kind":"application"}}
	}`)
	encoded, err := automaticContinuationParams(original)
	if err != nil {
		t.Fatal(err)
	}
	var params map[string]any
	if err := json.Unmarshal(encoded, &params); err != nil {
		t.Fatal(err)
	}
	if params["threadId"] != "thread-1" || params["model"] != "gpt-5.6-sol" ||
		params["effort"] != "high" || params["cwd"] != "/tmp/project" {
		t.Fatalf("turn configuration was not preserved: %#v", params)
	}
	if _, ok := params["clientUserMessageId"]; ok {
		t.Fatal("client message ID must not be reused")
	}
	if _, ok := params["responsesapiClientMetadata"]; ok {
		t.Fatal("original response metadata must not be reused")
	}
	if _, ok := params["additionalContext"]; ok {
		t.Fatal("original selected context must not be duplicated")
	}
	input, ok := params["input"].([]any)
	if !ok || len(input) != 1 || input[0].(map[string]any)["text"] != automaticContinuationText {
		t.Fatalf("unexpected automatic continuation input: %#v", params["input"])
	}
}

func TestActiveTurnIsConsumedExactlyOnce(t *testing.T) {
	multiplexer := &Multiplexer{activeTurns: make(map[string]activeTurnRoute)}
	route := externalRoute{
		accountID: "one",
		method:    "turn/start",
		message: protocol.Message{
			Method: "turn/start",
			Params: json.RawMessage(`{"threadId":"thread-1","input":[]}`),
		},
	}
	multiplexer.rememberActiveTurn(route, "one", "turn-1")
	if _, ok := multiplexer.takeActiveTurn("thread-1", "one", "different-turn"); ok {
		t.Fatal("a mismatched completion consumed the tracked turn")
	}
	if _, ok := multiplexer.takeActiveTurn("thread-1", "one", "turn-1"); !ok {
		t.Fatal("the matching completion did not consume the tracked turn")
	}
	if _, ok := multiplexer.takeActiveTurn("thread-1", "one", "turn-1"); ok {
		t.Fatal("a duplicate completion triggered a second recovery")
	}
}
