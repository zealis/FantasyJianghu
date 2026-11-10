package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrCharLimit = errors.New("store: character limit reached")

type Character struct {
	ID        int64
	AccountID int64
	Name      string
	Faction   string
	Gender    string // 男|女（M1.5 创角两步流）
	Level     int
	Exp       int
	Breakthrough int // 已跨越的最高突破关口（0=未突破，M2c 切片三）
	UnspentPoints int // 未分配属性点（每级 +4，M2c 切片三）
	Str, Spi, Agi, Con int // 一级属性已分配点（M2c 切片四）
	MapID     string
	PosX      float64
	PosY      float64
	CreatedAt time.Time
}

type CharactersRepo struct{ db *sql.DB }

// PointsMap 返回一级属性已分配点（供 battle 派生叠加，M2c 切片四）。
func (c Character) PointsMap() map[string]int {
	return map[string]int{"str": c.Str, "spi": c.Spi, "agi": c.Agi, "con": c.Con}
}

// CreateCharacter 在单事务内完成"计数→插入"，保证一账号 ≤3 角色
// 并发下也不超建（§6.4 原子结算口径）。
func (r *CharactersRepo) CreateCharacter(ctx context.Context, accountID int64, name, faction, gender string) (int64, error) {
	var id int64
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM characters WHERE account_id = ? AND deleted_at = 0`, accountID).
			Scan(&n); err != nil {
			return err
		}
		if n >= 3 {
			return ErrCharLimit
		}
		now := time.Now().Unix()
		res, err := tx.ExecContext(ctx,
			`INSERT INTO characters (account_id, name, faction, gender, created_at) VALUES (?, ?, ?, ?, ?)`,
			accountID, name, faction, gender, now)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

func (r *CharactersRepo) GetCharacter(ctx context.Context, id int64) (Character, error) {
	var c Character
	var createdAt int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id, account_id, name, faction, gender, level, exp, breakthrough, unspent_points, str, spi, agi, con, map_id, pos_x, pos_y, created_at
		 FROM characters WHERE id = ? AND deleted_at = 0`, id).
		Scan(&c.ID, &c.AccountID, &c.Name, &c.Faction, &c.Gender, &c.Level, &c.Exp, &c.Breakthrough, &c.UnspentPoints, &c.Str, &c.Spi, &c.Agi, &c.Con, &c.MapID, &c.PosX, &c.PosY, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Character{}, ErrNotFound
	}
	if err != nil {
		return Character{}, err
	}
	c.CreatedAt = time.Unix(createdAt, 0)
	return c, nil
}

func (r *CharactersRepo) ListByAccount(ctx context.Context, accountID int64) ([]Character, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, account_id, name, faction, gender, level, exp, map_id, pos_x, pos_y, created_at
		 FROM characters WHERE account_id = ? AND deleted_at = 0 ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Character
	for rows.Next() {
		var c Character
		var createdAt int64
		if err := rows.Scan(&c.ID, &c.AccountID, &c.Name, &c.Faction, &c.Gender, &c.Level, &c.Exp, &c.MapID, &c.PosX, &c.PosY, &createdAt); err != nil {
			return nil, err
		}
		c.CreatedAt = time.Unix(createdAt, 0)
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdatePosition 落盘角色位置（§6.1 位置并入 characters）。
func (r *CharactersRepo) UpdatePosition(ctx context.Context, id int64, mapID string, x, y float64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE characters SET map_id = ?, pos_x = ?, pos_y = ? WHERE id = ?`,
		mapID, x, y, id)
	return err
}

// SoftDelete 软删角色（§6.3：角色删除为软删）。
func (r *CharactersRepo) SoftDelete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE characters SET deleted_at = ? WHERE id = ?`, time.Now().Unix(), id)
	return err
}

func (r *CharactersRepo) withTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		if rb := tx.Rollback(); rb != nil {
			return fmt.Errorf("%w（回滚亦失败: %v）", err, rb)
		}
		return err
	}
	return tx.Commit()
}
