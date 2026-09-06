package backend

import (
	"slices"
	"testing"
)

func TestPrimaryHistorySharingDoesNotLeakIntoSecondaryAccount(t *testing.T) {
	base := []string{"CODEX_HOME=/official", "CODEX_SQLITE_HOME=/official", "CODEX_MUX_PRIMARY_SQLITE_HOME=/official"}
	primary := childEnvironment("primary", "/router/primary", base)
	if !slices.Contains(primary, "CODEX_HOME=/router/primary") || !slices.Contains(primary, "CODEX_SQLITE_HOME=/official") {
		t.Fatalf("primary did not separate config from history: %v", primary)
	}
	secondary := childEnvironment("secondary", "/router/secondary", base)
	if !slices.Contains(secondary, "CODEX_SQLITE_HOME=/router/secondary") || slices.Contains(secondary, "CODEX_SQLITE_HOME=/official") {
		t.Fatalf("secondary inherited primary history: %v", secondary)
	}
}
