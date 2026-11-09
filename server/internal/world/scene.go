// Package world 为世界域（开发技术规范 §4 world 行）：场景（横版）、位置广播、
// 位置校验防瞬移。场景图与路径/热区逻辑数据分离（逻辑数据来自配置包）。
package world

import (
	"sync"
	"time"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

// Hub 为网关推送契约（gateway.Server 实现之，装配时注入）。
type Hub interface {
	SendTo(token string, main, sub uint16, payload map[string]any)
}

// PlayerState 为场景内一人的连接期状态。
type PlayerState struct {
	CharID int64
	Token  string
	Name   string
	// Faction/Gender 供同屏玩家按门派×性别取行走皮肤（3/1 快照与 3/6 加入推送下发）。
	Faction  string
	Gender   string
	X, Y     float64
	LastMove time.Time
	dirty    bool // Sweep 落盘标记
}

// Scene 为一个分线实例（V1.27：场景按线承载，同图多线各自独立广播）。
type Scene struct {
	Logic    config.SceneLogic
	LineNo   int // 1 起编号
	Capacity int // 每线人数上限（按场景类型，V1.27 阈值）
	mu       sync.RWMutex
	players  map[int64]*PlayerState
}

func newScene(logic config.SceneLogic, lineNo, capacity int) *Scene {
	return &Scene{Logic: logic, LineNo: lineNo, Capacity: capacity, players: make(map[int64]*PlayerState)}
}

// playerCount 当前线人数（锁外读快照口径，调用方自负并发语义）。
func (sc *Scene) playerCount() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return len(sc.players)
}

type Service struct {
	scenes     map[string]config.SceneLogic // 场景逻辑数据（只读）
	lines      map[string][]*Scene          // 场景分线实例（key = map_id）
	muLines    sync.RWMutex                 // 保护 lines 切片结构（线开合），玩家表由各 Scene 自锁
	characters *store.CharactersRepo
	sessions   *session.Manager
	cfg        config.Data
	hub        Hub
	// OnEnter 为进场回调（任务 visit 目标推进用；nil 表示不关心）。
	OnEnter func(charID int64, mapID string)
	// now 可注入时钟（限速校验测试用）；生产为 time.Now。
	now func() time.Time
}

func NewService(characters *store.CharactersRepo, sessions *session.Manager, cfg config.Data, hub Hub) *Service {
	scenes := make(map[string]config.SceneLogic, len(cfg.Scenes))
	lines := make(map[string][]*Scene, len(cfg.Scenes))
	for id, logic := range cfg.Scenes {
		scenes[id] = logic
		lines[id] = nil // 首个进入者触发 1 线创建
	}
	return &Service{
		scenes:     scenes,
		lines:      lines,
		characters: characters,
		sessions:   sessions,
		cfg:        cfg,
		hub:        hub,
		now:        time.Now,
	}
}
