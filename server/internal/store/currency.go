package store

import (
	"context"
	"database/sql"
)

// CurrencyLog 为全货币变动流水条目（§6.2 强制）；金额一律整数（§6.3）。
type CurrencyLog struct {
	ID           int64
	CharacterID  int64
	Currency     string
	Delta        int64
	BalanceAfter int64
	Reason       string
	RefID        string
}

type CurrencyRepo struct{ db *sql.DB }

func (r *CurrencyRepo) AppendLog(ctx context.Context, charID int64, currency string, delta, balanceAfter int64, reason, refID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO currency_logs (character_id, currency, delta, balance_after, reason, ref_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, strftime('%s','now'))`,
		charID, currency, delta, balanceAfter, reason, refID)
	return err
}

func (r *CurrencyRepo) ListRecent(ctx context.Context, charID int64, limit int) ([]CurrencyLog, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, character_id, currency, delta, balance_after, reason, ref_id
		 FROM currency_logs WHERE character_id = ? ORDER BY id DESC LIMIT ?`, charID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CurrencyLog
	for rows.Next() {
		var l CurrencyLog
		if err := rows.Scan(&l.ID, &l.CharacterID, &l.Currency, &l.Delta, &l.BalanceAfter, &l.Reason, &l.RefID); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
