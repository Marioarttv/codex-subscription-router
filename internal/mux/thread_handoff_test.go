package mux

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/b-nnett/codex-subscription-router/internal/state"
)

func TestPrepareThreadRolloutCreatesSelfContainedTargetCopy(t *testing.T) {
	root := t.TempDir()
	threadID := "01a02d8b-b25e-74d1-aa5a-12a75cb07ca0"
	source := state.Account{ID: "source", CodexHome: filepath.Join(root, "source")}
	target := state.Account{ID: "target", CodexHome: filepath.Join(root, "target")}
	sourcePath := filepath.Join(
		source.CodexHome,
		"sessions", "2026", "08", "23",
		"rollout-2026-08-23T09-35-25-"+threadID+".jsonl",
	)
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o700); err != nil {
		t.Fatal(err)
	}
	rollout := strings.Join([]string{
		`{"timestamp":"2026-08-23T07:35:25Z","type":"session_meta","payload":{"id":"` + threadID + `","history_mode":"paginated","cwd":"/tmp/project"}}`,
		`{"timestamp":"2026-08-23T07:35:26Z","type":"response_item","payload":{"type":"message","role":"user","content":[]}}`,
		"",
	}, "\n")
	if err := os.WriteFile(sourcePath, []byte(rollout), 0o600); err != nil {
		t.Fatal(err)
	}

	destination, replaced, err := prepareThreadRollout(source, target, threadID, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if replaced {
		t.Fatal("first handoff unexpectedly replaced a target rollout")
	}
	wantDestination := filepath.Join(
		target.CodexHome,
		"sessions", "2026", "08", "23",
		filepath.Base(sourcePath),
	)
	if destination != wantDestination {
		t.Fatalf("destination = %q, want %q", destination, wantDestination)
	}
	file, err := os.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("target rollout has no session metadata")
	}
	var metadata struct {
		Timestamp string         `json:"timestamp"`
		Payload   map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Payload["history_mode"] != "legacy" {
		t.Fatalf("target history mode = %#v, want legacy", metadata.Payload["history_mode"])
	}
	if metadata.Timestamp != "2026-08-23T07:35:25Z" {
		t.Fatalf("target timestamp = %q, want source timestamp", metadata.Timestamp)
	}
	if !scanner.Scan() || !strings.Contains(scanner.Text(), `"role":"user"`) {
		t.Fatal("target rollout did not preserve history")
	}
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sourceBytes), `"history_mode":"paginated"`) {
		t.Fatal("source rollout was modified")
	}
	updatedRollout := strings.Replace(rollout, "\n", "\n"+`{"timestamp":"2026-08-23T07:35:27Z","type":"event_msg","payload":{"type":"turn_complete"}}`+"\n", 1)
	if err := os.WriteFile(sourcePath, []byte(updatedRollout), 0o600); err != nil {
		t.Fatal(err)
	}
	_, replaced, err = prepareThreadRollout(source, target, threadID, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !replaced {
		t.Fatal("repeat handoff did not replace the stale target rollout")
	}
	targetBytes, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(targetBytes), "turn_complete") {
		t.Fatal("replacement target rollout did not include the latest history")
	}
}

func TestPrepareThreadRolloutRejectsPathsOutsideSourceSessions(t *testing.T) {
	root := t.TempDir()
	source := state.Account{ID: "source", CodexHome: filepath.Join(root, "source")}
	target := state.Account{ID: "target", CodexHome: filepath.Join(root, "target")}
	outside := filepath.Join(root, "rollout-thread-1.jsonl")
	if err := os.WriteFile(outside, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareThreadRollout(source, target, "thread-1", outside); err == nil {
		t.Fatal("expected an out-of-scope rollout path to be rejected")
	}
}
