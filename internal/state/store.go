package state

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const stateVersion = 1

type Account struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	CodexHome  string `json:"codexHome"`
	Enabled    bool   `json:"enabled"`
	Controller bool   `json:"controller"`
	CreatedAt  int64  `json:"createdAt"`
}

type persistedState struct {
	Version          int               `json:"version"`
	Accounts         []Account         `json:"accounts"`
	RoutingAccountID string            `json:"routingAccountId,omitempty"`
	ThreadOwner      map[string]string `json:"threadOwner"`
}

// Store persists only routing metadata. OAuth credentials and conversation
// databases remain inside each account's isolated Codex home.
type Store struct {
	mu               sync.RWMutex
	root             string
	path             string
	primaryCodexHome string
	accounts         []Account
	routingAccountID string
	owners           map[string]string
}

func Open(root, primaryCodexHome string) (*Store, error) {
	if root == "" {
		return nil, errors.New("state root is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create state root: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, fmt.Errorf("secure state root: %w", err)
	}

	store := &Store{
		root:             root,
		path:             filepath.Join(root, "state.json"),
		primaryCodexHome: primaryCodexHome,
		owners:           make(map[string]string),
	}
	data, err := os.ReadFile(store.path)
	switch {
	case err == nil:
		var persisted persistedState
		if err := json.Unmarshal(data, &persisted); err != nil {
			return nil, fmt.Errorf("read state: %w", err)
		}
		if persisted.Version != stateVersion {
			return nil, fmt.Errorf("unsupported state version %d", persisted.Version)
		}
		store.accounts = persisted.Accounts
		store.routingAccountID = persisted.RoutingAccountID
		if persisted.ThreadOwner != nil {
			store.owners = persisted.ThreadOwner
		}
	case errors.Is(err, os.ErrNotExist):
		store.accounts = []Account{{
			ID:         "primary",
			Label:      "Primary",
			CodexHome:  primaryCodexHome,
			Enabled:    true,
			Controller: true,
			CreatedAt:  time.Now().Unix(),
		}}
		if err := store.saveLocked(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("read state: %w", err)
	}
	for _, account := range store.accounts {
		if samePath(account.CodexHome, primaryCodexHome) {
			continue
		}
		if err := syncIsolatedConfig(primaryCodexHome, account.CodexHome); err != nil {
			return nil, fmt.Errorf("sync account %q config: %w", account.ID, err)
		}
	}
	return store, nil
}

func (s *Store) Root() string {
	return s.root
}

// SyncManagedConfig propagates desktop-managed configuration (including
// plugins, marketplaces, skills, and MCP server definitions) to every
// isolated subscription. Credential stores and project trust remain local to
// each account; syncIsolatedConfig deliberately excludes both.
func (s *Store) SyncManagedConfig() error {
	s.mu.RLock()
	accounts := slices.Clone(s.accounts)
	primaryCodexHome := s.primaryCodexHome
	s.mu.RUnlock()

	for _, account := range accounts {
		if samePath(account.CodexHome, primaryCodexHome) {
			continue
		}
		if err := syncIsolatedConfig(primaryCodexHome, account.CodexHome); err != nil {
			return fmt.Errorf("sync account %q config: %w", account.ID, err)
		}
	}
	return nil
}

func (s *Store) Accounts() []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.accounts)
}

func (s *Store) Account(id string) (Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, account := range s.accounts {
		if account.ID == id {
			return account, true
		}
	}
	return Account{}, false
}

// RoutingAccountID returns the account selected for new threads. An empty
// value enables quota-aware automatic routing.
func (s *Store) RoutingAccountID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.routingAccountID
}

// SetRoutingAccountID selects an account for new threads. Passing an empty ID
// restores quota-aware automatic routing. Existing thread ownership is never
// changed by this preference.
func (s *Store) SetRoutingAccountID(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	if id != "" {
		found := false
		for _, account := range s.accounts {
			if account.ID == id {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("account %q not found", id)
		}
	}
	if s.routingAccountID == id {
		return nil
	}
	s.routingAccountID = id
	return s.saveLocked()
}

func (s *Store) Controller() (Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, account := range s.accounts {
		if account.Controller {
			return account, true
		}
	}
	if len(s.accounts) == 0 {
		return Account{}, false
	}
	return s.accounts[0], true
}

func (s *Store) AddAccount(label string) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	label = strings.TrimSpace(label)
	if label == "" {
		label = nextSubscriptionLabel(s.accounts)
	}
	id, err := randomID()
	if err != nil {
		return Account{}, err
	}
	codexHome := filepath.Join(s.root, "accounts", id, "codex-home")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return Account{}, fmt.Errorf("create account home: %w", err)
	}
	if err := os.Chmod(codexHome, 0o700); err != nil {
		return Account{}, fmt.Errorf("secure account home: %w", err)
	}
	if err := syncIsolatedConfig(s.primaryCodexHome, codexHome); err != nil {
		return Account{}, fmt.Errorf("write account config: %w", err)
	}

	account := Account{
		ID:        id,
		Label:     label,
		CodexHome: codexHome,
		Enabled:   true,
		CreatedAt: time.Now().Unix(),
	}
	s.accounts = append(s.accounts, account)
	if err := s.saveLocked(); err != nil {
		return Account{}, err
	}
	return account, nil
}

func nextSubscriptionLabel(accounts []Account) string {
	used := make(map[string]struct{}, len(accounts))
	for _, account := range accounts {
		used[strings.ToLower(strings.TrimSpace(account.Label))] = struct{}{}
	}
	for slot := 2; ; slot++ {
		label := fmt.Sprintf("Subscription %d", slot)
		if _, exists := used[strings.ToLower(label)]; !exists {
			return label
		}
	}
}

func (s *Store) UpdateAccount(id string, label *string, enabled *bool) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.accounts {
		if s.accounts[index].ID != id {
			continue
		}
		if label != nil {
			trimmed := strings.TrimSpace(*label)
			if trimmed == "" {
				return Account{}, errors.New("account label cannot be empty")
			}
			s.accounts[index].Label = trimmed
		}
		if enabled != nil {
			s.accounts[index].Enabled = *enabled
		}
		if err := s.saveLocked(); err != nil {
			return Account{}, err
		}
		return s.accounts[index], nil
	}
	return Account{}, fmt.Errorf("account %q not found", id)
}

// RemoveAccount permanently removes a non-controller account and its isolated
// Codex home. Accounts that still own threads must be retained so those threads
// do not become inaccessible.
func (s *Store) RemoveAccount(id string) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	index := -1
	var account Account
	for candidateIndex, candidate := range s.accounts {
		if candidate.ID == id {
			index = candidateIndex
			account = candidate
			break
		}
	}
	if index == -1 {
		return Account{}, fmt.Errorf("account %q not found", id)
	}
	if account.Controller {
		return Account{}, errors.New("the Primary account cannot be removed")
	}
	threadCount := 0
	for _, ownerID := range s.owners {
		if ownerID == account.ID {
			threadCount++
		}
	}
	if threadCount != 0 {
		return Account{}, fmt.Errorf(
			"%s cannot be removed because it owns %d task(s)",
			account.Label,
			threadCount,
		)
	}

	expectedHome := filepath.Join(s.root, "accounts", account.ID, "codex-home")
	if !samePath(account.CodexHome, expectedHome) {
		return Account{}, errors.New("refusing to remove an account with an unexpected Codex home")
	}
	accountRoot := filepath.Dir(account.CodexHome)
	stagingID, err := randomID()
	if err != nil {
		return Account{}, fmt.Errorf("prepare account removal: %w", err)
	}
	stagingRoot := filepath.Join(s.root, ".removing-"+stagingID)
	homeStaged := false
	if err := os.Rename(accountRoot, stagingRoot); err == nil {
		homeStaged = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return Account{}, fmt.Errorf("stage isolated account data for removal: %w", err)
	}

	previousAccounts := s.accounts
	previousRoutingAccountID := s.routingAccountID
	s.accounts = append(slices.Clone(s.accounts[:index]), s.accounts[index+1:]...)
	if s.routingAccountID == account.ID {
		s.routingAccountID = ""
	}
	if err := s.saveLocked(); err != nil {
		s.accounts = previousAccounts
		s.routingAccountID = previousRoutingAccountID
		if homeStaged {
			_ = os.Rename(stagingRoot, accountRoot)
		}
		return Account{}, err
	}
	if homeStaged {
		if err := os.RemoveAll(stagingRoot); err != nil {
			return account, fmt.Errorf("remove isolated account data: %w", err)
		}
	}
	return account, nil
}

func (s *Store) ThreadOwner(threadID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	owner, ok := s.owners[threadID]
	return owner, ok
}

func (s *Store) SetThreadOwner(threadID, accountID string) error {
	if threadID == "" || accountID == "" {
		return errors.New("thread and account IDs are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owners[threadID] == accountID {
		return nil
	}
	s.owners[threadID] = accountID
	return s.saveLocked()
}

func (s *Store) ThreadCounts() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := make(map[string]int)
	for _, accountID := range s.owners {
		counts[accountID]++
	}
	return counts
}

func (s *Store) saveLocked() error {
	persisted := persistedState{
		Version:          stateVersion,
		Accounts:         s.accounts,
		RoutingAccountID: s.routingAccountID,
		ThreadOwner:      s.owners,
	}
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		return fmt.Errorf("secure state: %w", err)
	}
	if err := os.Rename(temporary, s.path); err != nil {
		return fmt.Errorf("commit state: %w", err)
	}
	return nil
}

func randomID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate account ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
