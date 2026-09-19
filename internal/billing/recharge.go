package billing

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const rechargeCatchUpLimit = 1000

var rechargeModes = map[string]bool{"add": true, "reset": true}

func validRechargeMode(value string) bool {
	return rechargeModes[strings.ToLower(strings.TrimSpace(value))]
}

var rechargeCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

func parseRechargeCron(value string) (cron.Schedule, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("recharge cron expression is required")
	}
	schedule, err := rechargeCronParser.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("invalid recharge cron expression: %w", err)
	}
	return schedule, nil
}

func nextRechargeAt(now time.Time, expression string) time.Time {
	schedule, err := parseRechargeCron(expression)
	if err != nil {
		return time.Time{}
	}
	return schedule.Next(now)
}

func applyDueRechargesTx(tx *sql.Tx, now time.Time) error {
	rows, err := tx.Query(`SELECT api_key_id, balance, recharge_amount, recharge_cron, recharge_mode, recharge_next_at
		FROM api_key_accounts WHERE balance IS NOT NULL AND recharge_amount > 0 AND recharge_cron <> '' AND recharge_next_at <> '' AND recharge_next_at <= ?`, now.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("query due API key recharges: %w", err)
	}
	defer rows.Close()
	type dueRecharge struct {
		id                 string
		balance, amount    float64
		cron, mode, nextAt string
	}
	var due []dueRecharge
	for rows.Next() {
		var item dueRecharge
		if err := rows.Scan(&item.id, &item.balance, &item.amount, &item.cron, &item.mode, &item.nextAt); err != nil {
			return err
		}
		due = append(due, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range due {
		next, err := time.Parse(time.RFC3339Nano, item.nextAt)
		if err != nil {
			continue
		}
		schedule, err := parseRechargeCron(item.cron)
		if err != nil || !validRechargeMode(item.mode) {
			continue
		}
		count := 0
		for !next.After(now) && count < rechargeCatchUpLimit {
			count++
			next = schedule.Next(next)
		}
		if count == 0 {
			continue
		}
		balance := item.balance
		if item.mode == "reset" {
			balance = item.amount
		} else {
			balance += item.amount * float64(count)
		}
		if !finite(balance) {
			return fmt.Errorf("API key recharge exceeds the supported range")
		}
		if _, err := tx.Exec(`UPDATE api_key_accounts SET balance=?, balance_version=?, recharge_next_at=?, updated_at=? WHERE api_key_id=?`, balance, now.Format(time.RFC3339Nano), next.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), item.id); err != nil {
			return fmt.Errorf("apply API key recharge %q: %w", item.id, err)
		}
	}
	return nil
}

func (s *Store) applyDueRechargesLocked(now time.Time) error {
	return s.withTransaction(func(tx *sql.Tx) error { return applyDueRechargesTx(tx, now) })
}

// ApplyDueRecharges applies all periods that elapsed before now. It is called
// by the wallet API and request interceptor, while the worker keeps balances
// current even when no requests are arriving.
func (s *Store) ApplyDueRecharges(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyDueRechargesLocked(now.UTC())
}

func (s *Store) startRechargeWorker() {
	s.rechargeStop = make(chan struct{})
	s.rechargeDone.Add(1)
	go func() {
		defer s.rechargeDone.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				if err := s.ApplyDueRecharges(now); err != nil {
					s.mu.Lock()
					s.lastErr = err
					s.mu.Unlock()
				}
			case <-s.rechargeStop:
				return
			}
		}
	}()
}

func normalizeRecharge(amount *float64, expression, mode *string) (float64, string, string, error) {
	value := 0.0
	if amount != nil {
		value = *amount
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, "", "", fmt.Errorf("recharge amount must be a non-negative number")
	}
	cronExpression := ""
	if expression != nil {
		cronExpression = strings.TrimSpace(*expression)
	}
	m := "add"
	if mode != nil && strings.TrimSpace(*mode) != "" {
		m = strings.ToLower(strings.TrimSpace(*mode))
	}
	if value == 0 {
		return 0, "", "", nil
	}
	if _, err := parseRechargeCron(cronExpression); err != nil {
		return 0, "", "", err
	}
	if !validRechargeMode(m) {
		return 0, "", "", fmt.Errorf("invalid recharge mode")
	}
	return value, cronExpression, m, nil
}
