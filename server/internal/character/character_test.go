package character

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/kv"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

func testCfg() config.Data {
	return config.Data{
		App: config.AppSettings{
			MaxCharactersPerAccount: 3,
			Spawn:                   config.Spawn{MapID: "清河村", X: 400},
			Factions:                []string{"断岳剑宗", "破军枪府", "惊雷殿", "璇玑星宫", "缚灵符观", "织梦谷", "金刚禅院", "玄水宫"},
		},
		Scenes: map[string]config.SceneLogic{
			"清河村": {MapID: "清河村", Width: 2352, Height: 1008, GroundY: 900, Walk: config.WalkRange{XMin: 80, XMax: 2272}},
		},
	}
}

func newService(t *testing.T) (*Service, *store.DB) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	sessions := session.NewManager(kv.NewMemory(), time.Hour)
	return NewService(db.Characters, sessions, testCfg()), db
}

func authedSession(t *testing.T, m *session.Manager, accountID int64) *session.Session {
	t.Helper()
	token, err := m.Create(context.Background(), accountID)
	if err != nil {
		t.Fatal(err)
	}
	s, err := m.Get(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCreateOK(t *testing.T) {
	svc, db := newService(t)
	ctx := context.Background()
	aid, _ := db.Accounts.CreateAccount(ctx, "创角号", "h", "")
	sess := authedSession(t, svc.sessions, aid)

	p, code, msg := svc.HandleCreate(ctx, sess, map[string]any{"name": "初入江湖", "faction": "惊雷殿"})
	if code != 0 {
		t.Fatalf("创角 = code %d (%s)", code, msg)
	}
	ch := p["char"].(map[string]any)
	if ch["name"] != "初入江湖" || ch["faction"] != "惊雷殿" || ch["level"] != 1 {
		t.Fatalf("创角负载不一致: %+v", ch)
	}
	if ch["map_id"] != "清河村" || ch["x"] != 400.0 {
		t.Fatalf("初始出生点不一致: %+v", ch)
	}
}

func TestCreateValidation(t *testing.T) {
	svc, db := newService(t)
	ctx := context.Background()
	aid, _ := db.Accounts.CreateAccount(ctx, "校验号", "h", "")
	sess := authedSession(t, svc.sessions, aid)

	if _, code, _ := svc.HandleCreate(ctx, sess, map[string]any{"name": "无名氏", "faction": "不存在的派"}); code != 3 {
		t.Fatalf("非法门派 code = %d, 期望 3", code)
	}
	if _, code, _ := svc.HandleCreate(ctx, sess, map[string]any{"name": "甲", "faction": "惊雷殿"}); code != 3 {
		t.Fatalf("名字过短 code = %d, 期望 3", code)
	}
	if _, code, _ := svc.HandleCreate(ctx, sess, map[string]any{"faction": "惊雷殿"}); code != 3 {
		t.Fatalf("缺名字 code = %d, 期望 3", code)
	}
}

func TestCreateDupName(t *testing.T) {
	svc, db := newService(t)
	ctx := context.Background()
	aid, _ := db.Accounts.CreateAccount(ctx, "重名号", "h", "")
	sess := authedSession(t, svc.sessions, aid)
	if _, code, _ := svc.HandleCreate(ctx, sess, map[string]any{"name": "同名侠", "faction": "玄水宫"}); code != 0 {
		t.Fatal("首次创角应成功")
	}
	if _, code, _ := svc.HandleCreate(ctx, sess, map[string]any{"name": "同名侠", "faction": "玄水宫"}); code != 6 {
		t.Fatalf("重名 code = %d, 期望 6", code)
	}
}

func TestCreateLimitConcurrent(t *testing.T) {
	svc, db := newService(t)
	ctx := context.Background()
	aid, _ := db.Accounts.CreateAccount(ctx, "并发号", "h", "")
	sess := authedSession(t, svc.sessions, aid)

	// 预建 2 个，随后 10 个 goroutine 并发抢最后一个名额：
	// 事务内计数必须恰好放行 1 个，其余全部超限拒绝。
	for i := 0; i < 2; i++ {
		name := "并发侠甲乙"[:3] + string(rune('一'+i))
		if _, code, _ := svc.HandleCreate(ctx, sess, map[string]any{"name": name, "faction": "金刚禅院"}); code != 0 {
			t.Fatalf("预建角色 %d 失败: code %d", i+1, code)
		}
	}
	var ok, limited int64
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := authedSession(t, svc.sessions, aid)
			_, code, _ := svc.HandleCreate(context.Background(), s, map[string]any{"name": "并发侠肆", "faction": "金刚禅院"})
			switch code {
			case 0:
				atomic.AddInt64(&ok, 1)
			case 7:
				atomic.AddInt64(&limited, 1)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || limited != 9 {
		t.Fatalf("并发抢最后名额: 成功 %d / 超限 %d, 期望 1/9", ok, limited)
	}
}

func TestSelect(t *testing.T) {
	svc, db := newService(t)
	ctx := context.Background()
	aidA, _ := db.Accounts.CreateAccount(ctx, "甲号", "h", "")
	aidB, _ := db.Accounts.CreateAccount(ctx, "乙号", "h", "")
	sessA := authedSession(t, svc.sessions, aidA)
	sessB := authedSession(t, svc.sessions, aidB)
	p, code, _ := svc.HandleCreate(ctx, sessA, map[string]any{"name": "选角侠", "faction": "破军枪府"})
	if code != 0 {
		t.Fatal("预置角色失败")
	}
	charID := p["char"].(map[string]any)["id"].(int64)

	// 他人角色不可选。
	if _, code, _ := svc.HandleSelect(ctx, sessB, map[string]any{"char_id": float64(charID)}); code != 5 {
		t.Fatalf("选他人角色 code = %d, 期望 5", code)
	}
	// 本人可选并绑定。
	p, code, msg := svc.HandleSelect(ctx, sessA, map[string]any{"char_id": float64(charID)})
	if code != 0 || p["ok"] != true || p["map_id"] != "清河村" {
		t.Fatalf("选角 = (%+v, code %d, %s)", p, code, msg)
	}
	if sessA.CharID != charID {
		t.Fatalf("选角后 CharID = %d, 期望 %d", sessA.CharID, charID)
	}
	// 不存在角色。
	if _, code, _ := svc.HandleSelect(ctx, sessA, map[string]any{"char_id": float64(99999)}); code != 5 {
		t.Fatalf("选不存在角色 code = %d, 期望 5", code)
	}
}

func TestCreateGenderAndHanName(t *testing.T) {
	svc, db := newService(t)
	ctx := context.Background()
	aid, _ := db.Accounts.CreateAccount(ctx, "性别号", "h", "")
	sess := authedSession(t, svc.sessions, aid)

	// 显式 gender 往返。
	p, code, _ := svc.HandleCreate(ctx, sess, map[string]any{"name": "云溪", "faction": "惊雷殿", "gender": "女"})
	if code != 0 {
		t.Fatalf("创角 code=%d", code)
	}
	if p["char"].(map[string]any)["gender"] != "女" {
		t.Fatalf("char.gender = %v, 期望 女", p["char"].(map[string]any)["gender"])
	}
	// 缺 gender 默认男（评审 Focus 6：旧客户端兼容）。
	p, code, _ = svc.HandleCreate(ctx, sess, map[string]any{"name": "默认男", "faction": "惊雷殿"})
	if code != 0 || p["char"].(map[string]any)["gender"] != "男" {
		t.Fatalf("缺 gender 应默认男: %+v code=%d", p, code)
	}
	// 角色名收紧为 2–6 汉字（§9）：英文名拒、七字拒、二字可。
	if _, code, _ := svc.HandleCreate(ctx, sess, map[string]any{"name": "abcdef", "faction": "惊雷殿"}); code != 3 {
		t.Fatalf("英文名应拒 code=3, 收 %d", code)
	}
	if _, code, _ := svc.HandleCreate(ctx, sess, map[string]any{"name": "七字汉字的角色名", "faction": "惊雷殿"}); code != 3 {
		t.Fatalf("七字名应拒 code=3, 收 %d", code)
	}
	if _, code, _ := svc.HandleCreate(ctx, sess, map[string]any{"name": "二字", "faction": "惊雷殿"}); code != 0 {
		t.Fatal("二字名应可创")
	}
}
