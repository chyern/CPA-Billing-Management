package main

import (
	"github.com/chyern/CPA-Billing-Management/internal/billing"
	"os"
	"path/filepath"
	"testing"
)

const upstreamFixture = `codex-api-key:
  - api-key: test-key
    base-url: https://one.example/v1
  - api-key: test-key
    base-url: https://two.example/v1
openai-compatibility:
  - name: gateway
    base-url: https://gateway.example/v1
    api-key-entries:
      - api-key: compat-key
`

func TestUpstreamSnapshotRequiresExactCredentialIdentity(t *testing.T) {
	for _, tc := range []struct{ provider, index, want string }{
		{"codex", "fcf99b67f20b2c79", "one.example"},
		{"codex", "930912adea2ad812", "two.example"},
		{"codex", "", ""},
		{"codex", "not-found", ""},
		{"claude", "fcf99b67f20b2c79", ""},
		{"openai-compatible-gateway", "8ed7d14abbe851b6", "gateway.example"},
	} {
		got := domainSnapshotFromConfig(billing.UsageRecord{Provider: tc.provider, AuthIndex: tc.index, Source: "test-key"}, []byte(upstreamFixture))
		if got != tc.want {
			t.Fatalf("%s/%s: got %q want %q", tc.provider, tc.index, got, tc.want)
		}
	}
}

func TestStoredSnapshotDoesNotFollowConfigOrPriceChanges(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(config, []byte(upstreamFixture), 0600); err != nil {
		t.Fatal(err)
	}
	previous := hostConfigPath
	hostConfigPath = config
	t.Cleanup(func() { hostConfigPath = previous })
	s, err := billing.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"Provider":"codex","Model":"test","ReasoningEffort":"high","AuthIndex":"930912adea2ad812","Source":"test-key","ActualCost":2}`)
	if err := handleUsage(s, raw); err != nil {
		t.Fatal(err)
	}
	before := s.Summary().RecentEvents[0]
	if before.Domain != "two.example" || before.Currency != "USD" || before.ReasoningEffort != "high" {
		t.Fatalf("snapshot=%+v", before)
	}
	if err := os.WriteFile(config, []byte("codex-api-key: []"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRules([]billing.PriceRule{{Match: "*", InputPerMillion: 999}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfigureYAML([]byte("currency: CNY")); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = billing.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	after := s.Summary().RecentEvents[0]
	if after.Domain != before.Domain || after.Provider != before.Provider || after.Cost != 2 || after.Currency != "USD" || after.ReasoningEffort != "high" {
		t.Fatalf("saved snapshot changed: %+v", after)
	}
	if err := handleUsage(s, raw); err != nil {
		t.Fatal(err)
	}
	latest := s.Summary().RecentEvents[1]
	if latest.Domain != "" {
		t.Fatal("unmatched new event must remain empty")
	}
}
