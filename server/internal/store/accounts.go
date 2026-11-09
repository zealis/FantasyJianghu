package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrNotFound = errors.New("store: not found")

type Account struct {
	ID           int64
	Username     string
	PasswordHash string
	Nickname     string // §9 注册界面昵称（不做唯一校验）
	VipLevel     int
	BanReason    string
	BanUntil     time.Time
	CreatedAt    time.Time
}

type AccountsRepo struct{ db *sql.DB }

func (r *AccountsRepo) CreateAccount(ctx context.Context, username, passwordHash, nickname string) (int64, error) {
	now := time.Now().Unix()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO accounts (username, password_hash, nickname, created_at) VALUES (?, ?, ?, ?)`,
		username, passwordHash, nickname, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *AccountsRepo) GetAccountByUsername(ctx context.Context, username string) (Account, error) {
	var a Account
	var banUntil, createdAt int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, nickname, vip_level, ban_reason, ban_until, created_at
		 FROM accounts WHERE username = ?`, username).
		Scan(&a.ID, &a.Username, &a.PasswordHash, &a.Nickname, &a.VipLevel, &a.BanReason, &banUntil, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	a.BanUntil = time.Unix(banUntil, 0)
	a.CreatedAt = time.Unix(createdAt, 0)
	return a, nil
}

// SetBan 写封禁原因与到期时间（§4 account：封禁含原因与到期）。
func (r *AccountsRepo) SetBan(ctx context.Context, id int64, reason string, until time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE accounts SET ban_reason = ?, ban_until = ? WHERE id = ?`,
		reason, until.Unix(), id)
	return err
}
