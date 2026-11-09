package account

import (
	"context"
	"errors"

	"fantasy-jianghu/server/internal/gateway"
	"fantasy-jianghu/server/internal/session"
)

const (
	codeUnauthorized = 2
)

// HandleWSAuth 处理 WS 2/1 鉴权帧：凭令牌恢复会话并绑定连接（§5.2 断线重连）。
func (s *Service) HandleWSAuth(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	token, _ := payload["token"].(string)
	if token == "" {
		return nil, codeBadParams, "缺少 token"
	}
	sess2, err := s.sessions.Get(ctx, token)
	if errors.Is(err, session.ErrNotFound) || err == errNoSession {
		return nil, codeUnauthorized, "会话不存在或已过期"
	}
	if err != nil {
		return nil, codeGeneric, "会话读取失败"
	}
	if b := gateway.ConnFromCtx(ctx); b != nil {
		b.BindSession(sess2)
	}
	chars, err := s.characters.ListByAccount(ctx, sess2.AccountID)
	if err != nil {
		return nil, codeGeneric, "读取角色失败"
	}
	// 八派明细下发（§9 选择宗门卡片内容唯一来源：GDD §5.3 定案表）。
	factions := make([]map[string]any, 0, len(s.cfg.Factions))
	for _, f := range s.cfg.Factions {
		factions = append(factions, map[string]any{
			"name": f.Name, "category": f.Category, "color": f.Color,
			"residence": f.Residence, "weapon": f.Weapon,
			"feature": f.Feature, "secret": f.Secret,
		})
	}
	return map[string]any{
		"account_id": sess2.AccountID,
		"chars":      charsList(chars),
		"factions":   factions,
	}, codeOK, ""
}
