package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func levelCurve() LevelCurve {
	return LevelCurve{
		ExpToNext:      func(l int) int { return 100 },
		IsGate:         func(l int) bool { return l == 29 || l == 39 },
		MaxLevel:       89,
		PointsPerLevel: 4,
	}
}

func newLevelChar(t *testing.T) (*DB, int64) {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	acct, err := db.Accounts.CreateAccount(context.Background(), "等级侠", "h", "")
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.Characters.CreateCharacter(context.Background(), acct, "升级测试", "惊雷殿", "男")
	if err != nil {
		t.Fatal(err)
	}
	return db, id
}

func setLevel(t *testing.T, db *DB, id int64, level, exp, breakthrough int) {
	t.Helper()
	if _, err := db.SQL().Exec(`UPDATE characters SET level=?, exp=?, breakthrough=? WHERE id=?`,
		level, exp, breakthrough, id); err != nil {
		t.Fatal(err)
	}
}

func gainExp(t *testing.T, db *DB, id int64, gain int) {
	t.Helper()
	if err := withTx(context.Background(), db.SQL(), func(tx *sql.Tx) error {
		return AddExpTx(context.Background(), tx, id, gain, levelCurve())
	}); err != nil {
		t.Fatalf("AddExpTx: %v", err)
	}
}

func TestAddExpLevelsAndOverflow(t *testing.T) {
	db, id := newLevelChar(t)
	gainExp(t, db, id, 250)
	c, _ := db.Characters.GetCharacter(context.Background(), id)
	if c.Level != 3 || c.Exp != 50 || c.UnspentPoints != 8 {
		t.Fatalf("level/exp/points = %d/%d/%d, 期望 3/50/8", c.Level, c.Exp, c.UnspentPoints)
	}
}

func TestGateStopsOverflow(t *testing.T) {
	db, id := newLevelChar(t)
	setLevel(t, db, id, 29, 500, 0)
	gainExp(t, db, id, 0)
	c, _ := db.Characters.GetCharacter(context.Background(), id)
	if c.Level != 29 || c.Exp != 500 {
		t.Fatalf("关口应停 29 且溢出保留: %d/%d", c.Level, c.Exp)
	}
}

func TestBreakthroughCrossesOneGate(t *testing.T) {
	db, id := newLevelChar(t)
	setLevel(t, db, id, 29, 500, 0)
	c, err := db.Characters.Breakthrough(context.Background(), id, levelCurve())
	if err != nil {
		t.Fatalf("Breakthrough: %v", err)
	}
	if c.Breakthrough != 29 || c.Level != 34 {
		t.Fatalf("突破后 breakthrough/level = %d/%d, 期望 29/34", c.Breakthrough, c.Level)
	}
	if _, err := db.Characters.Breakthrough(context.Background(), id, levelCurve()); !errors.Is(err, ErrBreakthrough) {
		t.Fatalf("非关口应拒绝: %v", err)
	}
}

func TestBreakthroughRequiresFullExp(t *testing.T) {
	db, id := newLevelChar(t)
	setLevel(t, db, id, 29, 50, 0)
	if _, err := db.Characters.Breakthrough(context.Background(), id, levelCurve()); !errors.Is(err, ErrBreakthrough) {
		t.Fatalf("经验未满应拒绝: %v", err)
	}
}

func TestMaxLevelCap(t *testing.T) {
	db, id := newLevelChar(t)
	setLevel(t, db, id, 88, 500, 79)
	gainExp(t, db, id, 0)
	c, _ := db.Characters.GetCharacter(context.Background(), id)
	if c.Level != 89 {
		t.Fatalf("满级应停 89: %d", c.Level)
	}
}

func TestAllocateAndResetPoints(t *testing.T) {
	db, id := newLevelChar(t)
	// 给 8 点未分配（模拟升 2 级）。
	if _, err := db.SQL().Exec(`UPDATE characters SET unspent_points = 8 WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := db.Characters.AllocatePoints(ctx, id, map[string]int{"str": 3, "agi": 1}); err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	c, _ := db.Characters.GetCharacter(ctx, id)
	if c.UnspentPoints != 4 || c.Str != 3 || c.Agi != 1 {
		t.Fatalf("加点后 unspent/str/agi = %d/%d/%d, 期望 4/3/1", c.UnspentPoints, c.Str, c.Agi)
	}
	// 超量拒绝。
	if err := db.Characters.AllocatePoints(ctx, id, map[string]int{"con": 5}); !errors.Is(err, ErrPoints) {
		t.Fatalf("超量应拒绝: %v", err)
	}
	// 负增量拒绝。
	if err := db.Characters.AllocatePoints(ctx, id, map[string]int{"str": -1}); !errors.Is(err, ErrPoints) {
		t.Fatalf("负增量应拒绝: %v", err)
	}
	// 洗点返还。
	if err := db.Characters.ResetPoints(ctx, id); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	c, _ = db.Characters.GetCharacter(ctx, id)
	if c.UnspentPoints != 8 || c.Str != 0 || c.Agi != 0 {
		t.Fatalf("洗点后 unspent/str/agi = %d/%d/%d, 期望 8/0/0", c.UnspentPoints, c.Str, c.Agi)
	}
}