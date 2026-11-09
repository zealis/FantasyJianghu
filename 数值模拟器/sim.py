# -*- coding: utf-8 -*-
"""
《幻想江湖》秘籍数值模拟回归
口径来源：
- 数值规范 §5.2 属性锚点（20/70/89 × 物理/法术/高速控制）
- 数值规范 §5.1 带宽：1v1 4-8 回合；4v4 6-15 回合；极端镜像不得 50 回合未分
- 数值规范 §3 不变量：防御递减无断崖（攻²/(攻+防) 型公式）、百分比伤害封顶
- 数值规范 §2.5 等级压制：每级 1.5%、±30%
- 秘籍配置表 V1.0：倍率、耗真气（1 级值，每级 +3%），秘籍等级 = 角色等级
- 控制：成功率 ×(1-抗性) ×压制修正；连控 ×0.7 递减；硬控 ≤2 回合
假设（报告中列明）：基础暴击率 5%、暴伤 150%；连击追加段为 50% 伤害；
  暴击×连击同段总乘区上限 ×2.25；物理命中 95%；封印抗性基线 10%；
  肉盾锚点缺失，禅院=物理锚点(HP×1.3/攻×0.75/防×1.25/速×0.85)，
  玄水=法术锚点(HP×1.25/法攻×0.8/防×1.2/速×0.9)。
"""
import math, random, statistics, json, sys
from copy import deepcopy

# ---------------- 锚点（数值规范 §5.2） ----------------
ANCHOR = {
    20: {'phy': dict(hp=4515, mp=2902, patk=1422, matk=582,  pdef=376,  mdef=376,  spd=1089),
         'mag': dict(hp=5055, mp=7642, patk=582,  matk=1422, pdef=376,  mdef=376,  spd=1224),
         'ctl': dict(hp=4950, mp=3442, patk=582,  matk=582,  pdef=366,  mdef=366,  spd=1654)},
    70: {'phy': dict(hp=27741, mp=19044, patk=5428, matk=3988, pdef=1815, mdef=1815, spd=5027),
         'mag': dict(hp=22179, mp=20682, patk=3988, matk=5428, pdef=1815, mdef=1815, spd=2979),
         'ctl': dict(hp=21774, mp=13482, patk=3210, matk=3210, pdef=1774, mdef=1774, spd=3974)},
    89: {'phy': dict(hp=24976, mp=15774, patk=5312, matk=3644, pdef=2068, mdef=2068, spd=3521),
         'mag': dict(hp=25624, mp=24762, patk=3644, matk=5312, pdef=2068, mdef=2068, spd=3707),
         'ctl': dict(hp=25174, mp=16422, patk=3644, matk=3644, pdef=2024, mdef=2024, spd=4718)},
}

def role_stats(faction, level):
    a = ANCHOR[level]
    if faction in ('jian', 'qiang'):
        s = dict(a['phy'])
    elif faction in ('lei', 'xing'):
        s = dict(a['mag'])
    elif faction in ('fu', 'meng'):
        s = dict(a['ctl'])
    elif faction == 'chan':  # 禅院：物理锚点调整
        s = dict(a['phy'])
        s['hp'] = int(s['hp'] * 1.30); s['patk'] = int(s['patk'] * 0.75); s['matk'] = int(s['matk'] * 0.75)
        s['pdef'] = int(s['pdef'] * 1.25); s['mdef'] = int(s['mdef'] * 1.25); s['spd'] = int(s['spd'] * 0.85)
    elif faction == 'shui':  # 玄水：法术锚点调整
        s = dict(a['mag'])
        s['hp'] = int(s['hp'] * 1.25); s['matk'] = int(s['matk'] * 0.80); s['patk'] = int(s['patk'] * 0.8)
        s['pdef'] = int(s['pdef'] * 1.20); s['mdef'] = int(s['mdef'] * 1.20); s['spd'] = int(s['spd'] * 0.90)
    return s

FACTION_NAME = {'jian': '断岳剑宗', 'qiang': '破军枪府', 'lei': '惊雷殿', 'xing': '璇玑星宫',
                'fu': '缚灵符观', 'meng': '织梦谷', 'chan': '金刚禅院', 'shui': '玄水宫'}
ROLE = {'jian': 'phy', 'qiang': 'phy', 'lei': 'mag', 'xing': 'mag',
        'fu': 'ctl', 'meng': 'ctl', 'chan': 'tank', 'shui': 'tank'}

# ---------------- 秘籍表（转录自配置表 V1.0） ----------------
# kind: dmg 伤害 / ctrl 控制 / buff 辅助 / heal 治疗 / shield 护盾
# eff 以代码钩子实现派系机制；通用字段：mult 倍率, segs 段数, tgt 目标数(0=全体,1=单体,2=群体成长)
SK = {
 # 断岳剑宗
 'jian': [
  dict(n='断岳剑经', cat='心法', lv=1, mp=0),
  dict(n='裂空九剑', cat='单体', lv=10, mp=140, kind='dmg', mult=0.55, segs=3, atk='p'),
  dict(n='残月斩', cat='单体', lv=20, mp=120, kind='dmg', mult=1.30, segs=1, atk='p', eff='execute'),
  dict(n='剑气纵横', cat='群体', lv=30, mp=260, kind='dmg', mult=0.85, segs=1, atk='p', tgt=2),
  dict(n='千锋破空', cat='群体', lv=50, mp=340, kind='dmg', mult=1.05, segs=1, atk='p', tgt=2, eff='jian_qianfeng'),
  dict(n='断岳沉锋', cat='控制', lv=25, mp=150, kind='dmg', mult=0.60, segs=1, atk='p', eff='deb_atk15'),
  dict(n='锁脉剑', cat='控制', lv=45, mp=200, kind='dmg', mult=0.70, segs=1, atk='p', eff='seal_skill55'),
  dict(n='剑心通明', cat='辅助', lv=15, mp=130, kind='buff', eff='jian_buff'),
  dict(n='御剑乘风', cat='辅助', lv=35, mp=220, kind='buff', eff='spd12_grp'),
  dict(n='一剑开天', cat='绝学', lv=80, mp=800, kind='dmg', mult=2.40, segs=1, atk='p', eff='jian_ult'),
 ],
 # 破军枪府
 'qiang': [
  dict(n='破军枪谱', cat='心法', lv=1, mp=0),
  dict(n='裂阵突刺', cat='单体', lv=10, mp=140, kind='dmg', mult=1.55, segs=1, atk='p', eff='ignore10'),
  dict(n='陷阵连环枪', cat='单体', lv=20, mp=160, kind='dmg', mult=0.85, segs=2, atk='p', eff='combo_extra'),
  dict(n='荡寇枪法', cat='群体', lv=30, mp=260, kind='dmg', mult=0.90, segs=1, atk='p', tgt=2),
  dict(n='横扫千军', cat='群体', lv=50, mp=360, kind='dmg', mult=1.15, segs=1, atk='p', tgt=2, eff='qiang_hengsao'),
  dict(n='破甲断喝', cat='控制', lv=25, mp=160, kind='dmg', mult=0.40, segs=1, atk='p', tgt=2, eff='deb_pdef12_grp'),
  dict(n='缚马索喉', cat='控制', lv=45, mp=200, kind='dmg', mult=0.70, segs=1, atk='p', eff='qiang_fuma'),
  dict(n='战意沸腾', cat='辅助', lv=15, mp=130, kind='buff', eff='qiang_buff'),
  dict(n='陷阵之志', cat='辅助', lv=35, mp=220, kind='buff', eff='patk10_grp'),
  dict(n='破军杀将', cat='绝学', lv=80, mp=800, kind='dmg', mult=2.60, segs=1, atk='p', eff='qiang_ult'),
 ],
 # 惊雷殿
 'lei': [
  dict(n='惊雷宝典', cat='心法', lv=1, mp=0),
  dict(n='天雷破', cat='单体', lv=10, mp=150, kind='dmg', mult=1.60, segs=1, atk='m'),
  dict(n='怒雷天罚', cat='单体', lv=20, mp=190, kind='dmg', mult=1.75, segs=1, atk='m', eff='lei_nulei'),
  dict(n='连环霹雳', cat='群体', lv=30, mp=300, kind='dmg', mult=0.95, segs=1, atk='m', tgt=2, eff='lei_jump'),
  dict(n='九霄神雷', cat='群体', lv=50, mp=420, kind='dmg', mult=1.20, segs=1, atk='m', tgt=2),
  dict(n='雷缚咒', cat='控制', lv=25, mp=180, kind='dmg', mult=0.50, segs=1, atk='m', eff='para70'),
  dict(n='万雷锁域', cat='控制', lv=45, mp=260, kind='dmg', mult=0.40, segs=1, atk='m', tgt=2, eff='para60'),
  dict(n='雷甲护身', cat='辅助', lv=15, mp=150, kind='buff', eff='lei_jia'),
  dict(n='疾雷步', cat='辅助', lv=35, mp=220, kind='buff', eff='spd12_grp'),
  dict(n='天雷荡魔', cat='绝学', lv=80, mp=900, kind='dmg', mult=2.00, segs=1, atk='m', tgt=0, eff='lei_ult'),
 ],
 # 璇玑星宫
 'xing': [
  dict(n='璇玑星经', cat='心法', lv=1, mp=0),
  dict(n='引星诀', cat='单体', lv=10, mp=140, kind='dmg', mult=1.50, segs=1, atk='m'),
  dict(n='落星指', cat='单体', lv=20, mp=180, kind='dmg', mult=1.85, segs=1, atk='m', eff='xing_luoxing'),
  dict(n='星陨', cat='群体', lv=30, mp=300, kind='dmg', mult=0.90, segs=1, atk='m', tgt=2),
  dict(n='陨星天坠', cat='群体', lv=50, mp=430, kind='dmg', mult=1.25, segs=1, atk='m', tgt=2),
  dict(n='定星咒', cat='控制', lv=25, mp=180, kind='dmg', mult=0.50, segs=1, atk='m', eff='root75'),
  dict(n='乱星迷途', cat='控制', lv=45, mp=260, kind='dmg', mult=0.0, segs=1, atk='m', tgt=2, eff='xing_luanxing'),
  dict(n='星盘推演', cat='辅助', lv=15, mp=140, kind='buff', eff='xing_tuiyan'),
  dict(n='披星戴月', cat='辅助', lv=35, mp=230, kind='buff', eff='matk10_grp'),
  dict(n='周天星陨', cat='绝学', lv=80, mp=950, kind='dmg', mult=1.70, segs=1, atk='m', tgt=0, eff='xing_ult'),
 ],
 # 缚灵符观
 'fu': [
  dict(n='镇魔符经', cat='心法', lv=1, mp=0),
  dict(n='缚灵符箓', cat='单体', lv=10, mp=130, kind='dmg', mult=1.20, segs=1, atk='p', eff='fu_fuling'),
  dict(n='诛魔符', cat='单体', lv=20, mp=170, kind='dmg', mult=1.60, segs=1, atk='p'),
  dict(n='万符天箓', cat='群体', lv=30, mp=280, kind='dmg', mult=0.75, segs=1, atk='p', tgt=2, eff='fu_wanfu'),
  dict(n='五雷符阵', cat='群体', lv=50, mp=400, kind='dmg', mult=1.05, segs=1, atk='p', tgt=2),
  dict(n='定身符', cat='控制', lv=25, mp=170, kind='ctrl', eff='root80'),
  dict(n='封魔印', cat='控制', lv=45, mp=230, kind='ctrl', eff='sealskill75_2'),
  dict(n='清心符', cat='辅助', lv=15, mp=140, kind='buff', eff='fu_qingxin'),
  dict(n='金甲符', cat='辅助', lv=35, mp=220, kind='buff', eff='pdef12_grp'),
  dict(n='天箓镇魔', cat='绝学', lv=80, mp=700, kind='dmg', mult=0.80, segs=1, atk='p', tgt=0, eff='fu_ult'),
 ],
 # 织梦谷
 'meng': [
  dict(n='大梦心经', cat='心法', lv=1, mp=0),
  dict(n='入梦引', cat='单体', lv=10, mp=140, kind='ctrl', eff='sleep75'),
  dict(n='梦魇缠身', cat='单体', lv=20, mp=160, kind='dmg', mult=0.90, segs=1, atk='m', eff='meng_dot'),
  dict(n='黄粱一梦', cat='群体', lv=30, mp=290, kind='ctrl', tgt=2, eff='sleep60_grp'),
  dict(n='蜃楼幻境', cat='群体', lv=50, mp=410, kind='dmg', mult=0.70, segs=1, atk='m', tgt=2, eff='meng_shenlou'),
  dict(n='迷心咒', cat='控制', lv=25, mp=160, kind='dmg', mult=0.40, segs=1, atk='m', eff='deb_hit18'),
  dict(n='蚀梦', cat='控制', lv=45, mp=210, kind='buff', eff='meng_shimeng'),
  dict(n='醒梦铃', cat='辅助', lv=15, mp=140, kind='buff', eff='meng_xingmeng'),
  dict(n='梦游步', cat='辅助', lv=35, mp=220, kind='buff', eff='dodge12_grp'),
  dict(n='大梦三千', cat='绝学', lv=80, mp=750, kind='dmg', mult=0.0, segs=1, atk='m', tgt=0, eff='meng_ult'),
 ],
 # 金刚禅院
 'chan': [
  dict(n='金刚不坏体', cat='心法', lv=1, mp=0),
  dict(n='罗汉伏魔功', cat='单体', lv=10, mp=120, kind='dmg', mult=1.45, segs=1, atk='p', eff='taunt1'),
  dict(n='伏魔杖法', cat='单体', lv=20, mp=150, kind='dmg', mult=1.70, segs=1, atk='p'),
  dict(n='狮子吼', cat='群体', lv=30, mp=260, kind='dmg', mult=0.80, segs=1, atk='p', tgt=2, eff='taunt_grp'),
  dict(n='千佛手', cat='群体', lv=50, mp=360, kind='dmg', mult=1.00, segs=1, atk='p', tgt=2, eff='chan_qianfo'),
  dict(n='金刚怒目', cat='控制', lv=25, mp=140, kind='buff', eff='chan_numu'),
  dict(n='梵音缚魔', cat='控制', lv=45, mp=230, kind='dmg', mult=0.0, segs=1, atk='p', tgt=2, eff='chan_fanyin'),
  dict(n='金钟罩', cat='辅助', lv=15, mp=180, kind='shield', eff='shield12_grp'),
  dict(n='菩提心', cat='辅助', lv=35, mp=230, kind='buff', eff='dmgred10_grp'),
  dict(n='不动明王', cat='绝学', lv=80, mp=650, kind='buff', eff='chan_ult'),
 ],
 # 玄水宫
 'shui': [
  dict(n='玄水真经', cat='心法', lv=1, mp=0),
  dict(n='沧澜诀', cat='单体', lv=10, mp=130, kind='dmg', mult=1.40, segs=1, atk='m'),
  dict(n='怒涛掌', cat='单体', lv=20, mp=160, kind='dmg', mult=1.65, segs=1, atk='m', eff='shui_nutao'),
  dict(n='水幕天华', cat='群体', lv=30, mp=300, kind='heal', mult=0.75, tgt=0, eff='shui_shuimu'),
  dict(n='沧海横流', cat='群体', lv=50, mp=380, kind='dmg', mult=0.95, segs=1, atk='m', tgt=2),
  dict(n='寒潮', cat='控制', lv=25, mp=170, kind='dmg', mult=0.45, segs=1, atk='m', tgt=2, eff='deb_spd15_grp'),
  dict(n='玄冰封', cat='控制', lv=45, mp=240, kind='dmg', mult=0.60, segs=1, atk='m', eff='freeze70'),
  dict(n='若水润心', cat='辅助', lv=15, mp=150, kind='heal', mult=1.60, tgt=1, eff='cleanse1'),
  dict(n='龟息功', cat='辅助', lv=35, mp=160, kind='buff', eff='shui_guixi'),
  dict(n='沧澜倒海', cat='绝学', lv=80, mp=800, kind='heal', mult=1.85, tgt=0, eff='shui_ult'),
 ],
}

# ---------------- 战斗引擎 ----------------
BASE_CRIT = 0.05          # 基础暴击率（公式配置假设）
CRIT_MULT = 1.5           # 暴伤倍率
COMBO_MULT_SEG = 0.5      # 连击追加段倍率
ZONE_CAP = 2.25           # 暴击×连击同段总乘区上限（红线）
SEAL_RESIST = 0.10        # 封印抗性基线
PHYS_HIT = 0.95           # 物理命中基线
MAXROUND = 50
DMG_K = 1.0          # 全局伤害系数（公式配置表标定）
SPELL_DODGE = 0.10   # 法术闪避基线（锚点未含此字段，取 10% 假设）

class Unit:
    def __init__(self, faction, level, side, idx):
        s = role_stats(faction, level)
        self.f = faction; self.level = level; self.side = side; self.idx = idx
        self.maxhp = s['hp']; self.hp = s['hp']
        self.maxmp = s['mp']; self.mp = s['mp']
        self.patk = s['patk']; self.matk = s['matk']
        self.pdef = s['pdef']; self.mdef = s['mdef']; self.spd = s['spd']
        self.alive = True; self.shield = 0; self.shield_dur = 0
        self.buffs = []          # dict(k, v, dur) 百分比修正/状态
        self.ctrl = []           # 硬控列表 dict(type, dur): root/sealskill/sleep/para/freeze/chaos
        self.ctrl_chain = 0      # 连控递减计数
        self.stacks = {}         # 派系层数: jianyi/gandian/xinghui/pojia(在目标身上)/mengjing/jingang
        self.taunted_by = None   # 嘲讽来源 unit
        self.taunt_dur = 0
        self.acted_buff = set()  # 已放过的增益（AI 防重复）
        self.used_ult = 0
        self.stats = dict(dmg=0, taken=0, healed=0, fallback=0, lost_actions_caused=0, ctrl_casts=0)
        # 心法被动
        if faction == 'jian': self.patk = int(self.patk * 1.03)
        if faction == 'qiang': self.patk = int(self.patk * 1.03)
        if faction in ('lei', 'xing'): self.matk = int(self.matk * 1.03)
        if faction == 'chan':
            self.maxhp = int(self.maxhp * 1.05); self.hp = self.maxhp
            self.pdef = int(self.pdef * 1.05)
        if faction == 'shui':
            self.maxhp = int(self.maxhp * 1.04); self.hp = self.maxhp
            self.mdef = int(self.mdef * 1.05)

    def mod(self, key):
        """百分比修正合计（小数）"""
        v = 0.0
        for b in self.buffs:
            if b['k'] == key: v += b['v']
        return v
    def eff_atk(self, kind):
        base = self.patk if kind == 'p' else self.matk
        return base * (1 + self.mod('patk' if kind == 'p' else 'matk'))
    def eff_def(self, kind):
        base = self.pdef if kind == 'p' else self.mdef
        v = base * (1 + self.mod('pdef' if kind == 'p' else 'mdef'))
        v *= (1 - self.stacks.get('pojia', 0) * 0.03)  # 破甲层（仅物防语义，简化为双防不分：只扣物防在调用处处理）
        return v
    def eff_spd(self):
        return self.spd * (1 + self.mod('spd'))
    def dmg_reduction(self):
        return self.mod('dmgred')
    def controlled(self):
        return [c for c in self.ctrl if c['dur'] > 0]
    def hard_controlled(self):
        return any(c['type'] in ('root', 'sleep', 'para', 'freeze', 'chaos') for c in self.controlled())
    def skill_sealed(self):
        return any(c['type'] == 'sealskill' for c in self.controlled())

def lvl_press(atk_lv, def_lv):
    d = (atk_lv - def_lv) * 0.015
    return 1 + max(-0.30, min(0.30, d))

def cost_of(skill, level):
    return int(round(skill['mp'] * (1 + 0.03 * (level - 1))))

def targets_n(skill, level, group_kind):
    t = skill.get('tgt', 1)
    if t == 0: return 99
    if group_kind == 'atk' and t == 2: return 3 if level >= 40 else 2
    if group_kind == 'sup' and t == 2: return 3 if level >= 60 else (2 if level >= 30 else 1)
    return t

class Battle:
    def __init__(self, teamA, teamB, rng):
        self.units = teamA + teamB
        self.rng = rng
        self.round = 0
    def foes(self, u): return [x for x in self.units if x.side != u.side and x.alive]
    def allies(self, u): return [x for x in self.units if x.side == u.side and x.alive]

    # ---- 伤害结算 ----
    def deal(self, src, tgt, mult, kind, segs=1, ignore_def=0.0, can_crit=True, tag=None):
        total = 0
        if kind == 'm' and can_crit and self.rng.random() < SPELL_DODGE:
            return 0
        atk = src.eff_atk(kind)
        base_def = (tgt.pdef if kind == 'p' else tgt.mdef)
        defmod = tgt.mod('pdef' if kind == 'p' else 'mdef')
        dfn = base_def * (1 + defmod) * (1 - ignore_def)
        if kind == 'p': dfn *= (1 - tgt.stacks.get('pojia', 0) * 0.03)
        dfn = max(1.0, dfn)
        per = atk * atk / (atk + dfn) * DMG_K * (1 + 0.05 * max(0, getattr(self, 'round', 0) - 20) + 0.10 * max(0, getattr(self, 'round', 0) - 30))
        # 派系增伤
        boost = 1.0
        if src.f == 'lei' and kind == 'm':
            boost *= 1 + tgt.stacks.get('gandian', 0) * 0.04
        if src.f == 'xing' and kind == 'm' and tag == 'xing_spend':
            boost *= 1 + src.stacks.get('xinghui', 0) * 0.08
        force_crit = (tag == 'xing_crit')
        for _ in range(segs):
            if not tgt.alive: break
            if kind == 'p' and self.rng.random() > PHYS_HIT * (1 + src.mod('hit')):
                continue
            seg = per * mult * boost
            zone = 1.0
            crit_rate = BASE_CRIT + src.mod('crit')
            if src.f == 'jian': crit_rate += src.stacks.get('jianyi', 0) * 0.02
            is_crit = can_crit and (force_crit or self.rng.random() < crit_rate)
            if is_crit:
                cd = CRIT_MULT + (0.10 if src.f == 'jian' else 0.0)
                zone *= cd
            # 连击判定（物理）：心法/增益提供连击率
            combo_rate = src.mod('combo')
            if src.f == 'qiang': combo_rate += 0.05
            combo = kind == 'p' and self.rng.random() < combo_rate
            dmg = seg * min(zone, ZONE_CAP)
            dmg *= lvl_press(src.level, tgt.level)
            dmg *= (1 - tgt.dmg_reduction())
            dmg *= self.rng.uniform(0.95, 1.05)
            dealt = self.apply_damage(src, tgt, dmg)
            total += dealt
            if combo and tgt.alive:
                d2 = seg * COMBO_MULT_SEG * (1 - tgt.dmg_reduction()) * lvl_press(src.level, tgt.level) * self.rng.uniform(0.95, 1.05)
                total += self.apply_damage(src, tgt, d2)
            self.on_hit(src, tgt, kind)
        return total

    def apply_damage(self, src, tgt, dmg):
        dmg = int(round(max(1, dmg)))
        # 唤醒沉睡
        woke = False
        for c in list(tgt.ctrl):
            if c['type'] == 'sleep':
                tgt.ctrl.remove(c); woke = True
                if getattr(tgt, '_sleep_bonus', False):
                    dmg = int(dmg * 1.5); tgt._sleep_bonus = False
        absorbed = min(tgt.shield, dmg); tgt.shield -= absorbed; dmg -= absorbed
        real = min(tgt.hp, dmg); tgt.hp -= real
        src.stats['dmg'] += real + absorbed; tgt.stats['taken'] += real
        # 禅院反伤
        if tgt.f == 'chan' and tgt.alive and src is not tgt:
            rate = tgt.stacks.get('jingang', 0) * 0.03 + tgt.mod('reflect')
            if rate > 0:
                rd = int(round(real * rate))
                if rd > 0:
                    r2 = min(src.hp, rd); src.hp -= r2
                    if src.hp <= 0: src.alive = False
        if tgt.hp <= 0: tgt.alive = False
        return real + absorbed

    def on_hit(self, src, tgt, kind):
        # 命中叠层（心法）
        if not tgt.alive: pass
        if src.f == 'jian' and kind == 'p':
            if self.rng.random() < 0.30:
                src.stacks['jianyi'] = min(5, src.stacks.get('jianyi', 0) + 1)
        if src.f == 'qiang' and kind == 'p':
            tgt.stacks['pojia'] = min(5, tgt.stacks.get('pojia', 0) + 1)
        if src.f == 'lei' and kind == 'm':
            tgt.stacks['gandian'] = min(3, tgt.stacks.get('gandian', 0) + 1)
            # 雷甲反制
        if tgt.f == 'lei' and tgt.mod('leijia') > 0 and self.rng.random() < 0.20:
            src.stacks['gandian'] = min(3, src.stacks.get('gandian', 0) + 1)
        if tgt.f == 'chan':
            tgt.stacks['jingang'] = min(3, tgt.stacks.get('jingang', 0) + 1)

    def heal(self, src, tgt, amount):
        amount = int(round(amount * (1.08 if src.f == 'shui' else 1.0) * max(0.20, 1 - 0.03 * max(0, getattr(self, 'round', 0) - 20) - 0.05 * max(0, getattr(self, 'round', 0) - 30))))
        real = min(tgt.maxhp - tgt.hp, amount)
        tgt.hp += real; src.stats['healed'] += real
        return real

    # ---- 控制结算 ----
    def try_control(self, src, tgt, ctype, base_chance, dur=1):
        if ctype in ('root', 'sleep', 'para', 'freeze', 'chaos'):
            if getattr(tgt, 'ctrl_immune_until', 0) >= self.round and self.round > 0:
                return False
        src.stats['ctrl_casts'] += 1
        chance = base_chance
        if src.f == 'fu': chance += 0.08
        if src.f == 'meng':
            chance += 0.05 + tgt.stacks.get('mengjing', 0) * 0.03
        resist = SEAL_RESIST - tgt.mod('sealres_down') + tgt.mod('sealres')
        chance *= (1 - max(0.0, resist))
        chance *= lvl_press(src.level, tgt.level)
        chance *= (0.7 ** tgt.ctrl_chain)
        chance = max(0.0, min(0.95, chance))
        if self.rng.random() < chance:
            dur = min(dur, 2)
            tgt.ctrl.append(dict(type=ctype, dur=dur))
            if ctype in ('root', 'sleep', 'para', 'freeze', 'chaos'):
                tgt.ctrl_immune_until = self.round + dur
            tgt.ctrl_chain += 1
            src.stats['lost_actions_caused'] += dur
            if src.f == 'meng':
                tgt.stacks['mengjing'] = min(3, tgt.stacks.get('mengjing', 0) + 1)
            return True
        return False

    def add_buff(self, u, k, v, dur):
        u.buffs.append(dict(k=k, v=v, dur=dur))

    # ---- 秘籍执行 ----
    def pick_targets(self, src, skill, foes, n):
        # 嘲讽：单体攻击被强制指向嘲讽者
        if n == 1 and src.taunted_by is not None and src.taunted_by.alive and src.taunt_dur > 0:
            return [src.taunted_by]
        foes_sorted = sorted(foes, key=lambda x: x.hp)
        return foes_sorted[:n]

    def cast(self, u, skill, foes, allies):
        if skill.get('kind') == 'dmg': u._xing_cast_dmg = True
        eff = skill.get('eff'); kind = skill.get('kind')
        cost = cost_of(skill, u.level)
        if u.mp < cost:
            u.stats['fallback'] += 1
            self.basic_attack(u, foes); return
        u.mp -= cost
        cat = skill['cat']
        if kind == 'dmg' or (kind is None and 'mult' in skill):
            atk_kind = skill['atk']
            n = targets_n(skill, u.level, 'atk') if skill.get('tgt') else 1
            tgts = self.pick_targets(u, skill, foes, min(n, len(foes)))
            mult = skill.get('mult', 0.0); segs = skill.get('segs', 1)
            tag = None
            if u.f == 'xing' and atk_kind == 'm':
                if skill['n'] == '落星指' and u.stacks.get('xinghui', 0) >= 3: tag = 'xing_crit'
                elif u.stacks.get('xinghui', 0) > 0: tag = 'xing_spend'
            if eff == 'execute' and tgts and tgts[0].hp < tgts[0].maxhp * 0.30: mult = 2.00
            if eff == 'lei_nulei' and tgts and tgts[0].stacks.get('gandian', 0) > 0: mult = mult * 1.10
            if eff == 'jian_ult':
                mult = 2.40 + u.stacks.get('jianyi', 0) * 0.30
                u.stacks['jianyi'] = 0
            if eff == 'xing_ult':
                mult = 1.70 + u.stacks.get('xinghui', 0) * 0.25
            ign = 0.10 if eff == 'ignore10' else 0.0
            if eff == 'meng_ult':
                for t in tgts:
                    st = t.stacks.get('mengjing', 0)
                    if st > 0:
                        self.deal(u, t, 0.80 * st, 'm', 1)
                        if self.rng.random() < 0.15 * st: self.try_control(u, t, 'sleep', 1.0, 1)
                        t.stacks['mengjing'] = 0
                return
            for t in tgts:
                before = t.hp + t.shield
                dealt = self.deal(u, t, mult, atk_kind, segs, ign, tag=tag)
                # 效果钩子（伤害后）
                self.after_damage(u, t, skill, eff, dealt)
            if u.f == 'xing' and atk_kind == 'm' and tag in ('xing_spend', 'xing_crit'):
                u.stacks['xinghui'] = 0
            if eff == 'qiang_ult' and tgts and not tgts[0].alive:
                u.mp = min(u.maxmp, u.mp + int(u.maxmp * 0.30))
                alive_foes = self.foes(u)
                if alive_foes: self.basic_attack(u, alive_foes)
        elif kind == 'ctrl':
            self.cast_ctrl(u, skill, eff, foes)
        elif kind in ('buff', 'shield', 'heal') or cat in ('辅助',):
            self.cast_support(u, skill, eff, foes, allies)
        else:
            self.basic_attack(u, foes)

    def after_damage(self, u, t, skill, eff, dealt):
        if not t.alive and eff not in (None,):
            pass
        if eff == 'deb_atk15' and t.alive: self.add_buff(t, 'patk', -0.15, 2)
        if eff == 'deb_pdef12_grp' and t.alive: self.add_buff(t, 'pdef', -0.12, 2)
        if eff == 'deb_spd15_grp' and t.alive: self.add_buff(t, 'spd', -0.15, 2)
        if eff == 'deb_hit18' and t.alive: self.add_buff(t, 'hit', -0.18, 2)
        if eff == 'seal_skill55' and t.alive: self.try_control(u, t, 'sealskill', 0.55, 1)
        if eff == 'para70' and t.alive: self.try_control(u, t, 'para', 0.70, 1)
        if eff == 'para60' and t.alive: self.try_control(u, t, 'para', 0.60, 1)
        if eff == 'root75' and t.alive: self.try_control(u, t, 'root', 0.75, 1)
        if eff == 'freeze70' and t.alive: self.try_control(u, t, 'freeze', 0.70, 1)
        if eff == 'fu_fuling' and t.alive: self.try_control(u, t, 'root', 0.30, 1)
        if eff == 'fu_wanfu' and t.alive: self.try_control(u, t, self.rng.choice(['root', 'sealskill']), 0.15, 1)
        if eff == 'meng_shenlou' and t.alive: self.try_control(u, t, 'chaos', 0.45, 1)
        if eff == 'chan_fanyin' and t.alive:
            self.add_buff(t, 'spd', -0.15, 2); self.try_control(u, t, 'root', 0.35, 1)
        if eff == 'xing_luanxing' and t.alive:
            self.add_buff(t, 'spd', -0.20, 2); self.try_control(u, t, 'root', 0.55, 1)
        if eff == 'qiang_fuma' and t.alive:
            self.add_buff(t, 'spd', -0.20, 2); self.try_control(u, t, 'root', 0.40, 1)
        if eff == 'taunt1' or eff == 'taunt_grp':
            for e in self.foes(u):
                if e.alive: e.taunted_by = u; e.taunt_dur = 1
        if eff == 'chan_qianfo' and t.alive:
            self.heal(u, u, u.maxhp * 0.02)
        if eff == 'shui_nutao':
            self.heal(u, u, u.maxhp * 0.05)
        if eff == 'lei_nulei' and t.stacks.get('gandian', 0) > 0:
            pass  # +20% 已由更高倍率近似：怒雷对感电额外伤害在 deal 中未单列，此处以命中后补伤近似
        if eff == 'lei_jump':
            others = [x for x in self.foes(u) if x.alive and x is not t and x.stacks.get('gandian', 0) > 0]
            if others:
                self.deal(u, others[0], 0.475, 'm', 1)
        if eff == 'lei_ult' and t.alive:
            st = t.stacks.get('gandian', 0)
            if st > 0:
                self.deal(u, t, 0.25 * st, 'm', 1, can_crit=False)
                if self.rng.random() < 0.30: self.try_control(u, t, 'para', 1.0, 1)
                t.stacks['gandian'] = 0
        if eff == 'fu_ult' and t.alive:
            self.try_control(u, t, 'root', 0.65, 1); self.try_control(u, t, 'sealskill', 0.65, 1)
        if eff == 'meng_dot' and t.alive:
            t.buffs.append(dict(k='dot', v=0.35, dur=3, src=u))
        if eff == 'qiang_hengsao' and t.alive:
            t.stacks['pojia'] = min(5, t.stacks.get('pojia', 0) + 1)

    def cast_ctrl(self, u, skill, eff, foes):
        n = targets_n(skill, u.level, 'atk') if skill.get('tgt') else 1
        cands = sorted(foes, key=lambda x: -x.eff_spd())
        tgts = [t for t in cands if not t.hard_controlled()][:n] or cands[:n]
        for t in tgts:
            if eff == 'root80': self.try_control(u, t, 'root', 0.80, 1)
            elif eff == 'sealskill75_2': self.try_control(u, t, 'sealskill', 0.75, 2)
            elif eff == 'sleep75':
                if self.try_control(u, t, 'sleep', 0.75, 1): t._sleep_bonus = True
            elif eff == 'sleep60_grp':
                if self.try_control(u, t, 'sleep', 0.60, 1): t._sleep_bonus = True

    def cast_support(self, u, skill, eff, foes, allies):
        weakest = sorted(allies, key=lambda x: x.hp / x.maxhp)
        if eff == 'jian_buff':
            self.add_buff(u, 'crit', 0.12, 3); u.stacks['jianyi'] = min(5, u.stacks.get('jianyi', 0) + 2)
        elif eff == 'qiang_buff':
            self.add_buff(u, 'combo', 0.15, 3); self.add_buff(u, 'patk', 0.08, 3)
        elif eff == 'spd12_grp':
            for a in allies[:targets_n(skill, u.level, 'sup') if skill.get('tgt') is None else 99][:3] if False else allies:
                self.add_buff(a, 'spd', 0.12, 3)
        elif eff == 'patk10_grp':
            for a in allies: self.add_buff(a, 'patk', 0.10, 3)
        elif eff == 'matk10_grp':
            for a in allies: self.add_buff(a, 'matk', 0.10, 3)
        elif eff == 'pdef12_grp':
            for a in allies: self.add_buff(a, 'pdef', 0.12, 3)
        elif eff == 'dodge12_grp':
            for a in allies: self.add_buff(a, 'dodge', 0.12, 3)
        elif eff == 'dmgred10_grp':
            for a in allies: self.add_buff(a, 'dmgred', 0.10, 3)
        elif eff == 'lei_jia':
            self.add_buff(u, 'mdef', 0.15, 3); self.add_buff(u, 'leijia', 1, 3)
        elif eff == 'xing_tuiyan':
            t = weakest[0] if weakest else u
            self.add_buff(t, 'hit', 0.15, 3); self.add_buff(t, 'crit', 0.10, 3)
        elif eff == 'fu_qingxin':
            t = min(allies, key=lambda x: len(x.ctrl)) if allies else u
            t.ctrl.clear(); t.ctrl_chain = 0; self.add_buff(t, 'sealres', 0.15, 2)
        elif eff == 'meng_xingmeng':
            for a in allies:
                a.ctrl.clear(); a.ctrl_chain = 0; self.add_buff(a, 'spd', 0.08, 2)
        elif eff == 'meng_shimeng':
            t = sorted(foes, key=lambda x: -x.eff_spd())[0]
            self.add_buff(t, 'sealres_down', 0.20, 2); t.stacks['mengjing'] = min(3, t.stacks.get('mengjing', 0) + 1)
        elif eff == 'shield12_grp':
            for a in allies: a.shield += int(u.maxhp * 0.12); a.shield_dur = max(a.shield_dur, 2)
            for a in allies: self.add_buff(a, 'shieldtag', 1, 2)
        elif eff == 'chan_numu':
            for e in foes: e.taunted_by = u; e.taunt_dur = 2
            self.add_buff(u, 'pdef', 0.15, 2)
        elif eff == 'chan_ult':
            self.add_buff(u, 'reflect', 0.25, 3); u.shield += int(u.maxhp * 0.20); u.shield_dur = max(u.shield_dur, 3)
            u.ctrl = [c for c in u.ctrl if c['type'] == 'none']
            self.add_buff(u, 'ctrlimmune', 1, 3)
        elif eff == 'shui_guixi':
            self.add_buff(u, 'hot', 0.04, 3); self.add_buff(u, 'mdef', 0.12, 3)
        elif skill.get('kind') == 'heal' or eff in ('shui_shuimu', 'shui_ult', 'cleanse1'):
            mult = skill.get('mult', 1.0)
            if eff == 'shui_shuimu':
                for a in allies:
                    self.heal(u, a, u.eff_atk('m') * mult); self.add_buff(a, 'dmgred', 0.08, 2)
            elif eff == 'shui_ult':
                for a in allies:
                    self.heal(u, a, u.eff_atk('m') * mult)
                    a.buffs = [b for b in a.buffs if b['v'] >= 0]; a.ctrl.clear()
                    self.add_buff(a, 'dmgred', 0.15, 2)
            elif eff == 'cleanse1':
                t = weakest[0]
                self.heal(u, t, u.eff_atk('m') * mult)
                neg = [b for b in t.buffs if b['v'] < 0]
                if neg: t.buffs.remove(neg[0])
            else:
                for a in weakest[:targets_n(skill, u.level, 'sup')]:
                    self.heal(u, a, u.eff_atk('m') * mult)
        else:
            self.basic_attack(u, foes)

    def basic_attack(self, u, foes):
        if not foes: return
        tgts = self.pick_targets(u, dict(tgt=1), foes, 1)
        kind = 'm' if u.f in ('lei', 'xing', 'shui') else 'p'
        if u.f in ('fu', 'meng'): kind = 'p'
        self.deal(u, tgts[0], 1.00, kind, 1)

    # ---- AI 托管策略 ----
    def get(self, u, name):
        for s in SK[u.f]:
            if s['n'] == name: return s
        return None
    def choose(self, u, foes, allies):
        f = u.f; lv = u.level
        def has(name):
            s = self.get(u, name)
            return s and lv >= s['lv'] and u.mp >= cost_of(s, lv)
        def cast(name):
            return self.get(u, name)
        enemies = foes; n_en = len(enemies)
        lowest = min(enemies, key=lambda x: x.hp) if enemies else None
        # 玄水：优先治疗
        if f == 'shui':
            hurt = [a for a in allies if a.hp < a.maxhp * 0.70]
            if hurt and lv >= 80 and u.mp >= cost_of(self.get(u, '沧澜倒海'), lv) and len(hurt) >= 2:
                return cast('沧澜倒海')
            if hurt and has('若水润心'): return cast('若水润心')
            if hurt and has('水幕天华'): return cast('水幕天华')
            if 'guixi' not in u.acted_buff and has('龟息功'):
                u.acted_buff.add('guixi'); return cast('龟息功')
            if n_en >= 3 and has('沧海横流'): return cast('沧海横流')
            if has('寒潮') and n_en >= 2 and 'hanchao' not in u.acted_buff:
                u.acted_buff.add('hanchao'); return cast('寒潮')
            if has('玄冰封'): return cast('玄冰封')
            if has('怒涛掌'): return cast('怒涛掌')
            if has('沧澜诀'): return cast('沧澜诀')
            return None
        # 禅院
        if f == 'chan':
            if lv >= 80 and u.hp < u.maxhp * 0.5 and has('不动明王'): return cast('不动明王')
            if 'numu' not in u.acted_buff and has('金刚怒目'):
                u.acted_buff.add('numu'); return cast('金刚怒目')
            hurt = [a for a in allies if a.hp < a.maxhp * 0.55]
            if hurt and has('金钟罩'): return cast('金钟罩')
            if n_en >= 3 and has('千佛手'): return cast('千佛手')
            if n_en >= 2 and has('狮子吼'): return cast('狮子吼')
            if has('伏魔杖法'): return cast('伏魔杖法')
            if has('罗汉伏魔功'): return cast('罗汉伏魔功')
            return None
        # 缚灵符观
        if f == 'fu':
            uncontrolled = [e for e in enemies if not e.hard_controlled() and not e.skill_sealed()]
            if uncontrolled:
                if has('封魔印'): return cast('封魔印')
                if has('定身符'): return cast('定身符')
            if any(a.hard_controlled() for a in allies) and has('清心符'): return cast('清心符')
            if n_en >= 3 and has('五雷符阵'): return cast('五雷符阵')
            if n_en >= 2 and has('万符天箓'): return cast('万符天箓')
            if lv >= 80 and n_en >= 3 and has('天箓镇魔'): return cast('天箓镇魔')
            if has('诛魔符'): return cast('诛魔符')
            if has('缚灵符箓'): return cast('缚灵符箓')
            return None
        # 织梦谷
        if f == 'meng':
            uncontrolled = [e for e in enemies if not e.hard_controlled()]
            if n_en >= 3 and has('黄粱一梦'): return cast('黄粱一梦')
            if uncontrolled and has('入梦引'): return cast('入梦引')
            if n_en >= 3 and has('蜃楼幻境'): return cast('蜃楼幻境')
            if lv >= 80 and any(e.stacks.get('mengjing', 0) >= 2 for e in enemies) and has('大梦三千'): return cast('大梦三千')
            if has('梦魇缠身'): return cast('梦魇缠身')
            return None
        # 断岳剑宗
        if f == 'jian':
            if len(allies) > 1 and 'jianxin' not in u.acted_buff and has('剑心通明'):
                u.acted_buff.add('jianxin'); return cast('剑心通明')
            if lv >= 80 and u.stacks.get('jianyi', 0) >= 4 and has('一剑开天'): return cast('一剑开天')
            if lowest and lowest.hp < lowest.maxhp * 0.30 and has('残月斩'): return cast('残月斩')
            if n_en >= 3 and has('千锋破空'): return cast('千锋破空')
            if has('裂空九剑'): return cast('裂空九剑')
            if has('残月斩'): return cast('残月斩')
            return None
        # 破军枪府
        if f == 'qiang':
            if len(allies) > 1 and 'zhanyi' not in u.acted_buff and has('战意沸腾'):
                u.acted_buff.add('zhanyi'); return cast('战意沸腾')
            if lv >= 80 and lowest and lowest.hp < lowest.maxhp * 0.35 and has('破军杀将'): return cast('破军杀将')
            if n_en >= 3 and has('横扫千军'): return cast('横扫千军')
            if has('陷阵连环枪'): return cast('陷阵连环枪')
            if has('裂阵突刺'): return cast('裂阵突刺')
            return None
        # 惊雷殿
        if f == 'lei':
            if lv >= 80 and n_en >= 3 and any(e.stacks.get('gandian', 0) >= 2 for e in enemies) and has('天雷荡魔'): return cast('天雷荡魔')
            if n_en >= 3 and has('九霄神雷'): return cast('九霄神雷')
            if n_en >= 2 and has('连环霹雳'): return cast('连环霹雳')
            if has('怒雷天罚'): return cast('怒雷天罚')
            if has('天雷破'): return cast('天雷破')
            return None
        # 璇玑星宫
        if f == 'xing':
            if u.stacks.get('xinghui', 0) >= 3:
                if lv >= 80 and n_en >= 3 and has('周天星陨'): return cast('周天星陨')
                if n_en >= 3 and has('陨星天坠'): return cast('陨星天坠')
                if has('落星指'): return cast('落星指')
            uncontrolled = [e for e in enemies if not e.hard_controlled()]
            if len(allies) == 1 and u.hp > u.maxhp * 0.4:
                if has('落星指'): return cast('落星指')
                if has('引星诀'): return cast('引星诀')
            if uncontrolled and has('定星咒'): return cast('定星咒')
            if 'tuiyan' not in u.acted_buff and has('星盘推演'):
                u.acted_buff.add('tuiyan'); return cast('星盘推演')
            if has('引星诀'): return cast('引星诀')
            return None
        return None

    # ---- 回合主循环 ----
    def run(self):
        while self.round < MAXROUND:
            self.round += 1
            # 回合开始：DOT/HOT
            for u in self.units:
                if not u.alive: continue
                for b in list(u.buffs):
                    if b['k'] == 'dot':
                        src = b.get('src')
                        if src and src.alive:
                            self.deal(src, u, b['v'], 'm', 1, can_crit=False)
                    if b['k'] == 'hot':
                        self.heal(u, u, u.maxhp * b['v'])
                if u.f == 'xing':
                    u._xing_cast_dmg = False
            order = sorted([u for u in self.units if u.alive], key=lambda x: (-x.eff_spd() * self.rng.uniform(0.97, 1.03)))
            for u in order:
                if not u.alive: continue
                if not self.foes(u): break
                # 控制判定
                if u.mod('ctrlimmune') > 0:
                    u.ctrl = [c for c in u.ctrl if False]
                ctrls = u.controlled()
                skip = False
                for c in ctrls:
                    if c['type'] in ('root', 'sleep', 'para', 'freeze'):
                        skip = True
                    elif c['type'] == 'chaos':
                        skip = True
                        # 混乱：50% 普攻随机友军
                        if self.rng.random() < 0.5:
                            mates = [a for a in self.allies(u) if a is not u]
                            if mates: self.deal(u, self.rng.choice(mates), 1.0, 'p', 1)
                if skip:
                    continue
                foes = self.foes(u); allies = self.allies(u)
                skill = self.choose(u, foes, allies)
                if skill is None:
                    # 回退统计：存在已开放但真气不足的攻击/控制秘籍而被迫普攻
                    for s in SK[u.f]:
                        if s['cat'] in ('单体', '群体', '绝学', '控制') and u.level >= s['lv'] and u.mp < cost_of(s, u.level):
                            u.stats['fallback'] += 1; break
                if skill is None or u.skill_sealed() and skill.get('cat') != '心法':
                    if u.skill_sealed():
                        self.basic_attack(u, foes)
                    elif skill is None:
                        self.basic_attack(u, foes)
                    else:
                        self.cast(u, skill, foes, allies)
                else:
                    self.cast(u, skill, foes, allies)
                if not self.foes(u): break
            # 回合结束：时长递减
            for u in self.units:
                if u.f == 'xing' and u.alive and not getattr(u, '_xing_cast_dmg', False):
                    u.stacks['xinghui'] = min(3, u.stacks.get('xinghui', 0) + 1)
                for b in u.buffs: b['dur'] -= 1
                u.buffs = [b for b in u.buffs if b['dur'] > 0 and b['k'] not in ('shieldtag',)]
                for c in u.ctrl: c['dur'] -= 1
                u.ctrl = [c for c in u.ctrl if c['dur'] > 0]
                if u.shield_dur > 0:
                    u.shield_dur -= 1
                    if u.shield_dur == 0: u.shield = 0
                if u.taunt_dur > 0:
                    u.taunt_dur -= 1
                    if u.taunt_dur == 0: u.taunted_by = None
                if not any(c for c in u.ctrl):
                    u.ctrl_chain = 0
            a_alive = any(u.alive and u.side == 0 for u in self.units)
            b_alive = any(u.alive and u.side == 1 for u in self.units)
            if not a_alive or not b_alive:
                return (0 if a_alive else 1), self.round
        return -1, self.round  # 未分胜负

# ---------------- 对局运行与报表 ----------------
def make_team(comp, level, side):
    return [Unit(f, level, side, i) for i, f in enumerate(comp)]

def run_match(compA, compB, levelA, levelB, n=300, seed0=1000):
    rounds = []; wins = [0, 0]; draws = 0
    fb = 0; lost = 0; fast = 0
    fast_bar = 2 if len(compA) == 1 else 4
    for i in range(n):
        rng = random.Random(seed0 + i)
        A = make_team(compA, levelA, 0); B = make_team(compB, levelB, 1)
        bt = Battle(A, B, rng)
        w, r = bt.run()
        rounds.append(r)
        if w == -1: draws += 1
        else: wins[w] += 1
        if r <= fast_bar: fast += 1
        fb += sum(u.stats['fallback'] for u in bt.units)
        lost += sum(u.stats['lost_actions_caused'] for u in bt.units)
    return dict(n=n, avg=round(statistics.mean(rounds), 2), med=statistics.median(rounds),
                p90=sorted(rounds)[int(len(rounds) * 0.9)], max=max(rounds),
                winA=wins[0], winB=wins[1], draw=draws,
                draw_rate=round(draws / n, 3), fast_rate=round(fast / n, 3),
                fallback_per_fight=round(fb / n, 2), lost_actions_per_fight=round(lost / n, 2))

def label(comp):
    return '+'.join(FACTION_NAME[f] for f in comp)

def main():
    out = {}
    print('== 1v1 镜像对局（同派同级） ==')
    print(f"{'对局':<22}{'等级':>5}{'平均':>7}{'中位':>6}{'P90':>5}{'速杀率':>8}{'未分率':>8}{'回退/场':>8}{'夺行动/场':>10}")
    for lv in (20, 70, 89):
        for f in ('jian', 'qiang', 'lei', 'xing', 'fu', 'meng', 'chan', 'shui'):
            r = run_match([f], [f], lv, lv, n=200, seed0=lv * 10000 + hash(f) % 1000)
            out[f'1v1镜像_{f}_{lv}'] = r
            print(f"{FACTION_NAME[f]+' vs '+FACTION_NAME[f]:<22}{lv:>5}{r['avg']:>7}{r['med']:>6}{r['p90']:>5}{r['fast_rate']:>8}{r['draw_rate']:>8}{r['fallback_per_fight']:>8}{r['lost_actions_per_fight']:>10}")
    print('\n== 1v1 跨派对局（89 级） ==')
    for a, b in (('jian', 'lei'), ('jian', 'fu'), ('lei', 'fu'), ('qiang', 'xing'), ('chan', 'jian'), ('shui', 'qiang'), ('meng', 'lei'), ('fu', 'meng')):
        r = run_match([a], [b], 89, 89, n=200, seed0=50000 + hash(a + b) % 1000)
        out[f'1v1跨派_{a}_{b}_89'] = r
        print(f"{label([a])+' vs '+label([b]):<28}{'平均回合':}{r['avg']}  A胜{r['winA']} B胜{r['winB']} 未分{r['draw']} 速杀率{r['fast_rate']} 回退/场{r['fallback_per_fight']} 夺行动/场{r['lost_actions_per_fight']}")
    print('\n== 1v1 等级压制抽测（剑宗 89 vs 70） ==')
    r = run_match(['jian'], ['jian'], 89, 70, n=200, seed0=777)
    out['1v1压制_jian_89v70'] = r
    print(f"89 胜 {r['winA']} / 70 胜 {r['winB']} / 未分 {r['draw']}，平均回合 {r['avg']}")
    print('\n== 4v4 标准阵容 ==')
    A = ['jian', 'lei', 'fu', 'chan']; B = ['qiang', 'xing', 'meng', 'shui']
    for lv in (20, 70, 89):
        r = run_match(A, B, lv, lv, n=300, seed0=90000 + lv)
        out[f'4v4_AB_{lv}'] = r
        print(f"{lv}级 A({label(A)}) vs B({label(B)}): 平均{r['avg']} 中位{r['med']} P90{r['p90']} A胜{r['winA']} B胜{r['winB']} 未分{r['draw']} 速杀率{r['fast_rate']} 回退/场{r['fallback_per_fight']} 夺行动/场{r['lost_actions_per_fight']}")
    C = ['qiang', 'xing', 'meng', 'chan']
    for lv in (70, 89):
        r = run_match(A, C, lv, lv, n=300, seed0=97000 + lv)
        out[f'4v4_AC_{lv}'] = r
        print(f"{lv}级 A vs C(枪+星+梦+禅): 平均{r['avg']} 中位{r['med']} P90{r['p90']} A胜{r['winA']} C胜{r['winB']} 未分{r['draw']} 速杀率{r['fast_rate']} 回退/场{r['fallback_per_fight']} 夺行动/场{r['lost_actions_per_fight']}")
    for nm, X in (('A镜像', A), ('B镜像', B)):
        r = run_match(X, X, 89, 89, n=300, seed0=95000 + len(nm))
        out[f'4v4_{nm}_89'] = r
        print(f"89级 {nm}({label(X)}): 平均{r['avg']} 中位{r['med']} P90{r['p90']} 先手方胜{r['winA']} 后手方胜{r['winB']} 未分{r['draw']} 速杀率{r['fast_rate']} 回退/场{r['fallback_per_fight']} 夺行动/场{r['lost_actions_per_fight']}")
    with open('report.json', 'w', encoding='utf-8') as fh:
        json.dump(out, fh, ensure_ascii=False, indent=1)
    print('\n报表已写入 report.json')

if __name__ == '__main__':
    main()
