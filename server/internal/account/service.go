// Package account 为账号域（开发技术规范 §4）：注册、登录、令牌、封禁。
// 密码 bcrypt 加盐哈希落库；登录限流（10 分钟内 5 次失败锁定 10 分钟）。
package account

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"strings"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/kv"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

// 错误码（§gateway M1 错误码表）。
const (
	codeOK          = 0
	codeGeneric     = 1
	codeBadParams   = 3
	codeRateLimited = 4
	codeExists      = 6
)

const (
	usernameMin, usernameMax = 2, 16
	passwordMin              = 6
	bcryptCost               = 10
	loginFailLimit           = 5
	loginLockTTL             = 10 * time.Minute
)

type Service struct {
	accounts   *store.AccountsRepo
	characters *store.CharactersRepo
	sessions   *session.Manager
	store      kv.Store
	cfg        config.Data // 八派明细经 2/1 下发（宗门卡片客户端唯一来源）
}

func NewService(accounts *store.AccountsRepo, characters *store.CharactersRepo, sessions *session.Manager, st kv.Store, cfg config.Data) *Service {
	return &Service{accounts: accounts, characters: characters, sessions: sessions, store: st, cfg: cfg}
}

func validRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
		r == '_' || unicode.Is(unicode.Han, r)
}

func validName(name string, min, max int) bool {
	n := 0
	for _, r := range name {
		if !validRune(r) {
			return false
		}
		n++
	}
	return n >= min && n <= max
}

func (s *Service) Register(ctx context.Context, username, password, nickname string) (int64, int, string) {
	if !validName(username, usernameMin, usernameMax) {
		return 0, codeBadParams, "用户名须为 2–16 位中文/字母/数字/下划线"
	}
	if len(password) < passwordMin || len(password) > 64 {
		return 0, codeBadParams, "密码至少 6 位"
	}
	// 昵称（§9 注册界面）：宽进——裁剪空白、截断 16 rune，无唯一性要求。
	nickname = strings.TrimSpace(nickname)
	if rn := []rune(nickname); len(rn) > 16 {
		nickname = string(rn[:16])
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return 0, codeGeneric, "内部错误"
	}
	id, err := s.accounts.CreateAccount(ctx, username, string(hash), nickname)
	if err != nil {
		return 0, codeExists, "用户名已存在"
	}
	return id, codeOK, ""
}

func (s *Service) Login(ctx context.Context, username, password string) (string, []store.Character, int, string) {
	// 登录限流：锁定期直接拒绝（§4 account：登录限流）。
	if n, err := s.store.Get(ctx, rateKey(username)); err == nil && countOf(n) >= loginFailLimit {
		return "", nil, codeRateLimited, "登录失败次数过多，请 10 分钟后再试"
	}
	a, err := s.accounts.GetAccountByUsername(ctx, username)
	if err != nil {
		return "", nil, codeGeneric, "用户名或密码错误"
	}
	if a.BanUntil.After(time.Now()) {
		return "", nil, codeGeneric, fmt.Sprintf("账号封禁至 %s：%s", a.BanUntil.Format("2006-01-02 15:04"), a.BanReason)
	}
	if bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil {
		s.store.Incr(ctx, rateKey(username), loginLockTTL) // 失败计数（滑动 10 分钟窗）
		return "", nil, codeGeneric, "用户名或密码错误"
	}
	token, err := s.sessions.Create(ctx, a.ID)
	if err != nil {
		return "", nil, codeGeneric, "会话创建失败"
	}
	chars, err := s.characters.ListByAccount(ctx, a.ID)
	if err != nil {
		return "", nil, codeGeneric, "读取角色失败"
	}
	return token, chars, codeOK, ""
}

func rateKey(username string) string { return "rl:login:" + username }

func countOf(raw []byte) int {
	n := 0
	for _, d := range raw {
		n = n*10 + int(d-'0')
	}
	return n
}

// charsList 把角色实体转为回包负载（§6.3：JSON 扁平字段，无浮点敏感数据）。
func charsList(chars []store.Character) []map[string]any {
	out := make([]map[string]any, 0, len(chars))
	for _, c := range chars {
		out = append(out, map[string]any{
			"id": c.ID, "name": c.Name, "faction": c.Faction, "gender": c.Gender,
			"level": c.Level, "map_id": c.MapID, "x": c.PosX, "y": c.PosY,
		})
	}
	return out
}

var errNoSession = errors.New("session expired")
