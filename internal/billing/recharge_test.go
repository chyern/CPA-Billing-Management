package billing

import (
	"testing"
	"time"
)

func TestDueRechargeAddsMissedPeriodsAndResetsSchedule(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	key := "sk-recharge"
	id := APIKeyIdentifier(key)
	if err := s.SetKeyBalances([]APIKeyBalance{{APIKeyID: id, APIKey: key, Balance: 5}}); err != nil {
		t.Fatal(err)
	}
	amount, expression, mode := 2.0, "0 0 * * *", "add"
	row := balancesByID(t, s)[id]
	if err := s.PatchKeyBalances([]APIKeyBalanceUpdate{{APIKeyID: id, RechargeAmount: &amount, RechargeCron: &expression, RechargeMode: &mode, ExpectedBalanceVersion: &row.BalanceVersion}}); err != nil {
		t.Fatal(err)
	}
	var storedCron string
	if err := s.db.QueryRow(`SELECT recharge_cron FROM api_key_accounts WHERE api_key_id=?`, id).Scan(&storedCron); err != nil || storedCron != expression {
		t.Fatalf("stored cron=%q err=%v", storedCron, err)
	}
	past := time.Now().UTC().Add(-49 * time.Hour).Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`UPDATE api_key_accounts SET recharge_next_at=? WHERE api_key_id=?`, past, id); err != nil {
		t.Fatal(err)
	}
	items, err := s.KeyBalances()
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Balance != 11 || !items[0].RechargeConfigured {
		t.Fatalf("add recharge result=%+v", items[0])
	}
	amount, mode = 10, "reset"
	row = items[0]
	if err := s.PatchKeyBalances([]APIKeyBalanceUpdate{{APIKeyID: id, RechargeAmount: &amount, RechargeCron: &expression, RechargeMode: &mode, ExpectedBalanceVersion: &row.BalanceVersion}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE api_key_accounts SET recharge_next_at=? WHERE api_key_id=?`, past, id); err != nil {
		t.Fatal(err)
	}
	items, err = s.KeyBalances()
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Balance != 10 {
		t.Fatalf("reset recharge result=%+v", items[0])
	}
}

func TestRechargeRequiresTrackedBalanceAndVersion(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	amount, expression := 1.0, "0 0 * * *"
	if err := s.PatchKeyBalances([]APIKeyBalanceUpdate{{APIKeyID: "missing", RechargeAmount: &amount, RechargeCron: &expression}}); err == nil {
		t.Fatal("recharge without version accepted")
	}
	if err := s.PatchKeyBalances([]APIKeyBalanceUpdate{{APIKeyID: "missing", RechargeAmount: &amount, RechargeCron: &expression, ExpectedBalanceVersion: balancePointer("")}}); err == nil {
		t.Fatal("recharge without balance accepted")
	}
}
