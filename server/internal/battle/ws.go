package battle

import (
	"context"
	"fmt"
	"log"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

// Service 为战斗域 WS 协议接入（主码 4）。
type Service struct {
	mgr   *Manager
	chars *store.CharactersRepo
	cfg   config.Data
	// testAttach 仅测试注入（token → 战斗单位直连）。
	testUnits map[string]*Unit
	testMon   *Unit
}

func NewService(mgr *Manager, chars *store.CharactersRepo, cfg config.Data) *Service {
	return &Service{mgr: mgr, chars: chars, cfg: cfg, testUnits: map[string]*Unit{}}
}

// 测试注入钩子。
func (s *Service) testAttach(token string, u *Unit) { s.testUnits[token] = u }
func (s *Service) testAttachMonster(mid string) {
	for i := range s.cfg.Battle.Monsters {
		m := &s.cfg.Battle.Monsters[i]
		if m.ID == mid {
			s.testMon = monsterUnit(m, 9001, 0)
			return
		}
	}
}

func (s *Service) monsterByID(id string) *config.MonsterDef {
	for i := range s.cfg.Battle.Monsters {
		if s.cfg.Battle.Monsters[i].ID == id {
			return &s.cfg.Battle.Monsters[i]
		}
	}
	return nil
}

// skillsForLevel 按等级过滤可用秘籍（演练场按基准等级给谱）。
func (s *Service) skillsForLevel(level int) []config.SkillDef {
	var skills []config.SkillDef
	for _, sk := range s.cfg.Battle.Skills {
		if level >= sk.LevelReq {
			skills = append(skills, sk)
		}
	}
	return skills
}

// playerUnit 从角色表构造战斗单位（派生属性 + 门派秘籍谱按等级过滤）。
func (s *Service) playerUnit(ctx context.Context, charID int64) (*Unit, error) {
	c, err := s.chars.GetCharacter(ctx, charID)
	if err != nil {
		return nil, err
	}
	st, err := DerivedStatsWithPoints(c.Level, c.Faction, s.cfg.Battle, s.cfg.Levels.PointGains, c.PointsMap())
	if err != nil {
		return nil, err
	}
	u := &Unit{ID: c.ID, Name: c.Name, Side: 0, Level: c.Level, Faction: c.Faction,
		BaseStats: st, Stats: st, HP: st.HP, MP: st.MP, Skills: s.skillsForLevel(c.Level), Alive: true}
	return u, nil
}

// monsterUnit 从怪物模板构造战斗单位（含 M2a-2 反伤与站位）。
func monsterUnit(m *config.MonsterDef, id int64, slot int) *Unit {
	st := UnitStats{
		HP: m.Stats.HP, MP: m.Stats.MP, PAtk: m.Stats.PAtk, MAtk: m.Stats.MAtk,
		PDef: m.Stats.PDef, MDef: m.Stats.MDef, Speed: m.Stats.Speed,
	}
	return &Unit{ID: id, Name: m.Name, Side: 1, Level: m.Level, Slot: slot,
		BaseStats: st, Stats: st, HP: st.HP, MP: st.MP, Alive: true,
		ReflectPct: m.ReflectPct}
}

// allyUnit 从玩家派生托管队友（4v4 演练；缩放系数全部来自 formula.drill）。
func allyUnit(p *Unit, idx int, d config.DrillParams) *Unit {
	scale := func(v, pct int) int { return v * pct / 10000 }
	st := p.BaseStats
	st.HP = scale(st.HP, d.AllyHPPct)
	st.PAtk = scale(st.PAtk, d.AllyAtkPct)
	st.MAtk = scale(st.MAtk, d.AllyAtkPct)
	st.Speed = scale(st.Speed, d.AllySpeedPct)
	return &Unit{
		ID: p.ID + int64(1000+idx), Name: fmt.Sprintf("同伴·%d", idx),
		Side: 0, Level: p.Level, Faction: p.Faction, Slot: idx,
		BaseStats: st, Stats: st, HP: st.HP, MP: st.MP, Alive: true,
	}
}

// drillMonsterUnit 演练敌人：保留模板名字与反伤，属性按基准玩家缩放（配置驱动）。
func drillMonsterUnit(m *config.MonsterDef, id int64, slot int, base UnitStats, d config.DrillParams) *Unit {
	scale := func(v, pct int) int {
		n := v * pct / 10000
		if n < 1 {
			return 1
		}
		return n
	}
	st := UnitStats{
		HP: scale(base.HP, d.EnemyHPPctOfPlayer), MP: m.Stats.MP,
		PAtk: scale(base.PAtk, d.EnemyAtkPctOfPlayer), MAtk: scale(base.MAtk, d.EnemyAtkPctOfPlayer),
		PDef: scale(base.PDef, d.EnemyDefPctOfPlayer), MDef: scale(base.MDef, d.EnemyDefPctOfPlayer),
		Speed: scale(base.Speed, d.EnemySpeedPctOfPlayer),
		PHit:  base.PHit, PDodge: base.PDodge, MDodge: base.MDodge,
		PCrit: base.PCrit, MCrit: base.MCrit, PCombo: base.PCombo,
		SealHit: base.SealHit, SealRes: base.SealRes, Flee: base.Flee,
	}
	return &Unit{ID: id, Name: m.Name, Side: 1, Level: d.Level, Slot: slot,
		BaseStats: st, Stats: st, HP: st.HP, MP: st.MP, Alive: true,
		ReflectPct: m.ReflectPct}
}

// HandleEnter 处理 4/1。默认 1v1 切磋；mode=drill 时按 formula.drill 组建 4v4 演练。
func (s *Service) HandleEnter(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	log.Printf("battle enter request: sess=%v char=%v payload=%v", sess != nil, sess.CharID, payload)
	if sess == nil || sess.CharID == 0 {
		return nil, 2, "未鉴权或未选角"
	}
	mode, _ := payload["mode"].(string)
	monsterID, _ := payload["monster_id"].(string)
	if mode != "drill" && s.monsterByID(monsterID) == nil {
		return nil, 3, "野怪不存在"
	}

	player := s.testUnits[sess.Token]
	if player == nil {
		var err error
		player, err = s.playerUnit(ctx, sess.CharID)
		if err != nil {
			return nil, 5, "角色不存在"
		}
	}
	player.Slot = 0

	var sideB []*Unit
	monsterUnits := map[int64]string{} // 战斗单位 id → 野怪配置 id（任务 kill 记账用）
	participants := map[string]int64{sess.Token: player.ID}
	sideA := []*Unit{player}
	if mode == "drill" {
		d := s.cfg.Battle.Formula.Drill
		// 演练场基准等级：重建玩家战斗副本（不改角色表），并按基准等级开放秘籍，
		// 避免 Lv1 玩家被高等级怪物模板秒杀、也能完整演示感电/反伤/顺序条。
		// 演练场按基准等级重建（确定性演示），不含角色加点。
		if st, err := DerivedStats(d.Level, player.Faction, s.cfg.Battle); err == nil {
			player.Level, player.BaseStats, player.Stats = d.Level, st, st
			player.HP, player.MP = st.HP, st.MP
		}
		player.Skills = s.skillsForLevel(d.Level)
		for i := 0; i < d.Allies; i++ {
			ally := allyUnit(player, i+1, d)
			sideA = append(sideA, ally)
			participants[fmt.Sprintf("ally:%d", i)] = ally.ID
		}
		for i, mid := range d.Enemies {
			m := s.monsterByID(mid)
			if m == nil {
				return nil, 3, "演练阵容野怪不存在: " + mid
			}
			mon := drillMonsterUnit(m, int64(900000+i*1000+m.Level), i, player.BaseStats, d)
			sideB = append(sideB, mon)
			monsterUnits[mon.ID] = mid
			participants[fmt.Sprintf("monster:%d", i)] = mon.ID
		}
	} else {
		var mon *Unit
		if s.testUnits[sess.Token] != nil {
			mon = s.testMon
		} else {
			mon = monsterUnit(s.monsterByID(monsterID), int64(900000+s.monsterByID(monsterID).Level), 0)
		}
		sideB = []*Unit{mon}
		monsterUnits[mon.ID] = monsterID
		participants["monster:"+monsterID] = mon.ID
	}

	seed := s.mgr.NextSeed()
	room := s.mgr.Create(seed, sideA, sideB, participants)
	// 终局归属元数据：任务系统据此推进 kill 目标（M2b）。
	room.SetOutcomeMeta([]int64{sess.CharID}, monsterUnits)
	units := make([]map[string]any, 0, len(sideA)+len(sideB))
	for _, u := range append(append([]*Unit{}, sideA...), sideB...) {
		units = append(units, unitSnapshot(u))
	}
	return map[string]any{
		"battle_id":    room.ID,
		"turn_version": 0,
		"seed":         seed,
		"mode":         mode,
		"self":         unitSnapshot(player),
		"enemy":        unitSnapshot(sideB[0]),
		// units[] 与 4/7 Restore 同构，多单位站位直接复用。
		"units":  units,
		"skills": skillSnapshots(player.Skills),
	}, 0, ""
}

// HandleCmd 处理 4/2 指令（含 4v4 目标选择）。
func (s *Service) HandleCmd(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil {
		return nil, 2, "未鉴权"
	}
	room := s.mgr.RoomByToken(sess.Token)
	if room == nil {
		return nil, 5, "不在战斗中"
	}
	cmdType, _ := payload["cmd"].(string)
	skillID, _ := payload["skill_id"].(string)
	var targetID int64
	if v, ok := payload["target_id"].(float64); ok {
		targetID = int64(v)
	}
	room.OnCmd(sess.Token, Cmd{Type: cmdType, SkillID: skillID, TargetID: targetID})
	return map[string]any{"ok": true}, 0, ""
}

// HandleAck 处理 4/4 帧确认。
func (s *Service) HandleAck(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil {
		return nil, 2, "未鉴权"
	}
	room := s.mgr.RoomByToken(sess.Token)
	if room == nil {
		return nil, 5, "不在战斗中"
	}
	tv, _ := payload["turn_version"].(float64)
	cur, pending := room.OnAck(sess.Token, int(tv))
	// 客户端确认落后 → 凭 turn_version 补发未确认帧（不重算，§8.4）。
	s.mgr.Resend(sess.Token, cur, pending)
	return map[string]any{"ok": true}, 0, ""
}

// HandleRestore 处理 4/7 断线恢复（§8.4 整帧恢复）。
func (s *Service) HandleRestore(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil {
		return nil, 2, "未鉴权"
	}
	snap, pending, ok := s.mgr.Restore(sess.Token)
	if !ok {
		return nil, 5, "无进行中的战斗"
	}
	snap["pending"] = pending
	return snap, 0, ""
}

// sessionFor 测试会话（含 CharID 绑定）。
func sessionFor(token string) *session.Session {
	return &session.Session{Token: token, AccountID: 1, CharID: 1}
}

// fullData 由 BattleData 组装最小 config.Data（Service 构造用）。
func fullData(b config.BattleData) config.Data {
	return config.Data{Battle: b}
}

// skillSnapshots 下发玩家当前可用秘籍（供指令台数据驱动渲染）。
// 心法为被动，不进主动指令列表。
func skillSnapshots(skills []config.SkillDef) []map[string]any {
	out := make([]map[string]any, 0, len(skills))
	for _, sk := range skills {
		if sk.Category == "心法" {
			continue
		}
		out = append(out, map[string]any{
			"id": sk.ID, "name": sk.Name, "category": sk.Category,
			"mp_cost": sk.MPCost, "level_req": sk.LevelReq, "targets": sk.Targets,
		})
	}
	return out
}

// unitSnapshot 战斗单位回包（不含令牌——凭据不离开服务端）。
func unitSnapshot(u *Unit) map[string]any {
	return map[string]any{
		"id": u.ID, "name": u.Name, "side": u.Side, "level": u.Level, "slot": u.Slot,
		"hp": u.HP, "mp": u.MP, "max_hp": u.BaseStats.HP, "max_mp": u.BaseStats.MP,
		"shock_stacks": u.ShockStacks, "reflect_pct": u.ReflectPct,
	}
}
