package store

import (
	"context"
	"database/sql"
	"errors"
)

// ErrBreakthrough 为当前不满足突破条件（非关口/已突破/经验未满）。
var ErrBreakthrough = errors.New("store: 当前不可突破")

// LevelCurve 为等级结算参数（由 service 从配置注入，store 不依赖 config）。
// ExpToNext 为 nil 时只累加经验不升级（兼容未配置等级域的旧测试/旧包）。
type LevelCurve struct {
	ExpToNext      func(level int) int
	IsGate         func(level int) bool
	MaxLevel       int
	PointsPerLevel int
}

type levelState struct {
	level        int
	exp          int
	breakthrough int
	points       int
}

// settle 按 GDD §4.2 结算：经验满即升；关口未突破则停（溢出保留）；一次最多停到下一关口；满级停。
func (s *levelState) settle(c LevelCurve) {
	if c.ExpToNext == nil {
		return
	}
	for s.level < c.MaxLevel {
		if c.IsGate != nil && c.IsGate(s.level) && s.breakthrough < s.level {
			break // 停在关口，溢出经验保留，待手动突破
		}
		need := c.ExpToNext(s.level)
		if need <= 0 || s.exp < need {
			break
		}
		s.exp -= need
		s.level++
		s.points += c.PointsPerLevel
	}
}

func readLevelTx(ctx context.Context, tx *sql.Tx, charID int64) (levelState, error) {
	var s levelState
	err := tx.QueryRowContext(ctx,
		`SELECT level, exp, breakthrough, unspent_points FROM characters WHERE id = ?`, charID).
		Scan(&s.level, &s.exp, &s.breakthrough, &s.points)
	return s, err
}

func writeLevelTx(ctx context.Context, tx *sql.Tx, charID int64, s levelState) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE characters SET level = ?, exp = ?, breakthrough = ?, unspent_points = ? WHERE id = ?`,
		s.level, s.exp, s.breakthrough, s.points, charID)
	return err
}

// AddExpTx 在既有事务内累加经验并结算升级（单事务，溢出不丢）。
func AddExpTx(ctx context.Context, tx *sql.Tx, charID int64, gain int, c LevelCurve) error {
	s, err := readLevelTx(ctx, tx, charID)
	if err != nil {
		return err
	}
	s.exp += gain
	s.settle(c)
	return writeLevelTx(ctx, tx, charID, s)
}

// ErrPoints 为属性点不足或增量非法。
var ErrPoints = errors.New("store: 属性点不足或增量非法")

// AllocatePoints 单事务加点：校验增量非负且总和<=unspent，扣 unspent 加四属性（M2c 切片四）。
func (r *CharactersRepo) AllocatePoints(ctx context.Context, charID int64, delta map[string]int) error {
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		var unspent, str, spi, agi, con int
		if err := tx.QueryRowContext(ctx,
			`SELECT unspent_points, str, spi, agi, con FROM characters WHERE id = ?`, charID).
			Scan(&unspent, &str, &spi, &agi, &con); err != nil {
			return err
		}
		sum := 0
		for _, k := range []string{"str", "spi", "agi", "con"} {
			if delta[k] < 0 {
				return ErrPoints
			}
			sum += delta[k]
		}
		if sum == 0 || sum > unspent {
			return ErrPoints
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE characters SET unspent_points = unspent_points - ?, str = str + ?, spi = spi + ?, agi = agi + ?, con = con + ? WHERE id = ?`,
			sum, delta["str"], delta["spi"], delta["agi"], delta["con"], charID)
		return err
	})
}

// ResetPoints 单事务洗点：四属性归零、unspent 返还全部已分配点（本轮免费，洗髓丹消耗待物品轮）。
func (r *CharactersRepo) ResetPoints(ctx context.Context, charID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE characters SET unspent_points = unspent_points + str + spi + agi + con, str = 0, spi = 0, agi = 0, con = 0 WHERE id = ?`,
		charID)
	return err
}

// Breakthrough 手动突破当前关口：要求恰处关口、未突破、经验已满；突破后继续结算溢出
// 但最多停在下一关口（GDD §4.2 不能一次跨两关）。
func (r *CharactersRepo) Breakthrough(ctx context.Context, charID int64, c LevelCurve) (Character, error) {
	err := withTx(ctx, r.db, func(tx *sql.Tx) error {
		s, err := readLevelTx(ctx, tx, charID)
		if err != nil {
			return err
		}
		if c.ExpToNext == nil || c.IsGate == nil || !c.IsGate(s.level) || s.breakthrough >= s.level || s.exp < c.ExpToNext(s.level) {
			return ErrBreakthrough
		}
		s.breakthrough = s.level
		s.settle(c)
		return writeLevelTx(ctx, tx, charID, s)
	})
	if err != nil {
		return Character{}, err
	}
	return r.GetCharacter(ctx, charID)
}