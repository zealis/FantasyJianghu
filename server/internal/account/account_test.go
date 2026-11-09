package account

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/kv"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

func newService(t *testing.T) (*Service, *store.DB, *session.Manager) {
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
	return NewService(db.Accounts, db.Characters, sessions, kv.NewMemory(), minimalCfg()), db, sessions
}

// minimalCfg 为账号测试的最小配置（八派明细，2/1 下发用）。
func minimalCfg() config.Data {
	return config.Data{Factions: []config.FactionInfo{
		{Name: "断岳剑宗", Category: "物理", Color: "朱橙", Residence: "高天原·断剑崖", Weapon: "剑", Feature: "f", Secret: "s"},
		{Name: "破军枪府", Category: "物理", Color: "朱橙", Residence: "葬剑原", Weapon: "枪", Feature: "f", Secret: "s"},
		{Name: "惊雷殿", Category: "法术", Color: "紫", Residence: "北天庭", Weapon: "杖", Feature: "f", Secret: "s"},
		{Name: "璇玑星宫", Category: "法术", Color: "紫", Residence: "高天原", Weapon: "杖", Feature: "f", Secret: "s"},
		{Name: "缚灵符观", Category: "封印", Color: "青", Residence: "坠魔谷", Weapon: "爪", Feature: "f", Secret: "s"},
		{Name: "织梦谷", Category: "封印", Color: "青", Residence: "临渊镇", Weapon: "爪", Feature: "f", Secret: "s"},
		{Name: "金刚禅院", Category: "肉盾", Color: "绿", Residence: "望月城", Weapon: "杖", Feature: "f", Secret: "s"},
		{Name: "玄水宫", Category: "肉盾", Color: "绿", Residence: "蓬莱岛", Weapon: "杖", Feature: "f", Secret: "s"},
	}}
}

func TestRegisterLoginRoundtrip(t *testing.T) {
	svc, _, sessions := newService(t)
	ctx := context.Background()

	id, code, msg := svc.Register(ctx, "侠客一", "123456", "")
	if code != 0 || id <= 0 || msg != "" {
		t.Fatalf("Register = (%d, %d, %q)", id, code, msg)
	}
	token, chars, code, msg := svc.Login(ctx, "侠客一", "123456")
	if code != 0 || token == "" || len(chars) != 0 {
		t.Fatalf("Login = (token len %d, chars %d, code %d, %q)", len(token), len(chars), code, msg)
	}
	if sess, err := sessions.Get(ctx, token); err != nil || sess.AccountID != id {
		t.Fatalf("令牌未入会话: %v %+v", err, sess)
	}
}

func TestRegisterValidation(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	cases := []struct {
		name, user, pass string
		wantCode         int
	}{
		{"用户名过短", "甲", "123456", 3},
		{"用户名过长", strings.Repeat("甲", 17), "123456", 3},
		{"用户名非法字符", "bad name!", "123456", 3},
		{"密码过短", "侠客一", "12345", 3},
	}
	for _, c := range cases {
		if _, code, _ := svc.Register(ctx, c.user, c.pass, ""); code != c.wantCode {
			t.Errorf("%s: code = %d, 期望 %d", c.name, code, c.wantCode)
		}
	}
}

func TestDuplicateUsername(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	svc.Register(ctx, "独孤求败", "123456", "")
	if _, code, _ := svc.Register(ctx, "独孤求败", "654321", ""); code != 6 {
		t.Fatalf("重复用户名 code = %d, 期望 6", code)
	}
}

func TestPasswordHashedInDB(t *testing.T) {
	svc, db, _ := newService(t)
	ctx := context.Background()
	svc.Register(ctx, "铁掌帮", "s3cretPW", "")
	a, err := db.Accounts.GetAccountByUsername(ctx, "铁掌帮")
	if err != nil {
		t.Fatal(err)
	}
	if a.PasswordHash == "s3cretPW" || !strings.HasPrefix(a.PasswordHash, "$2") {
		t.Fatalf("密码未以 bcrypt 哈希落库: %q", a.PasswordHash)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte("s3cretPW")); err != nil {
		t.Fatalf("bcrypt 校验失败: %v", err)
	}
}

func TestLoginRateLimit(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	svc.Register(ctx, "限流侠", "123456", "")
	// 5 次错密码 → 之后正确密码也被锁。
	for i := 0; i < 5; i++ {
		if _, _, code, _ := svc.Login(ctx, "限流侠", "wrong!"); code != 1 {
			t.Fatalf("第 %d 次错密码 code = %d, 期望 1", i+1, code)
		}
	}
	if _, _, code, msg := svc.Login(ctx, "限流侠", "123456"); code != 4 {
		t.Fatalf("锁定期正确密码 code = %d (%s), 期望 4", code, msg)
	}
}

func TestBannedAccount(t *testing.T) {
	svc, db, _ := newService(t)
	ctx := context.Background()
	id, _, _ := svc.Register(ctx, "封禁侠", "123456", "")
	until := time.Now().Add(time.Hour)
	if err := db.Accounts.SetBan(ctx, id, "外挂行为", until); err != nil {
		t.Fatal(err)
	}
	_, _, code, msg := svc.Login(ctx, "封禁侠", "123456")
	if code != 1 || !strings.Contains(msg, "外挂行为") {
		t.Fatalf("封禁登录 = (code %d, msg %q), 期望 code 1 含原因", code, msg)
	}
}

func TestHTTPHandlers(t *testing.T) {
	svc, _, _ := newService(t)
	mux := http.NewServeMux()
	svc.MountHTTP(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	post := func(path, body string) (int, map[string]any) {
		resp, err := http.Post(ts.URL+path, "application/json", bytes.NewReader([]byte(body)))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, out
	}

	st, out := post("/api/register", `{"username":"http侠","password":"123456"}`)
	if st != 200 || out["code"] != float64(0) {
		t.Fatalf("注册响应 = (%d, %+v)", st, out)
	}
	st, out = post("/api/login", `{"username":"http侠","password":"123456"}`)
	if st != 200 || out["code"] != float64(0) {
		t.Fatalf("登录响应 = (%d, %+v)", st, out)
	}
	data := out["data"].(map[string]any)
	if data["token"] == "" {
		t.Fatal("登录响应缺 token")
	}
	st, out = post("/api/login", `{"username":"http侠","password":"bad"}`)
	if out["code"] == float64(0) {
		t.Fatal("错密码不应成功")
	}
}

func TestWSAuth(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	user := mustRegister(t, svc, "ws侠", "123456")
	token, _, _, _ := svc.Login(ctx, user, "123456")

	conn := newFakeConn()
	cctx := withConn(ctx, conn)
	p, code, _ := svc.HandleWSAuth(cctx, nil, map[string]any{"token": token})
	if code != 0 || p["account_id"] == nil || p["chars"] == nil {
		t.Fatalf("WS 鉴权 = (%+v, %d)", p, code)
	}
	if conn.bound == nil || conn.bound.Token != token {
		t.Fatal("WS 鉴权未绑定会话")
	}

	if _, code, _ := svc.HandleWSAuth(cctx, nil, map[string]any{"token": "bad-token"}); code != 2 {
		t.Fatalf("坏令牌 code = %d, 期望 2", code)
	}
}

func TestRegisterNickname(t *testing.T) {
	svc, db, _ := newService(t)
	ctx := context.Background()
	if _, code, msg := svc.Register(ctx, "昵称侠", "123456", "云溪阁下"); code != 0 {
		t.Fatalf("带昵称注册失败: %d %s", code, msg)
	}
	a, err := db.Accounts.GetAccountByUsername(ctx, "昵称侠")
	if err != nil || a.Nickname != "云溪阁下" {
		t.Fatalf("nickname 落库 = (%q, %v), 期望 云溪阁下", a.Nickname, err)
	}
	// 昵称可空（兼容旧客户端）。
	if _, code, _ := svc.Register(ctx, "无昵号", "123456", ""); code != 0 {
		t.Fatal("无昵称注册应成功")
	}
	// 超长昵称截断不报错（§9 未定唯一性，服务端宽进）。
	if _, code, _ := svc.Register(ctx, "长昵号", "123456", "这个昵称实在是长得离谱超过十六位了呀"); code != 0 {
		t.Fatal("长昵称应宽容处理（截断）不报错")
	}
}

func TestWSAuthCharsIncludeGender(t *testing.T) {
	svc, db, _ := newService(t)
	ctx := context.Background()
	user := mustRegister(t, svc, "性别侠", "123456")
	aid, _ := svc2AccountID(t, db, user)
	cid, _ := db.Characters.CreateCharacter(ctx, aid, "性别角色", "惊雷殿", "女")
	_ = cid
	token, _, _, _ := svc.Login(ctx, user, "123456")
	conn := newFakeConn()
	p, code, _ := svc.HandleWSAuth(withConn(ctx, conn), nil, map[string]any{"token": token})
	if code != 0 {
		t.Fatalf("鉴权 code=%d", code)
	}
	chars := p["chars"].([]map[string]any)
	if len(chars) != 1 || chars[0]["gender"] != "女" {
		t.Fatalf("chars 回包缺 gender: %+v", chars)
	}
}

func svc2AccountID(t *testing.T, db *store.DB, username string) (int64, string) {
	t.Helper()
	a, err := db.Accounts.GetAccountByUsername(context.Background(), username)
	if err != nil {
		t.Fatal(err)
	}
	return a.ID, username
}

func TestWSAuthCarriesFactions(t *testing.T) {
	svc, db, _ := newService(t)
	ctx := context.Background()
	user := mustRegister(t, svc, "宗门侠", "123456")
	aid, _ := svc2AccountID(t, db, user)
	_ = aid
	token, _, _, _ := svc.Login(ctx, user, "123456")
	p, code, _ := svc.HandleWSAuth(ctx, nil, map[string]any{"token": token})
	if code != 0 {
		t.Fatalf("鉴权 code=%d", code)
	}
	factions := p["factions"].([]map[string]any)
	if len(factions) != 8 {
		t.Fatalf("factions = %d 项, 期望 8", len(factions))
	}
	f0 := factions[0]
	if f0["name"] != "断岳剑宗" || f0["category"] != "物理" || f0["secret"] == "" {
		t.Fatalf("八派明细不一致: %+v", f0)
	}
}
