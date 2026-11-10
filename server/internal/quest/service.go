// Package quest 为任务域（开发技术规范 §4 / GDD §9）：
// NPC 发布 → 接取 → 服务端事件推进进度 → 交付发奖。
// 主线与三类日常分表计数；交付前对完整奖励清单做背包容量预检，不通过整单拒绝。
package quest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"fantasy-jianghu/server/internal/battle"
	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

// 错误码（沿用 M1 错误码表，任务域新增 8/9）。
const (
	codeOK           = 0
	codeGeneric      = 1
	codeUnauthorized = 2
	codeBadParams    = 3
	codeNotFound     = 5
	codeBagFull      = 8
	codeBadState     = 9
)

// cst 为服务器结算时区（GDD §10.1：活动与日常以服务器时间 UTC+8 为准）。
var cst = time.FixedZone("CST", 8*3600)

// Pusher 为按角色定向推送的契约（任务进度 11/4）。
type Pusher interface {
	SendToChar(charID int64, main, sub uint16, payload map[string]any) bool
}

type Service struct {
	quests  *store.QuestRepo
	chars   *store.CharactersRepo
	items   *store.ItemsRepo
	cfg     config.Data
	pusher  Pusher
	now     func() time.Time
	mainSub uint16 // 11/4 子码（常量注入，避免领域层依赖协议包）
}

// NewService 构造任务服务；mainSub 为主码 11 的进度推送子码。
func NewService(quests *store.QuestRepo, chars *store.CharactersRepo, items *store.ItemsRepo,
	cfg config.Data, pusher Pusher, mainSub uint16) *Service {
	return &Service{quests: quests, chars: chars, items: items, cfg: cfg, pusher: pusher,
		now: func() time.Time { return time.Now() }, mainSub: mainSub}
}

// SetClock 注入测试时钟。
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) today() string { return s.now().In(cst).Format("2006-01-02") }

// questView 为任务单条视图（客户端面板与 NPC 对话选项的唯一数据来源）。
type questView struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Chapter    int               `json:"chapter"`
	Desc       string            `json:"desc"`
	State      string            `json:"state"` // available|active|completable|done|locked
	Reason     string            `json:"reason,omitempty"`
	Giver      config.QuestGiver `json:"giver"`
	Objectives []objectiveView   `json:"objectives"`
	Rewards    rewardView        `json:"rewards"`
}

type objectiveView struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Cur    int    `json:"cur"`
	Need   int    `json:"need"`
}

type rewardItemView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type rewardView struct {
	Exp    int64            `json:"exp"`
	Xiuwei int64            `json:"xiuwei"`
	Gold   int64            `json:"gold"`
	Items  []rewardItemView `json:"items"`
}

type dailyView struct {
	Used  int `json:"used"`
	Limit int `json:"limit"`
}

// buildList 组装角色视角的任务全量状态（列表 + 日常计数 + 等级经验 + 钱包）。
func (s *Service) buildList(ctx context.Context, charID int64) (map[string]any, error) {
	c, err := s.chars.GetCharacter(ctx, charID)
	if err != nil {
		return nil, err
	}
	rows, err := s.quests.List(ctx, charID)
	if err != nil {
		return nil, err
	}
	wallet, err := s.items.WalletOf(ctx, charID)
	if err != nil {
		return nil, err
	}
	day := s.today()
	doneToday := map[string]int{}
	activeDaily := map[string]int{}
	byQuest := map[string]store.CharacterQuest{}
	doneMain := map[string]bool{}
	for _, r := range rows {
		if r.State == "active" {
			byQuest[r.QuestID] = r
			if cat := categoryOf(r.Kind); cat != "" {
				activeDaily[cat]++
			}
		} else if r.State == "done" {
			if r.Kind == "main" {
				doneMain[r.QuestID] = true
			}
		}
	}
	for _, cat := range []string{"normal", "sect", "dungeon"} {
		n, err := s.quests.CompletedToday(ctx, charID, cat, day)
		if err != nil {
			return nil, err
		}
		doneToday[cat] = n
	}

	var views []questView
	for _, q := range s.cfg.Quests.Quests {
		v := questView{
			ID: q.ID, Name: q.Name, Type: q.Type, Chapter: q.Chapter, Desc: q.Desc, Giver: q.Giver,
			Rewards: s.rewardView(q.Rewards),
		}
		// 目标进度：进行中取存档值，其余取 0/需求值。
		if row, ok := byQuest[q.ID]; ok {
			v.Objectives = objectivesFrom(row.Progress, q.Objectives)
			v.State = "active"
			if allDone(row.Progress) {
				v.State = "completable"
			}
		} else {
			v.Objectives = objectivesFrom(nil, q.Objectives)
			switch {
			case q.IsDaily():
				cat := q.DailyCategory()
				used := doneToday[cat]
				if used+activeDaily[cat] >= s.cfg.Quests.DailyLimit(cat) {
					v.State, v.Reason = "locked", fmt.Sprintf("今日%s日常已达上限（%d 次）", categoryName(cat), s.cfg.Quests.DailyLimit(cat))
				} else if c.Level < q.MinLevel {
					v.State, v.Reason = "locked", fmt.Sprintf("需 %d 级", q.MinLevel)
				} else {
					v.State = "available"
				}
			case doneMain[q.ID]:
				v.State = "done"
			case c.Level < q.MinLevel:
				v.State, v.Reason = "locked", fmt.Sprintf("需 %d 级", q.MinLevel)
			case q.Prev != "" && !doneMain[q.Prev]:
				v.State, v.Reason = "locked", "前置任务未完成"
			default:
				v.State = "available"
			}
		}
		views = append(views, v)
	}

	daily := map[string]any{}
	for _, cat := range []string{"normal", "sect", "dungeon"} {
		daily[cat] = dailyView{Used: doneToday[cat] + activeDaily[cat], Limit: s.cfg.Quests.DailyLimit(cat)}
	}
	expNext := s.cfg.Levels.ExpToNext(c.Level)
	gateReady := s.cfg.Levels.IsGate(c.Level) && c.Breakthrough < c.Level && c.Exp >= expNext
	// 属性预览（加点界面右栏）需要的派生值：服务端权威计算，客户端只做展示。
	statsView := map[string]any{}
	if st, err := battle.DerivedStatsWithPoints(c.Level, c.Faction, s.cfg.Battle,
		s.cfg.Levels.PointGains, c.PointsMap()); err == nil {
		statsView = map[string]any{
			"hp": st.HP, "mp": st.MP, "patk": st.PAtk, "matk": st.MAtk,
			"pdef": st.PDef, "mdef": st.MDef, "speed": st.Speed,
			"pdodge": st.PDodge, "pcrit": st.PCrit, "phit": st.PHit,
		}
	}
	return map[string]any{
		"level":          c.Level,
		"exp":            c.Exp,
		"exp_next":       expNext,
		"gate_ready":     gateReady,
		"unspent_points": c.UnspentPoints,
		"points":         c.PointsMap(),
		"stats":          statsView,
		"wallet":         map[string]any{"gold": wallet.Gold, "xiuwei": wallet.Xiuwei, "yuanbao": wallet.Yuanbao},
		"daily":          daily,
		"quests":         views,
	}, nil
}

func (s *Service) rewardView(r config.QuestRewards) rewardView {
	rv := rewardView{Exp: r.Exp, Xiuwei: r.Xiuwei, Gold: r.Gold, Items: []rewardItemView{}}
	for _, ir := range r.Items {
		name := ir.ID
		if it := s.cfg.Items.ByID(ir.ID); it != nil {
			name = it.Name
		}
		rv.Items = append(rv.Items, rewardItemView{ID: ir.ID, Name: name, Count: ir.Count})
	}
	return rv
}

func objectivesFrom(progress []store.QuestObjectiveProgress, defs []config.QuestObjective) []objectiveView {
	out := make([]objectiveView, 0, len(defs))
	for _, d := range defs {
		cur := 0
		for _, p := range progress {
			if p.Kind == d.Kind && p.Target == d.Target {
				cur = p.Cur
				break
			}
		}
		out = append(out, objectiveView{Kind: d.Kind, Target: d.Target, Cur: cur, Need: d.Count})
	}
	return out
}

func allDone(progress []store.QuestObjectiveProgress) bool {
	if len(progress) == 0 {
		return false
	}
	for _, p := range progress {
		if p.Cur < p.Need {
			return false
		}
	}
	return true
}

func categoryOf(kind string) string {
	switch kind {
	case "daily_normal":
		return "normal"
	case "daily_sect":
		return "sect"
	case "daily_dungeon":
		return "dungeon"
	}
	return ""
}

func categoryName(cat string) string {
	switch cat {
	case "normal":
		return "普通"
	case "sect":
		return "宗派"
	case "dungeon":
		return "副本"
	}
	return cat
}

// HandleList 处理 11/1 任务全量状态。
func (s *Service) HandleList(ctx context.Context, sess *session.Session, _ map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	v, err := s.buildList(ctx, sess.CharID)
	if err != nil {
		return nil, codeGeneric, "读取任务状态失败"
	}
	return v, codeOK, ""
}

// HandleAccept 处理 11/2 接取。
func (s *Service) HandleAccept(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	questID, _ := payload["quest_id"].(string)
	def := s.cfg.Quests.ByID(questID)
	if def == nil {
		return nil, codeNotFound, "任务不存在"
	}
	c, err := s.chars.GetCharacter(ctx, sess.CharID)
	if err != nil {
		return nil, codeNotFound, "角色不存在"
	}
	if c.Level < def.MinLevel {
		return nil, codeBadState, fmt.Sprintf("需 %d 级才能接取", def.MinLevel)
	}
	if def.Prev != "" {
		rows, err := s.quests.List(ctx, sess.CharID)
		if err != nil {
			return nil, codeGeneric, "读取任务状态失败"
		}
		prevDone := false
		for _, r := range rows {
			if r.QuestID == def.Prev && r.State == "done" {
				prevDone = true
				break
			}
		}
		if !prevDone {
			return nil, codeBadState, "前置任务未完成"
		}
	}
	progress := make([]store.QuestObjectiveProgress, 0, len(def.Objectives))
	for _, o := range def.Objectives {
		progress = append(progress, store.QuestObjectiveProgress{Kind: o.Kind, Target: o.Target, Need: o.Count})
	}
	cat := def.DailyCategory()
	_, err = s.quests.Accept(ctx, sess.CharID, questID, def.Type, progress, s.today(), cat,
		s.cfg.Quests.DailyLimit(cat), s.now().Unix())
	switch {
	case errors.Is(err, store.ErrAlreadyTaken):
		return nil, codeBadState, "任务已接取"
	case errors.Is(err, store.ErrDailyLimit):
		return nil, codeBadState, fmt.Sprintf("今日%s日常已达上限", categoryName(cat))
	case err != nil:
		return nil, codeGeneric, "接取失败"
	}
	v, err := s.buildList(ctx, sess.CharID)
	if err != nil {
		return nil, codeGeneric, "读取任务状态失败"
	}
	return map[string]any{"ok": true, "quests": v["quests"], "daily": v["daily"]}, codeOK, ""
}

// HandleSubmit 处理 11/3 交付：目标校验 → 背包预检 → 单事务发奖。
func (s *Service) HandleSubmit(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	questID, _ := payload["quest_id"].(string)
	def := s.cfg.Quests.ByID(questID)
	if def == nil {
		return nil, codeNotFound, "任务不存在"
	}
	rows, err := s.quests.List(ctx, sess.CharID)
	if err != nil {
		return nil, codeGeneric, "读取任务状态失败"
	}
	var row *store.CharacterQuest
	for i := range rows {
		if rows[i].QuestID == questID && rows[i].State == "active" {
			row = &rows[i]
			break
		}
	}
	if row == nil {
		return nil, codeBadState, "任务未接取或已交付"
	}
	if !allDone(row.Progress) {
		return nil, codeBadState, "任务目标未完成"
	}
	items := make([]store.GrantItem, 0, len(def.Rewards.Items))
	for _, ir := range def.Rewards.Items {
		items = append(items, store.GrantItem{ID: ir.ID, Count: ir.Count})
	}
	cat := def.DailyCategory()
	limit := s.cfg.Quests.DailyLimit(cat)
	res, err := s.quests.Submit(ctx, store.SubmitRequest{
		CharacterID: sess.CharID, QuestRowID: row.ID, QuestID: questID, Kind: def.Type,
		Day: s.today(), DailyCategory: cat, DailyLimit: limit,
		Exp: def.Rewards.Exp, Xiuwei: def.Rewards.Xiuwei, Gold: def.Rewards.Gold,
		Items: items, StackOf: s.cfg.Items.ItemStack, BagCapacity: s.cfg.Items.BagCapacity,
		Curve: store.LevelCurve{ExpToNext: s.cfg.Levels.ExpToNext, IsGate: s.cfg.Levels.IsGate,
			MaxLevel: s.cfg.Levels.MaxLevel, PointsPerLevel: s.cfg.Levels.PointsPerLevel},
	})
	switch {
	case errors.Is(err, store.ErrBagFull):
		return nil, codeBagFull, fmt.Sprintf("背包容量不足，请先整理背包再交付（%v）", err)
	case errors.Is(err, store.ErrDailyLimit):
		return nil, codeBadState, "今日该类日常已达上限"
	case errors.Is(err, store.ErrQuestState):
		return nil, codeBadState, "任务状态不允许交付"
	case err != nil:
		return nil, codeGeneric, "交付失败"
	}
	// 奖励明细（含名称）与到账后余额。
	itemsOut := make([]rewardItemView, 0, len(res.Items))
	for _, it := range res.Items {
		name := it.ID
		if d := s.cfg.Items.ByID(it.ID); d != nil {
			name = d.Name
		}
		itemsOut = append(itemsOut, rewardItemView{ID: it.ID, Name: name, Count: it.Count})
	}
	out := map[string]any{
		"ok":       true,
		"quest_id": questID,
		"rewards": map[string]any{
			"exp": res.Exp, "xiuwei": res.Xiuwei, "gold": res.Gold, "items": itemsOut,
		},
		"wallet": map[string]any{"gold": res.Gold, "xiuwei": res.Xiuwei},
	}
	// 日常达标标记：邮件宝箱待 M2c 接入，本轮只把状态告诉客户端。
	if cat != "" {
		out["daily_used"] = res.DailyCount
		out["daily_limit"] = limit
		if res.DailyCount >= limit {
			out["daily_chest"] = true
		}
	}
	s.pushProgress(sess.CharID, questID, "done", row.Progress)
	return out, codeOK, ""
}

// HandleDaily 处理 11/5 日常状态。
func (s *Service) HandleDaily(ctx context.Context, sess *session.Session, _ map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	day := s.today()
	out := map[string]any{"day": day}
	for _, cat := range []string{"normal", "sect", "dungeon"} {
		done, err := s.quests.CompletedToday(ctx, sess.CharID, cat, day)
		if err != nil {
			return nil, codeGeneric, "读取日常状态失败"
		}
		out[cat] = dailyView{Used: done, Limit: s.cfg.Quests.DailyLimit(cat)}
	}
	return out, codeOK, ""
}

// RecordProgress 推进某角色所有匹配目标的任务，并推送 11/4。
// 供战斗终局（kill）、NPC 对话（talk）、进场（visit）调用；无匹配时静默返回 nil。
func (s *Service) RecordProgress(ctx context.Context, charID int64, kind, target string, n int) error {
	if charID == 0 || n <= 0 {
		return nil
	}
	changed, err := s.quests.AddProgress(ctx, charID, kind, target, n)
	if err != nil {
		return err
	}
	for _, q := range changed {
		state := "active"
		if allDone(q.Progress) {
			state = "completable"
		}
		s.pushProgress(charID, q.QuestID, state, q.Progress)
	}
	return nil
}

// pushProgress 向在线角色推送任务进度（不在线则丢弃，登录后走 11/1 拉全量）。
func (s *Service) pushProgress(charID int64, questID, state string, progress []store.QuestObjectiveProgress) {
	if s.pusher == nil || s.mainSub == 0 {
		return
	}
	obj := make([]objectiveView, 0, len(progress))
	for _, p := range progress {
		obj = append(obj, objectiveView{Kind: p.Kind, Target: p.Target, Cur: p.Cur, Need: p.Need})
	}
	s.pusher.SendToChar(charID, 11, s.mainSub, map[string]any{
		"quest_id": questID, "state": state, "objectives": obj,
	})
}
