package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestMigrateIdempotent(t *testing.T) {
	db := openTestDB(t)
	if err := db.Migrate(); err != nil {
		t.Fatalf("重复 Migrate 应幂等: %v", err)
	}
}

func TestAccounts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	id, err := db.Accounts.CreateAccount(ctx, "侠客一", "bcrypt-hash-1", "")
	if err != nil || id <= 0 {
		t.Fatalf("CreateAccount = (%d, %v)", id, err)
	}
	a, err := db.Accounts.GetAccountByUsername(ctx, "侠客一")
	if err != nil {
		t.Fatalf("GetAccountByUsername: %v", err)
	}
	if a.ID != id || a.PasswordHash != "bcrypt-hash-1" {
		t.Fatalf("账号往返不一致: %+v", a)
	}

	if _, err := db.Accounts.CreateAccount(ctx, "侠客一", "h2", ""); err == nil {
		t.Fatal("重复用户名应报唯一冲突")
	}

	if _, err := db.Accounts.GetAccountByUsername(ctx, "不存在"); err == nil {
		t.Fatal("不存在用户名应报错")
	}

	until := time.Now().Add(30 * time.Minute)
	if err := db.Accounts.SetBan(ctx, id, "外挂", until); err != nil {
		t.Fatalf("SetBan: %v", err)
	}
	a2, _ := db.Accounts.GetAccountByUsername(ctx, "侠客一")
	if a2.BanReason != "外挂" || !a2.BanUntil.Equal(until.Truncate(time.Second)) {
		t.Fatalf("封禁读取不一致: reason=%q until=%v 期望 until≈%v", a2.BanReason, a2.BanUntil, until)
	}
}

func TestCharactersLimitThree(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	aid, _ := db.Accounts.CreateAccount(ctx, "角色号", "h", "")

	var lastID int64
	for i := 0; i < 3; i++ {
		cid, err := db.Characters.CreateCharacter(ctx, aid, "侠"+string(rune('甲'+i)), "惊雷殿", "男")
		if err != nil {
			t.Fatalf("第 %d 个角色创建失败: %v", i+1, err)
		}
		lastID = cid
	}
	if _, err := db.Characters.CreateCharacter(ctx, aid, "第四人", "惊雷殿", "男"); !errors.Is(err, ErrCharLimit) {
		t.Fatalf("第 4 个角色 = %v, 期望 ErrCharLimit", err)
	}

	// 软删后 List 不含，Get 报不存在。
	if err := db.Characters.SoftDelete(ctx, lastID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	list, err := db.Characters.ListByAccount(ctx, aid)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("软删后列表数 = %d, 期望 2", len(list))
	}
	if _, err := db.Characters.GetCharacter(ctx, lastID); err == nil {
		t.Fatal("软删角色 Get 应报错")
	}

	// 位置更新读回。
	c, _ := db.Characters.GetCharacter(ctx, list[0].ID)
	if err := db.Characters.UpdatePosition(ctx, c.ID, "清河村", 1234.5, 900); err != nil {
		t.Fatalf("UpdatePosition: %v", err)
	}
	c2, _ := db.Characters.GetCharacter(ctx, c.ID)
	if c2.MapID != "清河村" || c2.PosX != 1234.5 || c2.PosY != 900 {
		t.Fatalf("位置读回不一致: %+v", c2)
	}
}

func TestCurrencyLogs(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	aid, _ := db.Accounts.CreateAccount(ctx, "流水号", "h", "")
	cid, _ := db.Characters.CreateCharacter(ctx, aid, "流水侠", "玄水宫", "男")

	for i, d := range []int64{100, -30, 5} {
		if err := db.Currency.AppendLog(ctx, cid, "元宝", d, 100+int64(i)*5, "测试", "ref-1"); err != nil {
			t.Fatalf("AppendLog[%d]: %v", i, err)
		}
	}
	logs, err := db.Currency.ListRecent(ctx, cid, 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("流水数 = %d, 期望 3", len(logs))
	}
	if logs[0].Delta != 5 || logs[2].Delta != 100 {
		t.Fatalf("整数金额读回不一致（ListRecent 最新在前）: %+v", logs)
	}
}

func TestMigrateV2DefaultsAndFields(t *testing.T) {
	ctx := context.Background()
	// 模拟 v1 旧库：先只应用 v1 建表，插入"旧数据"，再升 v2 验默认值。
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	// v1 = 前三条建表迁移（v2 为 ALTER，见 migrations 尾部）。
	for _, m := range migrations[:3] {
		if _, err := db.SQL().Exec(m); err != nil {
			t.Fatalf("手工建 v1: %v", err)
		}
	}
	if _, err := db.SQL().Exec(`INSERT INTO accounts (id, username, password_hash, created_at) VALUES (1, '旧号', 'h', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`INSERT INTO characters (id, account_id, name, faction, created_at) VALUES (1, 1, '旧侠', '惊雷殿', 0)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("升 v2: %v", err)
	}
	a, err := db.Accounts.GetAccountByUsername(ctx, "旧号")
	if err != nil || a.Nickname != "" {
		t.Fatalf("旧账号 nickname = (%q, %v), 期望空", a.Nickname, err)
	}
	c, err := db.Characters.GetCharacter(ctx, 1)
	if err != nil || c.Gender != "男" {
		t.Fatalf("旧角色 gender = (%q, %v), 期望 男", c.Gender, err)
	}

	// 新写入：gender / nickname 往返。
	aid, _ := db.Accounts.CreateAccount(ctx, "新号", "h2", "云溪阁下")
	cid, err := db.Characters.CreateCharacter(ctx, aid, "新侠客", "玄水宫", "女")
	if err != nil {
		t.Fatalf("创角: %v", err)
	}
	a2, _ := db.Accounts.GetAccountByUsername(ctx, "新号")
	if a2.Nickname != "云溪阁下" {
		t.Fatalf("nickname 读回 = %q", a2.Nickname)
	}
	c2, _ := db.Characters.GetCharacter(ctx, cid)
	if c2.Gender != "女" {
		t.Fatalf("gender 读回 = %q, 期望 女", c2.Gender)
	}
}
