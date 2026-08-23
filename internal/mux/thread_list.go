package mux

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/b-nnett/codex-subscription-router/internal/protocol"
	"github.com/b-nnett/codex-subscription-router/internal/state"
)

type accountThreadList struct {
	accountID string
	threads   []map[string]any
}

func (m *Multiplexer) aggregateThreadList(request protocol.Message) {
	entries := m.childEntries()
	results := make(chan accountThreadList, len(entries))
	var wait sync.WaitGroup
	for _, entry := range entries {
		wait.Add(1)
		go func(entry childEntry) {
			defer wait.Done()
			results <- accountThreadList{accountID: entry.account.ID, threads: m.listAllThreads(entry, request.Params)}
		}(entry)
	}
	wait.Wait()
	close(results)

	threadLists := make([]accountThreadList, 0, len(entries))
	for accountResult := range results {
		threadLists = append(threadLists, accountResult)
	}
	threads := mergeThreadLists(m.store, threadLists)
	sortThreads(threads)
	encoded, err := json.Marshal(map[string]any{"data": threads, "nextCursor": nil})
	if err != nil {
		m.write(protocol.Failure(request.ID, -32603, "failed to merge thread list"))
		return
	}
	m.write(protocol.Success(request.ID, encoded))
}

// mergeThreadLists removes duplicate copies created by cross-account handoff.
// Existing explicit ownership wins even when only the source account currently
// advertises the thread, so a sidebar refresh cannot undo a completed move.
func mergeThreadLists(store *state.Store, lists []accountThreadList) []map[string]any {
	type candidate struct {
		accountID string
		thread    map[string]any
	}
	byID := make(map[string]candidate)
	withoutID := make([]map[string]any, 0)
	for _, list := range lists {
		for _, thread := range list.threads {
			threadID, ok := thread["id"].(string)
			if !ok || threadID == "" {
				withoutID = append(withoutID, thread)
				continue
			}
			existing, exists := byID[threadID]
			ownerID, assigned := store.ThreadOwner(threadID)
			if !exists || (assigned && list.accountID == ownerID && existing.accountID != ownerID) {
				byID[threadID] = candidate{accountID: list.accountID, thread: thread}
			}
		}
	}
	threads := make([]map[string]any, 0, len(byID)+len(withoutID))
	for threadID, selected := range byID {
		if _, assigned := store.ThreadOwner(threadID); !assigned {
			_ = store.SetThreadOwner(threadID, selected.accountID)
		}
		threads = append(threads, selected.thread)
	}
	return append(threads, withoutID...)
}

func (m *Multiplexer) listAllThreads(entry childEntry, originalParams json.RawMessage) []map[string]any {
	var params map[string]any
	if json.Unmarshal(originalParams, &params) != nil {
		params = make(map[string]any)
	}
	params["limit"] = 500
	threads := make([]map[string]any, 0)
	seenCursors := make(map[string]struct{})
	var cursor string
	for {
		if cursor == "" {
			params["cursor"] = nil
		} else {
			params["cursor"] = cursor
		}
		encodedParams, _ := json.Marshal(params)
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		response, err := entry.child.Request(ctx, "thread/list", encodedParams)
		cancel()
		if err != nil {
			return threads
		}
		var decoded struct {
			Data       []map[string]any `json:"data"`
			NextCursor *string          `json:"nextCursor"`
		}
		if json.Unmarshal(response.Result, &decoded) != nil {
			return threads
		}
		threads = append(threads, decoded.Data...)
		if decoded.NextCursor == nil || *decoded.NextCursor == "" {
			return threads
		}
		cursor = *decoded.NextCursor
		if _, repeated := seenCursors[cursor]; repeated {
			return threads
		}
		seenCursors[cursor] = struct{}{}
	}
}
