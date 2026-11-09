// Package npc 为 NPC 交互域（开发技术规范 §4 / 主码 10）。
// M2b 只做对话记账：校验 NPC 属于该场景，并把 talk 类任务目标推进一格。
// 对话文本仍由客户端场景逻辑数据渲染（M2a 口径），服务端不做文本权威。
package npc

import (
	"context"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/quest"
	"fantasy-jianghu/server/internal/session"
)

const (
	codeOK           = 0
	codeGeneric      = 1
	codeUnauthorized = 2
	codeBadParams    = 3
	codeNotFound     = 5
)

type Service struct {
	cfg    config.Data
	quests *quest.Service
}

func NewService(cfg config.Data, quests *quest.Service) *Service {
	return &Service{cfg: cfg, quests: quests}
}

// HandleTalk 处理 10/1 NPC 对话：校验归属并推进 talk 进度。
func (s *Service) HandleTalk(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	mapID, _ := payload["map_id"].(string)
	npcID, _ := payload["npc_id"].(string)
	scene, ok := s.cfg.Scenes[mapID]
	if !ok {
		return nil, codeNotFound, "场景不存在"
	}
	name := ""
	for _, n := range scene.NPCs {
		if n.ID == npcID {
			name = n.Name
			break
		}
	}
	if name == "" {
		return nil, codeBadParams, "NPC 不在该场景"
	}
	if err := s.quests.RecordProgress(ctx, sess.CharID, "talk", npcID, 1); err != nil {
		return nil, codeGeneric, "任务进度记录失败"
	}
	return map[string]any{"ok": true, "npc": map[string]any{"id": npcID, "name": name}}, codeOK, ""
}
