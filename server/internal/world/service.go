package world

import (
	"context"
	"errors"
	"log"
	"math"

	"fantasy-jianghu/server/internal/protocol"
	"fantasy-jianghu/server/internal/session"
)

// 错误码（M1 错误码表）。
const (
	codeOK           = 0
	codeGeneric      = 1
	codeUnauthorized = 2
	codeBadParams    = 3
	codeNotFound     = 5
	codeOverLimit    = 7
)

// assignLine 取最低编号未满线；全满自动追加新线（V1.27：新进入分配未满线、满线自动开线）。
func (s *Service) assignLine(mapID string) *Scene {
	logic := s.scenes[mapID]
	capacity := s.cfg.LineCapacity(logic.Type)
	for _, sc := range s.lines[mapID] {
		if sc.playerCount() < capacity {
			return sc
		}
	}
	sc := newScene(logic, len(s.lines[mapID])+1, capacity)
	s.lines[mapID] = append(s.lines[mapID], sc)
	return sc
}

// HandleEnter 处理 WS 3/1 进场帧：分配未满线，快照（场景逻辑数据 + 本人 + 同线他者）。
func (s *Service) HandleEnter(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	c, err := s.characters.GetCharacter(ctx, sess.CharID)
	if err != nil {
		return nil, codeNotFound, "角色不存在"
	}
	// 无存档场景（新角/旧数据）兜底到出生点场景。
	mapID := c.MapID
	if _, ok := s.scenes[mapID]; !ok {
		mapID = s.cfg.App.Spawn.MapID
	}
	if _, ok := s.scenes[mapID]; !ok {
		return nil, codeNotFound, "场景不存在"
	}
	scene := s.assignLine(mapID)
	ymin, ymax := scene.Logic.YBounds()
	spawnX, spawnY := scene.Logic.SpawnPoint()
	x := c.PosX
	if math.IsNaN(x) || math.IsInf(x, 0) || x <= 0 {
		x = spawnX
	}
	x = clamp(x, scene.Logic.Walk.XMin, scene.Logic.Walk.XMax)
	y := c.PosY
	if math.IsNaN(y) || math.IsInf(y, 0) || y <= 0 || y < ymin || y > ymax {
		y = spawnY
	}
	y = clamp(y, ymin, ymax)
	// 旧横版存档迁移：y 等于旧 ground_y 的角色回到俯视二维出生点。
	if scene.Logic.LegacyGroundY > 0 && c.PosY == scene.Logic.LegacyGroundY {
		x, y = spawnX, spawnY
	}

	scene.mu.Lock()
	// 快照在场者（排除本人）。
	var others []*PlayerState
	for _, p := range scene.players {
		if p.CharID == c.ID {
			continue
		}
		others = append(others, p)
	}
	scene.players[c.ID] = &PlayerState{
		CharID: c.ID, Token: sess.Token, Name: c.Name,
		Faction: c.Faction, Gender: c.Gender, X: x, Y: y,
		LastMove: s.now(),
	}
	scene.mu.Unlock()

	// 推送加入事件给同线在场者（§5.2 事件通知不可丢，M1 简化为直推）。
	for _, o := range others {
		s.hub.SendTo(o.Token, 3, protocol.SubPlayerJoin, map[string]any{
			"id": c.ID, "name": c.Name, "faction": c.Faction, "gender": c.Gender,
			"x": x, "y": y, "mount": nil, "pet": nil,
		})
	}
	// 进场回调：任务 visit 目标推进（M2b）。放在快照之后，失败不影响进场。
	if s.OnEnter != nil {
		s.OnEnter(c.ID, scene.Logic.MapID)
	}

	return s.enterSnapshot(scene, c.ID, c.Name, x, y, others), codeOK, ""
}

// enterSnapshot 组装进场/切线回包（同形状）。
func (s *Service) enterSnapshot(scene *Scene, charID int64, name string, x, y float64, others []*PlayerState) map[string]any {
	npcs := make([]map[string]any, 0, len(scene.Logic.NPCs))
	for _, n := range scene.Logic.NPCs {
		npcs = append(npcs, map[string]any{"id": n.ID, "name": n.Name, "x": n.X, "y": n.Y})
	}
	ymin, ymax := scene.Logic.YBounds()
	spawnX, spawnY := scene.Logic.SpawnPoint()
	playerList := make([]map[string]any, 0, len(others))
	for _, o := range others {
		playerList = append(playerList, publicPlayerDTO(o))
	}
	return map[string]any{
		"map_id":   scene.Logic.MapID,
		"line_no":  scene.LineNo,
		"ground_y": scene.Logic.GroundY,
		"walk":     map[string]any{"x_min": scene.Logic.Walk.XMin, "x_max": scene.Logic.Walk.XMax},
		"bounds":   map[string]any{"x_min": scene.Logic.Walk.XMin, "x_max": scene.Logic.Walk.XMax, "y_min": ymin, "y_max": ymax},
		"spawn":    map[string]any{"x": spawnX, "y": spawnY},
		"npcs":     npcs,
		"self":     map[string]any{"id": charID, "name": name, "x": x, "y": y},
		"players":  playerList,
	}
}

// HandleMove 处理 WS 3/2 移动帧：服务端权威钳制（防瞬移）+ 广播。
// 超速/越界一律钳到合法位置并回发权威坐标，不拒绝、不惩罚（客户端预测过冲纠偏口径）。
func (s *Service) HandleMove(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	scene := s.locate(sess.CharID)
	if scene == nil {
		return nil, codeGeneric, "不在场景中"
	}
	xt, ok := payload["x"].(float64)
	if !ok || math.IsNaN(xt) || math.IsInf(xt, 0) {
		return nil, codeGeneric, "坐标非法"
	}
	yt, hasY := 0.0, false
	if raw, exists := payload["y"]; exists {
		if v, ok := raw.(float64); ok && !math.IsNaN(v) && !math.IsInf(v, 0) {
			yt, hasY = v, true
		}
	}
	scene.mu.RLock()
	p, inScene := scene.players[sess.CharID]
	scene.mu.RUnlock()
	if !inScene {
		return nil, codeGeneric, "不在场景中"
	}

	scene.mu.Lock()
	defer scene.mu.Unlock()
	now := s.now()
	elapsed := now.Sub(p.LastMove).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	allowed := s.cfg.App.MoveSpeed*math.Min(elapsed, 1.0) + s.cfg.App.MoveTolerance
	if !hasY {
		yt = p.Y
	}
	ymin, ymax := scene.Logic.YBounds()
	tx := clamp(xt, scene.Logic.Walk.XMin, scene.Logic.Walk.XMax)
	ty := clamp(yt, ymin, ymax)
	// 二维防瞬移：欧氏距离超限则按方向等比缩回，x/y 保持同一比例，不产生斜向瞬移。
	dx, dy := tx-p.X, ty-p.Y
	if dist := math.Hypot(dx, dy); dist > allowed {
		k := allowed / dist
		tx = p.X + dx*k
		ty = p.Y + dy*k
	}
	log.Printf("move char=%d from=(%.1f,%.1f) req=(%.1f,%.1f) elapsed=%.2fs allowed=%.1f -> (%.1f,%.1f)",
		p.CharID, p.X, p.Y, xt, yt, elapsed, allowed, tx, ty)
	p.X, p.Y = tx, ty
	p.LastMove = now
	p.dirty = true

	// 广播给场景内其他人（不含本人）。
	for _, o := range scene.players {
		if o.CharID == p.CharID {
			continue
		}
		s.hub.SendTo(o.Token, 3, protocol.SubMoveBroadcast, map[string]any{"id": p.CharID, "x": p.X, "y": p.Y})
	}
	return map[string]any{"x": p.X, "y": p.Y}, codeOK, ""
}

// HandleLeave 处理 WS 3/4 离场帧：移除、落盘、广播。
func (s *Service) HandleLeave(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	scene := s.locate(sess.CharID)
	if scene == nil {
		return map[string]any{"ok": true}, codeOK, ""
	}
	scene.mu.Lock()
	p, inScene := scene.players[sess.CharID]
	if inScene {
		delete(scene.players, sess.CharID)
	}
	scene.mu.Unlock()
	if !inScene {
		return map[string]any{"ok": true}, codeOK, ""
	}
	if err := s.characters.UpdatePosition(ctx, p.CharID, scene.Logic.MapID, p.X, p.Y); err != nil {
		return nil, codeGeneric, "位置落盘失败"
	}
	for _, o := range s.snapshot(scene) {
		s.hub.SendTo(o.Token, 3, protocol.SubLeaveBroadcast, map[string]any{"id": p.CharID})
	}
	return map[string]any{"ok": true}, codeOK, ""
}

// HandleDisconnect 处理连接掉线（gateway OnClose 回调）：移除玩家、
// 位置落库（掉线即写）、向在场者广播离场——防止幽灵玩家。
func (s *Service) HandleDisconnect(ctx context.Context, token string) {
	scene := s.sceneByToken(token)
	if scene == nil {
		return
	}
	scene.mu.Lock()
	var p *PlayerState
	for _, pl := range scene.players {
		if pl.Token == token {
			p = pl
			delete(scene.players, pl.CharID)
			break
		}
	}
	scene.mu.Unlock()
	if p == nil {
		return
	}
	if err := s.characters.UpdatePosition(ctx, p.CharID, scene.Logic.MapID, p.X, p.Y); err != nil {
		log.Printf("disconnect persist error char=%d: %v", p.CharID, err)
	}
	for _, o := range s.snapshot(scene) {
		s.hub.SendTo(o.Token, 3, protocol.SubLeaveBroadcast, map[string]any{"id": p.CharID})
	}
}

// Sweep 将全部脏位落盘（装配层 30s 定时调用 + 断连兜底由 HandleLeave/掉线钩子承担）。
func (s *Service) Sweep(ctx context.Context) error {
	for _, lines := range s.lines {
		for _, scene := range lines {
			scene.mu.Lock()
			dirty := make([]*PlayerState, 0, len(scene.players))
			for _, p := range scene.players {
				if p.dirty {
					dirty = append(dirty, p)
					p.dirty = false
				}
			}
			scene.mu.Unlock()
			for _, p := range dirty {
				if err := s.characters.UpdatePosition(ctx, p.CharID, scene.Logic.MapID, p.X, p.Y); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// locate 按角色定位所在分线（跨该角色场景的全部线扫描）。
func (s *Service) locate(charID int64) *Scene {
	for _, lines := range s.lines {
		for _, scene := range lines {
			scene.mu.RLock()
			_, ok := scene.players[charID]
			scene.mu.RUnlock()
			if ok {
				return scene
			}
		}
	}
	return nil
}

// sceneByToken 按令牌定位所在分线（掉线清理用）。
func (s *Service) sceneByToken(token string) *Scene {
	for _, lines := range s.lines {
		for _, scene := range lines {
			scene.mu.RLock()
			found := false
			for _, pl := range scene.players {
				if pl.Token == token {
					found = true
					break
				}
			}
			scene.mu.RUnlock()
			if found {
				return scene
			}
		}
	}
	return nil
}

// HandleLineList 处理 WS 3/7：各线编号/人数/容量 + 本人当前线。
func (s *Service) HandleLineList(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	mapID, _ := payload["map_id"].(string)
	if mapID == "" {
		mapID = s.cfg.App.Spawn.MapID
	}
	if _, ok := s.scenes[mapID]; !ok {
		return nil, codeNotFound, "场景不存在"
	}
	lineSummaries := make([]map[string]any, 0, len(s.lines[mapID]))
	for _, sc := range s.lines[mapID] {
		lineSummaries = append(lineSummaries, map[string]any{
			"no":       sc.LineNo,
			"count":    sc.playerCount(),
			"capacity": sc.Capacity,
		})
	}
	current := 0
	if sc := s.locate(sess.CharID); sc != nil && sc.Logic.MapID == mapID {
		current = sc.LineNo
	}
	return map[string]any{"map_id": mapID, "current": current, "lines": lineSummaries}, codeOK, ""
}

// HandleSwitchLine 处理 WS 3/8：同图切线——目标未满才放行（满 code 7），
// 摘除当前线（广播离场）→ 加入目标线（广播加入）→ 回目标线快照。
func (s *Service) HandleSwitchLine(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	scene := s.locate(sess.CharID)
	if scene == nil {
		return nil, codeGeneric, "不在场景中"
	}
	noF, ok := payload["line_no"].(float64)
	if !ok || noF != float64(int(noF)) || int(noF) < 1 {
		return nil, codeBadParams, "线号非法"
	}
	targetNo := int(noF)
	if targetNo == scene.LineNo {
		return nil, codeBadParams, "已在目标线"
	}
	s.muLines.RLock()
	var target *Scene
	if targetNo <= len(s.lines[scene.Logic.MapID]) {
		target = s.lines[scene.Logic.MapID][targetNo-1]
	}
	s.muLines.RUnlock()
	if target == nil {
		return nil, codeBadParams, "线号非法"
	}
	if target.playerCount() >= target.Capacity {
		return nil, codeOverLimit, "目标线已满，请稍后再试"
	}

	scene.mu.Lock()
	p, inScene := scene.players[sess.CharID]
	if inScene {
		delete(scene.players, sess.CharID)
	}
	scene.mu.Unlock()
	if !inScene {
		return nil, codeGeneric, "不在场景中"
	}
	// 通知原线他人离场。
	for _, o := range s.snapshot(scene) {
		s.hub.SendTo(o.Token, 3, protocol.SubLeaveBroadcast, map[string]any{"id": p.CharID})
	}

	// 加入目标线：位置沿用在原线的权威坐标（切线不瞬移）。
	x := p.X
	y := p.Y
	target.mu.Lock()
	var others []*PlayerState
	for _, o := range target.players {
		others = append(others, o)
	}
	target.players[p.CharID] = &PlayerState{
		CharID: p.CharID, Token: p.Token, Name: p.Name,
		Faction: p.Faction, Gender: p.Gender, X: x, Y: y,
		LastMove: s.now(),
	}
	target.mu.Unlock()
	for _, o := range others {
		s.hub.SendTo(o.Token, 3, protocol.SubPlayerJoin, map[string]any{
			"id": p.CharID, "name": p.Name, "faction": p.Faction, "gender": p.Gender,
			"x": x, "y": y, "mount": nil, "pet": nil,
		})
	}
	return s.enterSnapshot(target, p.CharID, p.Name, x, y, others), codeOK, ""
}

func (s *Service) snapshot(scene *Scene) []*PlayerState {
	scene.mu.RLock()
	defer scene.mu.RUnlock()
	out := make([]*PlayerState, 0, len(scene.players))
	for _, p := range scene.players {
		out = append(out, p)
	}
	return out
}

// publicPlayerDTO 只含公开字段——会话令牌不得离开服务端。
// mount/pet 为 V1.27 同屏展示范围预留（M3 坐骑/宠物系统落地前恒 null）。
func publicPlayerDTO(p *PlayerState) map[string]any {
	return map[string]any{
		"id": p.CharID, "name": p.Name, "faction": p.Faction, "gender": p.Gender,
		"x": p.X, "y": p.Y, "mount": nil, "pet": nil,
	}
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}

func charVal(x float64, scene *Scene) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return scene.Logic.Walk.XMin
	}
	return x
}

var errNoScene = errors.New("world: scene not found")
