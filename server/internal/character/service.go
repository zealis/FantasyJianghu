// Package character 为角色域（开发技术规范 §4）：创建（账号 ≤3，原资料口径）、
// 选择。门派八选一必选（设计文档 §5.3）。
package character

import (
	"context"
	"errors"
	"fmt"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
	"fantasy-jianghu/server/internal/validate"
)

// 错误码（M1 错误码表）。
const (
	codeOK           = 0
	codeGeneric      = 1
	codeUnauthorized = 2
	codeBadParams    = 3
	codeNotFound     = 5
	codeExists       = 6
	codeOverLimit    = 7
)

type Service struct {
	characters *store.CharactersRepo
	sessions   *session.Manager
	cfg        config.Data // 全量配置：出生点与 ground_y 均取自场景逻辑数据
}

func NewService(characters *store.CharactersRepo, sessions *session.Manager, cfg config.Data) *Service {
	return &Service{characters: characters, sessions: sessions, cfg: cfg}
}

// HandleCreate 处理 WS 2/2 创建角色帧。
func (s *Service) HandleCreate(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil {
		return nil, codeUnauthorized, "未鉴权"
	}
	name, _ := payload["name"].(string)
	faction, _ := payload["faction"].(string)
	gender, _ := payload["gender"].(string)
	if gender != "女" {
		gender = "男" // 缺省男（旧客户端/旧负载兼容，评审 Focus 6）
	}
	if !validate.HanName(name) {
		return nil, codeBadParams, "角色名须为 2–6 个汉字"
	}
	if !s.validFaction(faction) {
		return nil, codeBadParams, fmt.Sprintf("门派须八选一，收到 %q", faction)
	}
	id, err := s.characters.CreateCharacter(ctx, sess.AccountID, name, faction, gender)
	switch {
	case errors.Is(err, store.ErrCharLimit):
		return nil, codeOverLimit, fmt.Sprintf("角色数量已达上限（%d 个）", s.cfg.App.MaxCharactersPerAccount)
	case err != nil:
		return nil, codeExists, "角色名已存在"
	}
	// 出生点来自配置（spawn.map_id/x + 场景 ground_y），落库供进场复用。
	scene := s.cfg.Scenes[s.cfg.App.Spawn.MapID]
	_ = s.characters.UpdatePosition(ctx, id, s.cfg.App.Spawn.MapID, s.cfg.App.Spawn.X, scene.GroundY)
	c, err := s.characters.GetCharacter(ctx, id)
	if err != nil {
		return nil, codeGeneric, "角色读取失败"
	}
	return map[string]any{"char": charDTO(c)}, codeOK, ""
}

// HandleSelect 处理 WS 2/3 选择角色帧：校验归属并绑定会话。
func (s *Service) HandleSelect(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil {
		return nil, codeUnauthorized, "未鉴权"
	}
	idf, ok := payload["char_id"].(float64)
	if !ok || idf <= 0 {
		return nil, codeBadParams, "缺少 char_id"
	}
	c, err := s.characters.GetCharacter(ctx, int64(idf))
	if err != nil {
		return nil, codeNotFound, "角色不存在"
	}
	if c.AccountID != sess.AccountID {
		return nil, codeNotFound, "角色不存在"
	}
	s.sessions.BindCharacter(sess, c.ID)
	if err := s.sessions.Save(ctx, sess); err != nil {
		return nil, codeGeneric, "会话保存失败"
	}
	return map[string]any{"ok": true, "map_id": c.MapID, "x": c.PosX, "y": c.PosY}, codeOK, ""
}

func (s *Service) validFaction(f string) bool {
	for _, want := range s.cfg.App.Factions {
		if want == f {
			return true
		}
	}
	return false
}

func charDTO(c store.Character) map[string]any {
	return map[string]any{
		"id": c.ID, "name": c.Name, "faction": c.Faction, "gender": c.Gender,
		"level": c.Level, "map_id": c.MapID, "x": c.PosX, "y": c.PosY,
	}
}
