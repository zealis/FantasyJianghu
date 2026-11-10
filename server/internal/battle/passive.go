package battle

import "fantasy-jianghu/server/internal/config"

// PassiveBonus 为心法/绝学被动的汇总（数值规范 §1.3 第 ④ 步来源，万分制）。
//
// 心法被动是**门派身份**：1 级裸装即生效，不依赖装备与品质共鸣。
// 此前引擎完全没读 passive（惊雷宝典的法攻 +3% 一直空转），本轮一并接上。
type PassiveBonus struct {
	PAtkPct      int // 物攻 %
	MAtkPct      int // 法攻 %
	HPPct        int // 生命 %
	PDefPct      int // 物防 %
	MDefPct      int // 法防 %
	HealBonusPct int // 治疗加成 %

	// 概率型（万分制）
	PCritPct   int // 暴击率
	MCritPct   int // 法暴率
	PComboPct  int // 连击率
	SealHitPct int // 封印命中
	PDodgePct  int // 闪避
	CritDmgPct int // 暴击伤害

	// ShockOnHit 表示持有《惊雷宝典》心法：法术命中可施加感电（惊雷殿专属）。
	ShockOnHit bool

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
		out.PAtkPct += p.PAtkPct
		out.MAtkPct += p.MAtkPct
		out.HPPct += p.HPPct
		out.PDefPct += p.PDefPct
		out.MDefPct += p.MDefPct
		out.HealBonusPct += p.HealBonusPct
		out.PCritPct += p.PCritPct
		out.MCritPct += p.MCritPct
		out.PComboPct += p.PComboPct
		out.SealHitPct += p.SealHitPct
		out.PDodgePct += p.PDodgePct
		out.CritDmgPct += p.CritDmgPct
		if p.ShockOnHit {
			out.ShockOnHit = true
		}
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
