package battle

import "fantasy-jianghu/server/internal/config"

// PassiveBonus 为心法/绝学被动的汇总（数值规范 §1.3 第 ④ 步来源，万分制）。
//
// 心法被动是**门派身份**：1 级裸装即生效，不依赖装备与品质共鸣。
// 此前引擎完全没读 passive（惊雷宝典的法攻 +3% 一直空转），本轮一并接上。
type PassiveBonus struct {
	HPPct        int // 生命 %
	PDefPct      int // 物防 %
	MDefPct      int // 法防 %
	MAtkPct      int // 法攻 %
	HealBonusPct int // 治疗加成 %

	// 金刚印（金刚不坏体）：受击叠层，每层反伤 +ReflectPerStack，最多 ReflectMaxStacks 层。
	ReflectPerStack  int
	ReflectMaxStacks int
}

// CollectPassives 汇总角色在本等级已开放的心法/绝学被动。
// 只统计本门派秘籍（多门派秘籍谱合并后必须按 Faction 过滤，否则人人都有金刚印）。
func CollectPassives(skills []config.SkillDef, faction string, level int) PassiveBonus {
	var out PassiveBonus
	for _, sk := range skills {
		if sk.Faction != faction || sk.LevelReq > level || sk.Passive == nil {
			continue
		}
		p := sk.Passive
		out.HPPct += p.HPPct
		out.PDefPct += p.PDefPct
		out.MDefPct += p.MDefPct
		out.MAtkPct += p.MAtkPct
		out.HealBonusPct += p.HealBonusPct
		// 多层反伤被动取最强一档（当前只有金刚不坏体一本，留出扩展余地）。
		if p.ReflectPerStack > out.ReflectPerStack {
			out.ReflectPerStack = p.ReflectPerStack
			out.ReflectMaxStacks = p.ReflectMaxStacks
		}
	}
	return out
}

// ReflectStacksCap 返回金刚印层数上限（无被动则为 0）。
func (p PassiveBonus) ReflectStacksCap() int {
	if p.ReflectPerStack <= 0 {
		return 0
	}
	return p.ReflectMaxStacks
}
