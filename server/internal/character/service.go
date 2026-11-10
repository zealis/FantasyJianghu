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

// HandleBreakthrough 处理 WS 2/4 手动突破关口（GDD §4.2，M2c 切片三）。
func (s *Service) HandleBreakthrough(ctx context.Context, sess *session.Session, _ map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	curve := store.LevelCurve{ExpToNext: s.cfg.Levels.ExpToNext, IsGate: s.cfg.Levels.IsGate,
		MaxLevel: s.cfg.Levels.MaxLevel, PointsPerLevel: s.cfg.Levels.PointsPerLevel}
	c, err := s.characters.Breakthrough(ctx, sess.CharID, curve)
	if err != nil {
		if errors.Is(err, store.ErrBreakthrough) {
			return nil, codeBadParams, "当前不可突破：需恰处关口且经验已满"
		}
		return nil, codeGeneric, "突破失败"
	}
	return map[string]any{"ok": true, "level": c.Level, "exp": c.Exp,
		"exp_next": s.cfg.Levels.ExpToNext(c.Level)}, codeOK, ""
}

// HandleAllocate 处理 WS 2/5 加点：{str,spi,agi,con} 为本次预加增量（M2c 切片四）。
func (s *Service) HandleAllocate(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	delta := map[string]int{}
	for _, k := range []string{"str", "spi", "agi", "con"} {
		v, _ := payload[k].(float64)
		delta[k] = int(v)
	}
	if err := s.characters.AllocatePoints(ctx, sess.CharID, delta); err != nil {
		if errors.Is(err, store.ErrPoints) {
			return nil, codeBadParams, "属性点不足或增量非法"
		}
		return nil, codeGeneric, "加点失败"
	}
	c, err := s.characters.GetCharacter(ctx, sess.CharID)
	if err != nil {
		return nil, codeGeneric, "读取角色失败"
	}
	return map[string]any{"ok": true, "unspent": c.UnspentPoints, "points": c.PointsMap()}, codeOK, ""
}

// HandleResetPoints 处理 WS 2/6 洗点：返还全部已分配点（本轮免费）。
func (s *Service) HandleResetPoints(ctx context.Context, sess *session.Session, _ map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	if err := s.characters.ResetPoints(ctx, sess.CharID); err != nil {
		return nil, codeGeneric, "洗点失败"
	}
	c, err := s.characters.GetCharacter(ctx, sess.CharID)
	if err != nil {
		return nil, codeGeneric, "读取角色失败"
	}
	return map[string]any{"ok": true, "unspent": c.UnspentPoints, "points": c.PointsMap()}, codeOK, ""
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
	spawnX := s.cfg.App.Spawn.X
	spawnY := s.cfg.App.Spawn.Y
	if spawnY <= 0 {
		spawnY = scene.GroundY
	}
	_ = s.characters.UpdatePosition(ctx, id, s.cfg.App.Spawn.MapID, spawnX, spawnY)
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
