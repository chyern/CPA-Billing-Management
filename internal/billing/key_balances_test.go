package billing

import (
	"errors"
	"math"
	"testing"
)

func balancePointer[T any](value T) *T { return &value }

func balancesByID(t *testing.T, store *Store) map[string]APIKeyBalance {
	t.Helper()
	rows, err := store.KeyBalances()
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string]APIKeyBalance, len(rows))
	for _, row := range rows {
		result[row.APIKeyID] = row
	}
	return result
}

func TestKeyBalancePatchPreservesOtherKeysAndConcurrentDeductions(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	key, other := "sk-patch-first", "sk-patch-exhausted"
	id, otherID := APIKeyIdentifier(key), APIKeyIdentifier(other)
	if err := store.SetKeyBalances([]APIKeyBalance{{APIKeyID: id, APIKey: key, Balance: 10}, {APIKeyID: otherID, APIKey: other, Balance: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetKeyBalanceNotes([]APIKeyBalance{{APIKeyID: otherID, APIKey: other, Note: "keep"}}); err != nil {
		t.Fatal(err)
	}
	snapshot := balancesByID(t, store)
	store.HandleUsage(UsageRecord{APIKey: key, Cost: 2, CostProvided: true})
	store.HandleUsage(UsageRecord{APIKey: other, Cost: 3, CostProvided: true})
	if err := store.PatchKeyBalances([]APIKeyBalanceUpdate{{APIKeyID: id, APIKey: key, Note: balancePointer("updated note")}}); err != nil {
		t.Fatal(err)
	}
	current := balancesByID(t, store)
	if current[id].Balance != 8 || current[id].Note != "updated note" || current[otherID].Balance != -2 || current[otherID].Note != "keep" || !current[otherID].Configured {
		t.Fatalf("patch changed unrelated state: %+v", current)
	}
	if current[id].BalanceVersion == snapshot[id].BalanceVersion {
		t.Fatal("usage did not advance balance version")
	}
	if err := store.PatchKeyBalances([]APIKeyBalanceUpdate{{APIKeyID: otherID, Note: balancePointer("exhausted")}}); err != nil {
		t.Fatal(err)
	}
	if balance, tracked, err := store.BalanceForCallerScope(CallerScope(other)); err != nil || !tracked || balance != -2 {
		t.Fatalf("negative key was untracked: %v %v %v", balance, tracked, err)
	}
}

func TestKeyBalancePatchRejectsStaleBalanceDisableAndDeleteAtomically(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	key := "sk-patch-conflict"
	id := APIKeyIdentifier(key)
	if err := store.SetKeyBalances([]APIKeyBalance{{APIKeyID: id, APIKey: key, Balance: 10}}); err != nil {
		t.Fatal(err)
	}
	initial := balancesByID(t, store)[id]
	// Returning to the same amount must not allow a stale editor to write it.
	for _, amount := range []float64{8, 10} {
		row := balancesByID(t, store)[id]
		if err := store.PatchKeyBalances([]APIKeyBalanceUpdate{{APIKeyID: id, Balance: &amount, ExpectedBalanceVersion: &row.BalanceVersion}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, change := range []APIKeyBalanceUpdate{
		{Balance: balancePointer(20.0)}, {Configured: balancePointer(false)}, {Delete: true},
	} {
		change.APIKeyID = id
		change.ExpectedBalanceVersion = &initial.BalanceVersion
		err := store.PatchKeyBalances([]APIKeyBalanceUpdate{{APIKeyID: "unrelated", Note: balancePointer("must roll back")}, change})
		if !errors.Is(err, ErrBalanceConflict) {
			t.Fatalf("stale update accepted: %+v, err=%v", change, err)
		}
		current := balancesByID(t, store)
		if _, exists := current["unrelated"]; exists {
			t.Fatal("partial patch persisted despite conflict")
		}
		if current[id].Balance != 10 || !current[id].Configured {
			t.Fatalf("conflict changed balance: %+v", current[id])
		}
	}
	row := balancesByID(t, store)[id]
	if err := store.PatchKeyBalances([]APIKeyBalanceUpdate{{APIKeyID: id, Configured: balancePointer(false), ExpectedBalanceVersion: &row.BalanceVersion, Note: balancePointer("note stays")}}); err != nil {
		t.Fatal(err)
	}
	row = balancesByID(t, store)[id]
	if row.Configured || row.Note != "note stays" {
		t.Fatalf("disable lost note: %+v", row)
	}
	if err := store.PatchKeyBalances([]APIKeyBalanceUpdate{{APIKeyID: id, Delete: true, ExpectedBalanceVersion: balancePointer("")}}); err != nil {
		t.Fatal(err)
	}
	if len(balancesByID(t, store)) != 0 {
		t.Fatal("delete retained key metadata")
	}
}

func TestKeyBalancePatchValidatesManualBalancesAndRequiresVersion(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, amount := range []float64{-1, math.NaN(), math.Inf(1)} {
		if err := store.PatchKeyBalances([]APIKeyBalanceUpdate{{APIKeyID: "key", Balance: &amount, ExpectedBalanceVersion: balancePointer("")}}); err == nil {
			t.Fatalf("accepted manual balance %v", amount)
		}
	}
	for _, update := range []APIKeyBalanceUpdate{{Balance: balancePointer(0.0)}, {Configured: balancePointer(false)}, {Delete: true}} {
		update.APIKeyID = "key"
		if err := store.PatchKeyBalances([]APIKeyBalanceUpdate{update}); err == nil {
			t.Fatalf("accepted change without version: %+v", update)
		}
	}
	if err := store.PatchKeyBalances([]APIKeyBalanceUpdate{{APIKeyID: "key", Balance: balancePointer(0.0), ExpectedBalanceVersion: balancePointer("")}}); err != nil {
		t.Fatal(err)
	}
	if !balancesByID(t, store)["key"].Configured {
		t.Fatal("zero balance did not enable tracking")
	}
}
