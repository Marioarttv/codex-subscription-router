package mux

import (
	"path/filepath"
	"testing"

	"github.com/b-nnett/codex-subscription-router/internal/state"
)

func TestMergeThreadListsPreservesHandoffOwnerAndDeduplicates(t *testing.T) {
	root := t.TempDir()
	store, err := state.Open(filepath.Join(root, "mux"), filepath.Join(root, "primary"))
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.AddAccount("Target")
	if err != nil {
		t.Fatal(err)
	}
	threadID := "thread-1"
	if err := store.SetThreadOwner(threadID, target.ID); err != nil {
		t.Fatal(err)
	}

	merged := mergeThreadLists(store, []accountThreadList{
		{accountID: "primary", threads: []map[string]any{{"id": threadID, "title": "source copy"}}},
		{accountID: target.ID, threads: []map[string]any{{"id": threadID, "title": "target copy"}}},
	})
	if len(merged) != 1 {
		t.Fatalf("merged thread count = %d, want 1", len(merged))
	}
	if merged[0]["title"] != "target copy" {
		t.Fatalf("selected thread = %#v, want target copy", merged[0])
	}
	owner, ok := store.ThreadOwner(threadID)
	if !ok || owner != target.ID {
		t.Fatalf("owner = %q, want %q", owner, target.ID)
	}
}

func TestMergeThreadListsKeepsAssignedOwnerWhenOnlySourceListsThread(t *testing.T) {
	root := t.TempDir()
	store, err := state.Open(filepath.Join(root, "mux"), filepath.Join(root, "primary"))
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.AddAccount("Target")
	if err != nil {
		t.Fatal(err)
	}
	threadID := "thread-1"
	if err := store.SetThreadOwner(threadID, target.ID); err != nil {
		t.Fatal(err)
	}

	merged := mergeThreadLists(store, []accountThreadList{{
		accountID: "primary",
		threads:   []map[string]any{{"id": threadID, "title": "visible copy"}},
	}})
	if len(merged) != 1 {
		t.Fatalf("merged thread count = %d, want 1", len(merged))
	}
	owner, _ := store.ThreadOwner(threadID)
	if owner != target.ID {
		t.Fatalf("owner changed to %q, want %q", owner, target.ID)
	}
}
