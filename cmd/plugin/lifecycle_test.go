package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/chyern/CPA-Billing-Management/internal/abi"
	"github.com/chyern/CPA-Billing-Management/internal/billing"
)

func isolatePluginStore(t *testing.T) string {
	t.Helper()
	previous, previousErr, previousDir, previousFallback := store, storeErr, storeDataDir, storeFallbackDir
	fallback := filepath.Join(t.TempDir(), "default")
	store, storeErr, storeDataDir = nil, nil, ""
	storeFallbackDir = func() string { return fallback }
	t.Cleanup(func() {
		if store != nil {
			_ = store.Close()
		}
		store, storeErr, storeDataDir, storeFallbackDir = previous, previousErr, previousDir, previousFallback
	})
	return fallback
}

func lifecycleRequest(dir string) []byte {
	config := "currency: USD\n"
	if dir != "" {
		config += fmt.Sprintf("cpa_billing_data_dir: %q\n", dir)
	}
	raw, _ := json.Marshal(abi.LifecycleRequest{ConfigYAML: []byte(config)})
	return raw
}

func TestLifecycleKeepsConfiguredDirectoryAndRecoversFromInvalidConfig(t *testing.T) {
	fallback := isolatePluginStore(t)
	dir := filepath.Join(t.TempDir(), "custom")
	if _, err := handleMethod(abi.MethodPluginRegister, lifecycleRequest(dir)); err != nil {
		t.Fatal(err)
	}
	configured := store
	if raw, err := handleMethod(abi.MethodUsageHandle, []byte(`{"Model":"custom-model","ActualCost":2}`)); err != nil || !resultContains(t, raw, `"accepted":true`) {
		t.Fatalf("usage: %s %v", raw, err)
	}
	req, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodGet, Path: "/cpa-billing-management/summary"})
	raw, err := handleMethod(abi.MethodManagementHandle, req)
	if err != nil {
		t.Fatal(err)
	}
	var summary billing.Summary
	if err := json.Unmarshal(managementBody(t, raw), &summary); err != nil {
		t.Fatal(err)
	}
	if store != configured || storeDataDir != dir || summary.Totals.Requests != 1 || summary.Totals.Cost != 2 {
		t.Fatalf("store switched on normal request: dir=%s summary=%+v", storeDataDir, summary.Totals)
	}
	bad := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(bad, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := handleMethod(abi.MethodPluginReconfigure, lifecycleRequest(bad)); err == nil {
		t.Fatal("invalid directory accepted")
	}
	if store != configured || storeDataDir != dir {
		t.Fatal("failed reconfiguration replaced active store")
	}
	if raw, err := handleMethod(abi.MethodUsageHandle, []byte(`{"Model":"custom-model","ActualCost":3}`)); err != nil || !resultContains(t, raw, `"accepted":true`) {
		t.Fatalf("old store unusable: %s %v", raw, err)
	}
	if _, err := handleMethod(abi.MethodPluginReconfigure, lifecycleRequest("")); err != nil {
		t.Fatal(err)
	}
	if store == configured || storeDataDir != fallback {
		t.Fatalf("explicit empty config did not restore default: %s", storeDataDir)
	}
	reopened, err := billing.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.Summary().Totals; got.Requests != 2 || got.Cost != 5 {
		t.Fatalf("custom data not persisted: %+v", got)
	}
}

func TestConcurrentLifecycleAndUsagePreserveAllAcceptedEvents(t *testing.T) {
	isolatePluginStore(t)
	dirs := []string{filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")}
	if _, err := handleMethod(abi.MethodPluginRegister, lifecycleRequest(dirs[0])); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	errors := make(chan error, 100)
	for worker := 0; worker < 4; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := 0; i < 15; i++ {
				raw, err := handleMethod(abi.MethodUsageHandle, []byte(`{"Model":"race-model","ActualCost":1}`))
				if err != nil {
					errors <- err
					continue
				}
				var envelope struct {
					Result struct {
						Accepted bool `json:"accepted"`
					} `json:"result"`
				}
				if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.Result.Accepted {
					errors <- fmt.Errorf("usage rejected: %s", raw)
				}
			}
		}()
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		for i := 0; i < 12; i++ {
			if _, err := handleMethod(abi.MethodPluginReconfigure, lifecycleRequest(dirs[i%2])); err != nil {
				errors <- err
			}
		}
	}()
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	var total int64
	for _, dir := range dirs {
		s, err := billing.NewStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		total += s.Summary().Totals.Requests
		_ = s.Close()
	}
	if total != 60 {
		t.Fatalf("persisted requests=%d, want 60", total)
	}
}

func TestUsageReportsInvalidCostAndPersistenceFailure(t *testing.T) {
	dir := isolatePluginStore(t)
	if _, err := handleMethod(abi.MethodPluginRegister, nil); err != nil {
		t.Fatal(err)
	}
	for _, cost := range []string{`"NaN"`, `"+Inf"`, `"-Inf"`, `"1e999"`, `"bad"`} {
		raw, err := handleMethod(abi.MethodUsageHandle, []byte(`{"ActualCost":`+cost+`}`))
		if err != nil || !resultContains(t, raw, `"accepted":false`) {
			t.Fatalf("invalid cost accepted: %s %v", raw, err)
		}
	}
	db, err := sql.Open("sqlite3", filepath.Join(dir, "billing.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER reject_usage BEFORE INSERT ON usage_events BEGIN SELECT RAISE(ABORT, 'test write failure'); END`); err != nil {
		t.Fatal(err)
	}
	raw, err := handleMethod(abi.MethodUsageHandle, []byte(`{"ActualCost":2}`))
	if err != nil || !resultContains(t, raw, `"accepted":false`) {
		t.Fatalf("failed write acknowledged: %s %v", raw, err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_usage`); err != nil {
		t.Fatal(err)
	}
	raw, err = handleMethod(abi.MethodUsageHandle, []byte(`{"ActualCost":3}`))
	if err != nil || !resultContains(t, raw, `"accepted":true`) {
		t.Fatalf("retry rejected: %s %v", raw, err)
	}
	if got := store.Summary().Totals; got.Requests != 1 || got.Cost != 3 {
		t.Fatalf("failed usage polluted summary: %+v", got)
	}
}
