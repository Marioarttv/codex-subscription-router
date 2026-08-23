package mux

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/b-nnett/codex-subscription-router/internal/state"
)

// prepareThreadRollout creates a target-owned, self-contained rollout copy.
// Paginated Codex threads normally depend on account-local SQLite projections;
// changing the copied session metadata to legacy makes Codex rebuild history
// directly from the rollout without sharing either account's database.
func prepareThreadRollout(source, target state.Account, threadID, sourcePath string) (string, bool, error) {
	threadID = strings.TrimSpace(threadID)
	sourcePath = strings.TrimSpace(sourcePath)
	if threadID == "" || sourcePath == "" {
		return "", false, errors.New("thread ID and rollout path are required")
	}
	if source.ID == target.ID {
		return "", false, errors.New("source and target subscriptions must differ")
	}

	sourceSessions, err := filepath.EvalSymlinks(filepath.Join(source.CodexHome, "sessions"))
	if err != nil {
		return "", false, fmt.Errorf("resolve source sessions directory: %w", err)
	}
	resolvedSourcePath, err := filepath.EvalSymlinks(sourcePath)
	if err != nil {
		return "", false, fmt.Errorf("resolve source rollout: %w", err)
	}
	relativePath, err := filepath.Rel(sourceSessions, resolvedSourcePath)
	if err != nil || relativePath == "." || filepath.IsAbs(relativePath) || pathEscapesRoot(relativePath) {
		return "", false, errors.New("source rollout is outside the subscription sessions directory")
	}
	if !strings.Contains(filepath.Base(relativePath), threadID) {
		return "", false, errors.New("source rollout filename does not match the thread ID")
	}

	destinationPath := filepath.Join(target.CodexHome, "sessions", relativePath)
	replacingExisting := false
	if existing, err := os.Open(destinationPath); err == nil {
		replacingExisting = true
		if err := validateAndRewriteSessionMeta(existing, nil, threadID); err != nil {
			_ = existing.Close()
			return "", false, fmt.Errorf("validate existing target rollout: %w", err)
		}
		_ = existing.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("inspect target rollout: %w", err)
	}

	sourceFile, err := os.Open(resolvedSourcePath)
	if err != nil {
		return "", false, fmt.Errorf("open source rollout: %w", err)
	}
	defer sourceFile.Close()
	sourceInfo, err := sourceFile.Stat()
	if err != nil || !sourceInfo.Mode().IsRegular() {
		return "", false, errors.New("source rollout is not a regular file")
	}

	destinationDir := filepath.Dir(destinationPath)
	if err := os.MkdirAll(destinationDir, 0o700); err != nil {
		return "", false, fmt.Errorf("create target sessions directory: %w", err)
	}
	temporary, err := os.CreateTemp(destinationDir, ".codex-mux-handoff-*.jsonl")
	if err != nil {
		return "", false, fmt.Errorf("create target rollout: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return "", false, fmt.Errorf("secure target rollout: %w", err)
	}
	if err := validateAndRewriteSessionMeta(sourceFile, temporary, threadID); err != nil {
		return "", false, err
	}
	if _, err := io.Copy(temporary, sourceFile); err != nil {
		return "", false, fmt.Errorf("copy rollout history: %w", err)
	}
	latestSourceInfo, err := os.Stat(resolvedSourcePath)
	if err != nil || latestSourceInfo.Size() != sourceInfo.Size() || !latestSourceInfo.ModTime().Equal(sourceInfo.ModTime()) {
		return "", false, errors.New("chat history changed during handoff; try again after the current turn finishes")
	}
	if err := temporary.Sync(); err != nil {
		return "", false, fmt.Errorf("flush target rollout: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", false, fmt.Errorf("close target rollout: %w", err)
	}
	if err := os.Rename(temporaryPath, destinationPath); err != nil {
		return "", false, fmt.Errorf("commit target rollout: %w", err)
	}
	committed = true
	return destinationPath, replacingExisting, nil
}

func validateAndRewriteSessionMeta(source io.Reader, destination io.Writer, threadID string) error {
	reader := bufio.NewReader(source)
	firstLine, err := reader.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read session metadata: %w", err)
	}
	if len(firstLine) == 0 {
		return errors.New("source rollout is empty")
	}
	var metadata map[string]any
	if err := json.Unmarshal(firstLine, &metadata); err != nil {
		return fmt.Errorf("decode session metadata: %w", err)
	}
	if eventType, _ := metadata["type"].(string); eventType != "session_meta" {
		return errors.New("rollout does not start with session metadata")
	}
	if timestamp, _ := metadata["timestamp"].(string); timestamp == "" {
		return errors.New("rollout session metadata has no timestamp")
	}
	payload, ok := metadata["payload"].(map[string]any)
	if !ok {
		return errors.New("rollout session metadata has no payload")
	}
	if id, _ := payload["id"].(string); id != threadID {
		return errors.New("rollout session metadata does not match the thread ID")
	}
	if destination == nil {
		return nil
	} else {
		payload["history_mode"] = "legacy"
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("encode target session metadata: %w", err)
		}
		if _, err := destination.Write(append(encoded, '\n')); err != nil {
			return fmt.Errorf("write target session metadata: %w", err)
		}
		if _, err := io.Copy(destination, reader); err != nil {
			return fmt.Errorf("write buffered rollout history: %w", err)
		}
	}
	return nil
}

func pathEscapesRoot(relativePath string) bool {
	return relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator))
}
