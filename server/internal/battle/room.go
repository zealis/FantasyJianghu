package battle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/store"
)

// fakeClock 供 battle 包测试复用（与 world 包同构）。
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// FrameDelivery 为一次结算的投递描述。
//
// 关键约定：投递信息在 Room.mu 内一次性快照生成，hook 必须在 **锁外** 调用。
// 旧实现里 hook 回调反查 Manager.Room(roomID).Participants()，而 Participants()
// 会再抢同一把非重入的 Room.mu —— 生产装配第一帧即自死锁（测试用假 hook 不反查，故测不出）。
// 现在 hook 直接拿到 Tokens/Result，不再需要反查 Room，锁顺序问题被结构性消除。
type FrameDelivery struct {
	RoomID      string
	TurnVersion int
	Events      []Event
	Tokens      []string
	Result      map[string]any // 非 nil 表示终局，装配层另发 4/5
	Outcome     *Outcome       // 非 nil 表示终局归属（任务进度等外部系统消费）
}

// Outcome 为终局归属快照：谁打的、赢了没、打的是哪些怪。
// 在锁内生成副本，外部系统据此推进任务，无需反查 Room（沿用 M2a 的锁序原则）。
type Outcome struct {
	RoomID     string
	Winner     int
	CharIDs    []int64  // 玩家侧角色 id（托管队友不计）
	MonsterIDs []string // 本场野怪 id（PVE 演练按阵容顺序）
	// Killed 为本场实际阵亡的野怪 id（任务 kill 目标按"目标阵亡"记账，
	// 不与玩家存活挂钩：切磋/演练场同归于尽也应推进任务）。
	Killed []string
}

// Room 为一场进行中的战斗（帧模型四段：指令→结算→帧下发→确认，§8.1）。
type Room struct {
	ID           string
	B            *Battle
	participants map[string]int64 // token → unitID
	acked        map[string]int   // token → 已确认回合
	ackedEvents  []Event          // 已广播事件（累计）
	lastCmdTime  time.Time
	finishedAt   time.Time // 终局时刻（供延迟回收，留出客户端确认窗口）
	logs         *store.BattleLogsRepo
	now          func() time.Time
	onFrameHook  func(FrameDelivery)
	cmdStream    []string
	outcome      Outcome          // 终局归属元数据（Create 后由装配层填充）
	monsterUnits map[int64]string // 战斗单位 id → 野怪配置 id
	mu           sync.Mutex
	closed       bool
}

// SetOutcomeMeta 登记终局归属（开局时调用一次，之后只读）。
// monsters 为"战斗单位 id → 野怪配置 id"映射，用于识别本场哪些野怪阵亡。
func (r *Room) SetOutcomeMeta(charIDs []int64, monsters map[int64]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outcome.CharIDs = append([]int64(nil), charIDs...)
	r.monsterUnits = make(map[int64]string, len(monsters))
	seen := map[string]bool{}
	for uid, mid := range monsters {
		r.monsterUnits[uid] = mid
	}
	for _, mid := range r.monsterUnits {
		if !seen[mid] {
			seen[mid] = true
			r.outcome.MonsterIDs = append(r.outcome.MonsterIDs, mid)
		}
	}
}

// OnCmd 收指令；收齐全员（存活方各一）即结算广播帧。
func (r *Room) OnCmd(token string, cmd Cmd) {
	r.mu.Lock()
	if r.closed || r.B.Over {
		r.mu.Unlock()
		return
	}
	unitID, ok := r.participants[token]
	if !ok {
		r.mu.Unlock()
		return
	}
	r.B.SubmitCmd(unitID, cmd)
	r.cmdStream = append(r.cmdStream, fmt.Sprintf("t%d:%d:%s:%s:%d", r.B.Turn+1, unitID, cmd.Type, cmd.SkillID, cmd.TargetID))
	r.lastCmdTime = r.now()
	// PVE 最小 AI：托管侧令牌（monster: / ally: 前缀）由服务端立即出招。
	for tok, id := range r.participants {
		if !isHostedToken(tok) {
			continue
		}
		if u := r.B.unit(id); u != nil && u.Alive {
			if _, has := r.B.cmds[id]; !has {
				u.Hosted = true
				r.B.SubmitCmd(id, Cmd{Type: "attack"})
			}
		}
	}
	var d FrameDelivery
	if r.allSubmitted() {
		d = r.resolve()
	}
	r.mu.Unlock()
	r.deliver(d)
}

// OnAck 帧确认；返回当前回合版本与该令牌尚未确认的事件（供装配层补发）。
// 旧确认号不得回退，否则会重复补发历史帧。
func (r *Room) OnAck(token string, turnVersion int) (int, []Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if turnVersion > r.acked[token] {
		r.acked[token] = turnVersion
	}
	cur := r.B.Turn
	if turnVersion >= cur {
		return cur, nil
	}
	return cur, r.pendingForLocked(token)
}

// pendingFor 取该 token 尚未确认的事件（补发队列）。
func (r *Room) pendingFor(token string) []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pendingForLocked(token)
}

// pendingForLocked 调用方必须持 r.mu。
func (r *Room) pendingForLocked(token string) []Event {
	acked := r.acked[token]
	if len(r.ackedEvents) == 0 {
		return nil
	}
	var out []Event
	for _, e := range r.ackedEvents {
		if e.Turn > acked {
			out = append(out, e)
		}
	}
	return out
}

func (r *Room) allSubmitted() bool {
	for _, unitID := range r.participants {
		if u := r.B.unit(unitID); u != nil && u.Alive {
			if _, has := r.B.cmds[unitID]; !has {
				return false
			}
		}
	}
	return true
}

// resolve 在 Room.mu 内完成结算与状态变更，只返回投递描述；不在此调用任何回调。
func (r *Room) resolve() FrameDelivery {
	evs := r.B.ResolveTurn()
	r.ackedEvents = append(r.ackedEvents, evs...)

	var result map[string]any
	var outcome *Outcome
	winner := -1
	for _, e := range evs {
		if e.Type == "end" {
			if w, ok := e.Flags["winner"].(int); ok {
				winner = w
			}
		}
	}
	resultJSON := ""
	if r.B.Over {
		result = map[string]any{"winner": winner, "turns": r.B.Turn, "battle_id": r.ID}
		if raw, err := json.Marshal(result); err == nil {
			resultJSON = string(raw)
		}
		r.finishedAt = r.now()
		outcome = &Outcome{
			RoomID:     r.ID,
			Winner:     winner,
			CharIDs:    append([]int64(nil), r.outcome.CharIDs...),
			MonsterIDs: append([]string(nil), r.outcome.MonsterIDs...),
		}
		// 本场阵亡的野怪（按死亡事件 → 单位 id → 野怪 id）。
		dead := map[string]bool{}
		for _, e := range evs {
			if e.Type != "death" {
				continue
			}
			if mid, ok := r.monsterUnits[e.Target]; ok && !dead[mid] {
				dead[mid] = true
				outcome.Killed = append(outcome.Killed, mid)
			}
		}
	}
	// 落库指令流 + 终局结果（§8.2 可复盘；同步写——M2a 单库轻量）。
	_ = r.logs.AppendBattle(context.Background(), r.B.Seed, r.participantsJSON(), joinStream(r.cmdStream), resultJSON)
	r.B.cmds = map[int64]Cmd{}
	r.cmdStream = nil

	tokens := make([]string, 0, len(r.participants))
	for tok := range r.participants {
		tokens = append(tokens, tok)
	}
	return FrameDelivery{RoomID: r.ID, TurnVersion: r.B.Turn, Events: evs, Tokens: tokens, Result: result, Outcome: outcome}
}

// deliver 必须在 Room.mu 之外调用。
func (r *Room) deliver(d FrameDelivery) {
	if d.Events == nil {
		return
	}
	if h := r.onFrameHook; h != nil {
		h(d)
	}
}

func (r *Room) participantsJSON() string {
	out := "{"
	first := true
	for tok, id := range r.participants {
		if !first {
			out += ","
		}
		out += fmt.Sprintf("%q:%d", tok, id)
		first = false
	}
	return out + "}"
}

// isHostedToken 判断该令牌是否由服务端托管出招（4v4 队友与野怪）。
func isHostedToken(tok string) bool {
	return strings.HasPrefix(tok, "monster:") || strings.HasPrefix(tok, "ally:")
}

// joinStream 拼接复盘指令流。
func joinStream(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ";"
		}
		out += p
	}
	return out
}

// Manager 战斗房间集合 + 托管定时推进。
type Manager struct {
	cfg      config.BattleData
	logs     *store.BattleLogsRepo
	now      func() time.Time
	onFrame  func(FrameDelivery)
	onResend func(token string, turnVersion int, events []Event)
	mu       sync.Mutex
	rooms    map[string]*Room
	seq      int
}

func NewManager(cfg config.BattleData, logs *store.BattleLogsRepo) *Manager {
	return &Manager{cfg: cfg, logs: logs, now: time.Now, rooms: map[string]*Room{}}
}

// SetFrameHook 广播回调（装配层接 gateway Hub）。回调在房间锁外执行。
func (m *Manager) SetFrameHook(f func(FrameDelivery)) { m.onFrame = f }

// SetResendHook 补发回调（4/4 确认落后时把未确认帧重新推给该令牌）。
func (m *Manager) SetResendHook(f func(token string, turnVersion int, events []Event)) {
	m.onResend = f
}

// Resend 触发补发（不持任何房间锁）。
func (m *Manager) Resend(token string, turnVersion int, events []Event) {
	if len(events) == 0 {
		return
	}
	if h := m.onResend; h != nil {
		h(token, turnVersion, events)
	}
}

// Create 开局：同一令牌的旧房先关闭（避免 RoomByToken 命中已结束的旧战斗）。
func (m *Manager) Create(seed int64, sideA, sideB []*Unit, tokens map[string]int64) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, old := range m.rooms {
		if old.closed {
			continue
		}
		for tok := range tokens {
			if _, hit := old.participants[tok]; hit {
				old.closed = true
				delete(m.rooms, id)
				break
			}
		}
	}
	m.seq++
	room := &Room{
		ID:           fmt.Sprintf("b%d-%d", seed, m.seq),
		B:            NewBattle(seed, sideA, sideB, m.cfg),
		participants: tokens,
		acked:        map[string]int{},
		logs:         m.logs,
		now:          m.now,
		lastCmdTime:  m.now(),
		onFrameHook:  m.onFrame,
	}
	m.rooms[room.ID] = room
	return room
}

// Tick 托管推进：超 30 秒未收齐指令 → 缺席者托管普攻（§8.3/§8.1 超时口径）；
// 并回收终局超过 2 分钟的房间，避免 rooms 只增不减。
func (m *Manager) Tick() {
	m.mu.Lock()
	rooms := make([]*Room, 0, len(m.rooms))
	for _, r := range m.rooms {
		rooms = append(rooms, r)
	}
	m.mu.Unlock()

	var deliveries []FrameDelivery
	for _, r := range rooms {
		r.mu.Lock()
		if r.closed || r.B.Over {
			r.mu.Unlock()
			continue
		}
		if !r.allSubmitted() && r.now().Sub(r.lastCmdTime) > time.Duration(r.B.F.CmdTimeoutSeconds)*time.Second {
			for _, unitID := range r.participants {
				if u := r.B.unit(unitID); u != nil && u.Alive {
					if _, has := r.B.cmds[unitID]; !has {
						u.Hosted = true
						r.B.SubmitCmd(unitID, Cmd{Type: "attack"})
					}
				}
			}
			deliveries = append(deliveries, r.resolve())
		}
		r.mu.Unlock()
	}
	// 所有投递都在锁外执行。
	for _, d := range deliveries {
		if room := m.Room(d.RoomID); room != nil {
			room.deliver(d)
		}
	}
	m.reap()
}

// reap 关闭并从集合中移除终局已久的房间（留出 2 分钟确认/补发窗口）。
func (m *Manager) reap() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, r := range m.rooms {
		r.mu.Lock()
		expired := r.B.Over && !r.finishedAt.IsZero() && m.now().Sub(r.finishedAt) > 2*time.Minute
		if expired {
			r.closed = true
		}
		r.mu.Unlock()
		if expired {
			delete(m.rooms, id)
		}
	}
}

// Restore 断线重连：凭令牌取整帧快照 + 未确认事件（§8.4）。
// 快照改为 DTO 副本，不再把 []*Unit 指针直接交给序列化层（并发读写 + 内部字段泄漏）。
func (m *Manager) Restore(token string) (map[string]any, []Event, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rooms {
		if _, ok := r.participants[token]; !ok || r.closed {
			continue
		}
		r.mu.Lock()
		units := make([]map[string]any, 0, len(r.B.Units))
		for _, u := range r.B.Units {
			units = append(units, unitSnapshot(u))
		}
		snapshot := map[string]any{
			"battle_id":    r.ID,
			"turn_version": r.B.Turn,
			"units":        units,
			"seed":         r.B.Seed,
		}
		pending := r.pendingForLocked(token)
		r.mu.Unlock()
		return snapshot, pending, true
	}
	return nil, nil, false
}

// NextSeed 生成战斗种子（时间 + 自增，复盘唯一键）。
func (m *Manager) NextSeed() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	return time.Now().UnixNano() + int64(m.seq)
}

// RoomByToken 按令牌查所在房间。
func (m *Manager) RoomByToken(token string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rooms {
		if _, ok := r.participants[token]; ok && !r.closed {
			return r
		}
	}
	return nil
}

// Room 按 ID 查房间。
func (m *Manager) Room(id string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rooms[id]
}

// Participants 参与者令牌映射副本（供外部只读使用；帧广播请使用 FrameDelivery.Tokens）。
func (r *Room) Participants() map[string]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int64, len(r.participants))
	for k, v := range r.participants {
		out[k] = v
	}
	return out
}
