package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BattleFormula 为《战斗公式配置表 V1.0》参数（唯一数值口径，逐字转录）。
// DamageParams 伤害公式参数（公式表 §1）。
type DamageParams struct {
	K                   float64 `json:"K"`
	FloatLow            float64 `json:"float_low"`
	FloatHigh           float64 `json:"float_high"`
	MinDamage           int     `json:"min_damage"`
	LevelDiffPerLevel   float64 `json:"level_diff_per_level"`
	LevelDiffClamp      float64 `json:"level_diff_clamp"`
	ArmorBreakPerStack  float64 `json:"armor_break_per_stack"`
	ArmorBreakMaxStacks int     `json:"armor_break_max_stacks"`
}

// CritParams 暴击（公式表 §2）。
type CritParams struct {
	BaseRate     float64 `json:"base_rate"`
	Damage       float64 `json:"damage"`
	DuanyueBonus float64 `json:"duanyue_bonus"`
}

// HitParams 命中与闪避（公式表 §3）。
type HitParams struct {
	PhysicalBase   float64 `json:"physical_base"`
	ClampLow       float64 `json:"clamp_low"`
	ClampHigh      float64 `json:"clamp_high"`
	MagicDodgeBase float64 `json:"magic_dodge_base"`
}

// ControlParams 控制结算（公式表 §4）。
type ControlParams struct {
	SealResBase  float64  `json:"seal_res_base"`
	ComboDecay   float64  `json:"combo_decay"`
	ImmuneTurns  int      `json:"immune_turns"`
	HardMaxTurns int      `json:"hard_max_turns"`
	HardTypes    []string `json:"hard_types"`
}

// DecayStep 长局衰减（公式表 §6）。
type DecayStep struct {
	From      int     `json:"from"`
	To        int     `json:"to"`
	Dmg       float64 `json:"dmg"`
	Heal      float64 `json:"heal"`
	DmgStep   float64 `json:"dmg_step"`
	HealStep  float64 `json:"heal_step"`
	HealFloor float64 `json:"heal_floor"`
}

// M2a-2 效果参数（数值规范 §2.4 反伤体系、§3 DOT 不走命中；首版参数，登记待回归校准）。
// 百分比一律万分制整数，禁止浮点落库/落配置。
type ShockParams struct {
	MaxStacks            int `json:"max_stacks"`
	DmgTakenPctPerStack  int `json:"dmg_taken_pct_per_stack"`   // 每层受雷法伤害 +4%
	DOTPctOfMAtkPerStack int `json:"dot_pct_of_matk_per_stack"` // 每层每回合 DOT = 施加者法攻 × 3%
	DOTTurns             int `json:"dot_turns"`
	BonusVsShockedPct    int `json:"bonus_vs_shocked_pct"` // shock_bonus_10 秘籍额外 +10%
}

type ReflectParams struct {
	PerStackPct int `json:"per_stack_pct"` // 每层反伤 +3%
	MaxPct      int `json:"max_pct"`
	MinDamage   int `json:"min_damage"`
}

type DotParams struct {
	MinDamage int `json:"min_damage"`
}

type EffectsParams struct {
	Shock   ShockParams   `json:"shock"`
	Reflect ReflectParams `json:"reflect"`
	Dot     DotParams     `json:"dot"`
}

// DrillParams 为 4v4 演练阵容（配置驱动，代码不写死数值/成员）。
// 演练场用固定 level 派生双方属性，避免低等级玩家被高等级怪物模板秒杀。
type DrillParams struct {
	Level                 int      `json:"level"`
	Allies                int      `json:"allies"`
	Enemies               []string `json:"enemies"`
	AllyHPPct             int      `json:"ally_hp_pct"`
	AllyAtkPct            int      `json:"ally_atk_pct"`
	AllySpeedPct          int      `json:"ally_speed_pct"`
	EnemyHPPctOfPlayer    int      `json:"enemy_hp_pct_of_player"`
	EnemyAtkPctOfPlayer   int      `json:"enemy_atk_pct_of_player"`
	EnemyDefPctOfPlayer   int      `json:"enemy_def_pct_of_player"`
	EnemySpeedPctOfPlayer int      `json:"enemy_speed_pct_of_player"`
}

// CoeffCaps 为角色级系数上限（数值规范 §9.7：反伤 ≤50%、治疗加成 ≤60%，入配置可调）。
// 万分制整数；0 表示该项不裁剪。
type CoeffCaps struct {
	ReflectPct   int `json:"reflect_cap_pct"`
	HealBonusPct int `json:"heal_bonus_cap_pct"`
}

type BattleFormula struct {
	Damage    DamageParams `json:"damage"`
	CoeffCaps CoeffCaps    `json:"coeff_caps"`
	Crit      CritParams   `json:"crit"`
	Combo     struct {
		ExtraMult float64 `json:"extra_mult"`
	} `json:"combo"`
	MultiplierCap     float64       `json:"multiplier_cap"`
	Hit               HitParams     `json:"hit"`
	Control           ControlParams `json:"control"`
	Decay             []DecayStep   `json:"decay"`
	SpeedJitter       float64       `json:"speed_jitter"`
	MaxTurns          int           `json:"max_turns"`
	CmdTimeoutSeconds int           `json:"cmd_timeout_seconds"`
	Effects           EffectsParams `json:"effects"`
	Drill             DrillParams   `json:"drill"`
	SpecialsDeferred  []string      `json:"specials_deferred"`
}

// BattleClass 为门派战斗定位与锚点（数值规范 §5.2 逐字 + 肉盾派生系数 §5.3）。
// AnchorStat 为定位锚点（数值规范 §5.2 表行）。
type AnchorStat struct {
	HP    int `json:"hp"`
	MP    int `json:"mp"`
	PAtk  int `json:"patk"`
	MAtk  int `json:"matk"`
	PDef  int `json:"pdef"`
	MDef  int `json:"mdef"`
	Speed int `json:"speed"`
}

type BattleClass struct {
	Class      string                `json:"class"`
	Factions   []string              `json:"factions"`
	Anchors    map[string]AnchorStat `json:"anchors"`
	DeriveFrom string                `json:"derive_from"` // 肉盾派生来源锚点
	Coeff      map[string]float64    `json:"coeff"`       // 派生系数
}

// ClassBook 为 classes.json 顶层结构（含锚点外延曲线参数，助手定案首版）。
type ClassBook struct {
	Classes        []BattleClass `json:"classes"`
	LowCurveExp    float64       `json:"low_curve_exp"`
	LowCurveBaseLv int           `json:"low_curve_base_level"`
	PointsPerLevel int           `json:"points_per_level"`
}

// SkillSegment 为秘籍一段（多段逐段独立结算）。
type SkillSegment struct {
	Mult float64 `json:"mult"` // 百分比（如 160 = 160%）
}

// SkillDef 为秘籍定义（秘籍配置表数据驱动；引擎不写死门派）。
type SkillDef struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Category      string         `json:"category"` // 心法|单体|群体|控制|辅助|绝学
	LevelReq      int            `json:"level_req"`
	MPCost        int            `json:"mp_cost"`
	AttackType    string         `json:"attack_type"` // physical|magic
	Segments      []SkillSegment `json:"segments"`
	Targets       int            `json:"targets"`
	TargetsGrowth *struct {
		Level   int `json:"level"`
		Targets int `json:"targets"`
	} `json:"targets_growth"`
	Control *struct {
		Type     string  `json:"type"`
		Rate     float64 `json:"rate"` // 百分比
		Duration int     `json:"duration"`
	} `json:"control"`
	BuffSelf *struct {
		Stat  string `json:"stat"`
		Value int    `json:"value"` // 万分制
		Turns int    `json:"turns"`
	} `json:"buff_self"`
	BuffGroup bool        `json:"buff_group"`
	Passive   *PassiveDef `json:"passive"`
	Special   string      `json:"special"` // 特效标记；须匹配 Formula.SpecialsDeferred 或已实现集合
	// ConsumesStar 标记「释放时消耗星辉」的攻击秘籍：该类秘籍伤害吃角色级蓄力伤害系数
	// （数值规范 §9.7 蓄力钩子）。不消耗星辉的秘籍不适用。
	ConsumesStar bool `json:"consumes_star"`
	// Faction 为秘籍谱所属门派（由 skills_<门派>.json 文件名/内容注入，非手写字段）。
	Faction string `json:"-"`
}

// PassiveDef 为心法/绝学被动（数值规范 §1.3 第 ④ 步来源之一，万分制）。
// 心法被动是**门派身份**：1 级裸装即可生效，不依赖装备与共鸣。
type PassiveDef struct {
	MAtkPct      int `json:"matk_pct"`       // 法攻 +%
	HPPct        int `json:"hp_pct"`         // 生命 +%
	PDefPct      int `json:"pdef_pct"`       // 物防 +%
	MDefPct      int `json:"mdef_pct"`       // 法防 +%
	HealBonusPct int `json:"heal_bonus_pct"` // 治疗加成 +%（玄水心法）

	// 金刚印：受击叠层，每层反伤 +reflect_per_stack，最多 reflect_max_stacks 层。
	// 禅院「挨打就有反伤」的来源，与品质共鸣（微调）分列，互不替代。
	ReflectPerStack  int `json:"reflect_per_stack"`
	ReflectMaxStacks int `json:"reflect_max_stacks"`
}

// MonsterDef 为野怪模板（M2a 首版手写数值，登记助手定案）。
type MonsterDef struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Level      int          `json:"level"`
	Class      string       `json:"class"`
	Stats      UnitStatsDef `json:"stats"`
	Skills     []string     `json:"skills"`
	ReflectPct int          `json:"reflect_pct"` // 万分制；M2a-2 反伤体系（§2.4）
}

type UnitStatsDef struct {
	HP    int `json:"hp"`
	MP    int `json:"mp"`
	PAtk  int `json:"patk"`
	MAtk  int `json:"matk"`
	PDef  int `json:"pdef"`
	MDef  int `json:"mdef"`
	Speed int `json:"speed"`
}

// BattleData 为战斗域配置聚合。
type BattleData struct {
	Formula   BattleFormula
	ClassBook ClassBook
	Classes   []BattleClass
	Skills    []SkillDef
	Monsters  []MonsterDef
}

// SkillByID 按秘籍 ID 查找。
func (b BattleData) SkillByID(id string) *SkillDef {
	for i := range b.Skills {
		if b.Skills[i].ID == id {
			return &b.Skills[i]
		}
	}
	return nil
}

// ClassByFaction 按门派查战斗定位。
func (b BattleData) ClassByFaction(faction string) *BattleClass {
	for i := range b.Classes {
		for _, f := range b.Classes[i].Factions {
			if f == faction {
				return &b.Classes[i]
			}
		}
	}
	return nil
}

// specialDeferred 判断特效是否登记延后（支持前缀通配 "shock_*"）。
func (f *BattleFormula) specialDeferred(special string) bool {
	for _, p := range f.SpecialsDeferred {
		if p == special {
			return true
		}
		if strings.HasSuffix(p, "*") && strings.HasPrefix(special, strings.TrimSuffix(p, "*")) {
			return true
		}
	}
	return false
}

// implementedSpecials 为引擎已实现特效集合。未实现且未登记延后的特效拒载（配置驱动，禁止静默）。
var implementedSpecials = map[string]bool{
	"shock_bonus_10": true, // M2a-2：怒雷天罚对感电目标额外伤害
}

func validateBattle(b BattleData) error {
	if b.Formula.Damage.K != 1.0 {
		return fmt.Errorf("公式 K = %v，回归锁定值 1.0，禁止漂移", b.Formula.Damage.K)
	}
	if b.Formula.MultiplierCap <= 1.0 || b.Formula.MultiplierCap > 2.25 {
		return fmt.Errorf("乘区上限 %v 超红线（§6：≤2.25）", b.Formula.MultiplierCap)
	}
	if b.Formula.Control.ComboDecay <= 0 || b.Formula.Control.ComboDecay >= 1 {
		return fmt.Errorf("连控递减 %v 须在 (0,1)", b.Formula.Control.ComboDecay)
	}
	if b.Formula.MaxTurns <= 0 || b.Formula.CmdTimeoutSeconds <= 0 {
		return fmt.Errorf("回合上限/指令超时必须为正")
	}
	// M2a-2 效果参数门禁（首版参数，非法即停不回退，§7）。
	e := b.Formula.Effects
	if e.Shock.MaxStacks <= 0 || e.Shock.MaxStacks > 10 {
		return fmt.Errorf("感电层数上限 %d 非法", e.Shock.MaxStacks)
	}
	if e.Shock.DmgTakenPctPerStack < 0 || e.Shock.DOTPctOfMAtkPerStack < 0 || e.Shock.DOTTurns <= 0 || e.Shock.BonusVsShockedPct < 0 {
		return fmt.Errorf("感电参数非法: %+v", e.Shock)
	}
	if e.Reflect.PerStackPct < 0 || e.Reflect.MaxPct < 0 || e.Reflect.MaxPct > 5000 || e.Reflect.MinDamage <= 0 {
		return fmt.Errorf("反伤参数非法: %+v", e.Reflect)
	}
	if e.Dot.MinDamage <= 0 {
		return fmt.Errorf("DOT 伤害下限必须为正")
	}
	// 角色级系数上限（§9.7）：万分制、0≤x≤100%，首版反伤 ≤50%、治疗 ≤60%。
	caps := b.Formula.CoeffCaps
	if caps.ReflectPct < 0 || caps.ReflectPct > 10000 {
		return fmt.Errorf("反伤系数上限 %d 非法（万分制 0..10000）", caps.ReflectPct)
	}
	if caps.HealBonusPct < 0 || caps.HealBonusPct > 10000 {
		return fmt.Errorf("治疗加成上限 %d 非法（万分制 0..10000）", caps.HealBonusPct)
	}
	d := b.Formula.Drill
	if d.Level <= 0 || d.Allies < 0 || d.Allies > 3 || len(d.Enemies) == 0 || len(d.Enemies) > 4 {
		return fmt.Errorf("4v4 演练阵容规模/等级非法: %+v", d)
	}
	for _, pct := range []int{d.AllyHPPct, d.AllyAtkPct, d.AllySpeedPct,
		d.EnemyHPPctOfPlayer, d.EnemyAtkPctOfPlayer, d.EnemyDefPctOfPlayer, d.EnemySpeedPctOfPlayer} {
		if pct <= 0 {
			return fmt.Errorf("演练缩放系数必须为正: %+v", d)
		}
	}
	for _, mid := range d.Enemies {
		found := false
		for _, m := range b.Monsters {
			if m.ID == mid {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("演练阵容引用不存在野怪 %q", mid)
		}
	}
	cats := map[string]bool{"心法": true, "单体": true, "群体": true, "控制": true, "辅助": true, "绝学": true}
	for _, sk := range b.Skills {
		if !cats[sk.Category] {
			return fmt.Errorf("秘籍 %s 类别非法: %q", sk.ID, sk.Category)
		}
		if sk.Special != "" && !implementedSpecials[sk.Special] && !b.Formula.specialDeferred(sk.Special) {
			return fmt.Errorf("秘籍 %s 特效 %q 未实现且未登记延后（配置驱动，禁止静默）", sk.ID, sk.Special)
		}
		if sk.Control != nil && sk.Control.Duration > b.Formula.Control.HardMaxTurns && isHardControl(sk.Control.Type, b.Formula.Control.HardTypes) {
			return fmt.Errorf("秘籍 %s 硬控时长 %d 超上限 %d", sk.ID, sk.Control.Duration, b.Formula.Control.HardMaxTurns)
		}
	}
	if len(b.Skills) == 0 {
		return fmt.Errorf("秘籍表不得为空")
	}
	factionSet := map[string]bool{}
	for _, c := range b.Classes {
		for _, f := range c.Factions {
			factionSet[f] = true
		}
	}
	for _, sk := range b.Skills {
		if sk.Faction == "" {
			return fmt.Errorf("秘籍 %s 缺所属门派", sk.ID)
		}
		if len(factionSet) > 0 && !factionSet[sk.Faction] {
			return fmt.Errorf("秘籍 %s 门派 %q 未在定位表中登记", sk.ID, sk.Faction)
		}
	}
	seenSkill := map[string]bool{}
	for _, sk := range b.Skills {
		if seenSkill[sk.ID] {
			return fmt.Errorf("秘籍 id 重复: %s（多门派秘籍谱合并后必须全局唯一）", sk.ID)
		}
		seenSkill[sk.ID] = true
		if p := sk.Passive; p != nil {
			for name, v := range map[string]int{
				"matk_pct": p.MAtkPct, "hp_pct": p.HPPct, "pdef_pct": p.PDefPct,
				"mdef_pct": p.MDefPct, "heal_bonus_pct": p.HealBonusPct,
				"reflect_per_stack": p.ReflectPerStack,
			} {
				if v < 0 || v > 10000 {
					return fmt.Errorf("秘籍 %s 被动 %s=%d 非法（万分制 0..10000）", sk.ID, name, v)
				}
			}
			if p.ReflectMaxStacks < 0 || p.ReflectMaxStacks > 10 {
				return fmt.Errorf("秘籍 %s 金刚印层数上限 %d 非法（0..10）", sk.ID, p.ReflectMaxStacks)
			}
			if p.ReflectPerStack > 0 && p.ReflectMaxStacks == 0 {
				return fmt.Errorf("秘籍 %s 配了每层反伤但层数上限为 0", sk.ID)
			}
			if p.ReflectPerStack*p.ReflectMaxStacks > 15000 {
				return fmt.Errorf("秘籍 %s 金刚印反伤上限 %d 超 150%%（万分制 15000）",
					sk.ID, p.ReflectPerStack*p.ReflectMaxStacks)
			}
		}
	}
	factionOf := map[string]bool{}
	for _, c := range b.Classes {
		for _, f := range c.Factions {
			factionOf[f] = true
		}
		if len(c.Anchors) == 0 && c.DeriveFrom == "" {
			return fmt.Errorf("定位 %s 缺锚点且无派生来源", c.Class)
		}
	}
	for _, m := range b.Monsters {
		for _, sid := range m.Skills {
			if b.SkillByID(sid) == nil {
				return fmt.Errorf("怪物 %s 引用不存在秘籍 %q", m.ID, sid)
			}
		}
		if m.ReflectPct < 0 || m.ReflectPct > e.Reflect.MaxPct {
			return fmt.Errorf("怪物 %s 反伤 %d 超上限 %d", m.ID, m.ReflectPct, e.Reflect.MaxPct)
		}
	}
	return nil
}

func isHardControl(t string, hard []string) bool {
	for _, h := range hard {
		if h == t {
			return true
		}
	}
	return false
}

// loadBattleDir 从配置包 battle/ 子目录解析（哈希复核已在 Load 主体完成）。
func loadBattleDir(dir string) (BattleData, error) {
	var b BattleData
	skillDir := filepath.Join(dir, "battle")
	read := func(name string, v any) error {
		raw, err := os.ReadFile(filepath.Join(dir, "battle", name))
		if err != nil {
			return fmt.Errorf("读 battle/%s: %w", name, err)
		}
		return unmarshalStrict(raw, v)
	}
	if err := read("formula.json", &b.Formula); err != nil {
		return b, err
	}
	if err := read("classes.json", &b.ClassBook); err != nil {
		return b, err
	}
	b.Classes = b.ClassBook.Classes
	// 秘籍谱按门派分文件（skills_<门派>.json）：八派各一份，合并后按缺省门派过滤使用。
	skills, err := loadSkillFiles(skillDir)
	if err != nil {
		return b, err
	}
	b.Skills = skills
	var monsterBook struct {
		Monsters []MonsterDef `json:"monsters"`
	}
	if err := read("monsters.json", &monsterBook); err != nil {
		return b, err
	}
	b.Monsters = monsterBook.Monsters
	if err := validateBattle(b); err != nil {
		return BattleData{}, fmt.Errorf("战斗配置复核不通过: %w", err)
	}
	return b, nil
}

// packBattleDir 打包战斗域源目录（configtool 调用）。
func packBattleDir(mf *Manifest, target, source string) error {
	battleDir := filepath.Join(source, "battle")
	entries, err := os.ReadDir(battleDir)
	if err != nil {
		return fmt.Errorf("读 battle 源目录: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(battleDir, e.Name()))
		if err != nil {
			return err
		}
		if err := appendPacked(mf, target, "battle/"+e.Name(), raw); err != nil {
			return err
		}
	}
	return nil
}

// loadSkillFiles 读取 battle/skills_*.json 全部门派秘籍谱并合并为一个列表，
// 同时把文件声明的门派写进每本秘籍的 Faction（供按角色门派过滤）。
// 单文件时代遗留：此前只认惊雷殿一份，导致别的门派在战斗里拿到雷系秘籍。
func loadSkillFiles(battleDir string) ([]SkillDef, error) {
	entries, err := os.ReadDir(battleDir)
	if err != nil {
		return nil, fmt.Errorf("读 battle 目录: %w", err)
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, "skills_") || !strings.HasSuffix(n, ".json") {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("battle 目录无 skills_*.json 秘籍谱")
	}
	var out []SkillDef
	for _, n := range names {
		raw, err := os.ReadFile(filepath.Join(battleDir, n))
		if err != nil {
			return nil, fmt.Errorf("读 battle/%s: %w", n, err)
		}
		var book struct {
			Faction string     `json:"faction"`
			Source  string     `json:"source"` // 文档出处备注（仅存档，不参与结算）
			Skills  []SkillDef `json:"skills"`
		}
		if err := unmarshalStrict(raw, &book); err != nil {
			return nil, fmt.Errorf("解析 battle/%s: %w", n, err)
		}
		if book.Faction == "" {
			return nil, fmt.Errorf("battle/%s 缺 faction 字段（秘籍谱须声明所属门派）", n)
		}
		for i := range book.Skills {
			book.Skills[i].Faction = book.Faction
		}
		out = append(out, book.Skills...)
	}
	return out, nil
}

// loadBattleDirFromData 直接从源目录解析战斗域（Pack 校验用，绕过哈希复核）。
func loadBattleDirFromData(source string) (BattleData, error) {
	return loadBattleDirRaw(filepath.Join(source, "battle"))
}

func loadBattleDirRaw(battleDir string) (BattleData, error) {
	var b BattleData
	skillDir := battleDir
	read := func(name string, v any) error {
		raw, err := os.ReadFile(filepath.Join(battleDir, name))
		if err != nil {
			return fmt.Errorf("读 battle/%s: %w", name, err)
		}
		return unmarshalStrictRaw(raw, v)
	}
	if err := read("formula.json", &b.Formula); err != nil {
		return b, err
	}
	if err := read("classes.json", &b.ClassBook); err != nil {
		return b, err
	}
	b.Classes = b.ClassBook.Classes
	// 秘籍谱按门派分文件（skills_<门派>.json）：八派各一份，合并后按缺省门派过滤使用。
	skills, err := loadSkillFiles(skillDir)
	if err != nil {
		return b, err
	}
	b.Skills = skills
	var monsterBook struct {
		Monsters []MonsterDef `json:"monsters"`
	}
	if err := read("monsters.json", &monsterBook); err != nil {
		return b, err
	}
	b.Monsters = monsterBook.Monsters
	if err := validateBattle(b); err != nil {
		return BattleData{}, fmt.Errorf("战斗配置复核不通过: %w", err)
	}
	return b, nil
}

func unmarshalStrictRaw(b []byte, v any) error {
	return unmarshalStrict(b, v)
}
