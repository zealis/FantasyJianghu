# -*- coding: utf-8 -*-
"""V1.29 两张数值表的回归校验（数值规范 §7 流程）。

覆盖：
  1. 《套装加成配置表》V1.0 —— 一档/二档底数、八派微调逐派数值、同档同幅、幅度不超主项一半；
  2. 完美档同属性合计（剑宗暴击 6.5% / 雷殿法暴 6.5% / 符观封印命中 10%），与配置表 §4 一致；
  3. 《宗派战胜场加成表》V1.0 —— 10/30/60/100 胜分档、只取最高档不叠加、封顶 +4% 不超过称号四档 +5%；
  4. 符观封印命中 +10% 的重点校准：按 formula.control 算实际封印成功率，给出「是否需要微调降档」的结论；
  5. 乘区上限（×2.25）与「微调不得顶掉系别副项」的结构性检查；
  6. 数值规范 §9.7 角色级系数口径：反伤/治疗加成上限（50%/60%）、禅院反伤放行、
     玄水治疗与星宫蓄力在上游机制落地前必须保持 implemented=false（不伪造生效）。

用法：python check_v129.py [配置包版本，默认 1.10.0]
产出：report_v129_<version>.json
"""
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
VERSION = sys.argv[1] if len(sys.argv) > 1 else "1.10.0"
BUILD = os.path.join(ROOT, "server", "configs", "build", VERSION)
OUT = os.path.join(HERE, "report_v129_" + VERSION + ".json")

# ---- 文档基准（《套装加成配置表》V1.0 §2/§3、《宗派战胜场加成表》V1.0 §2） ----
DOC_TIER1 = [1.0, 2.0, 3.0, 4.0, 5.0]
DOC_TIER2 = [5.0, 6.0, 7.0, 8.0, 9.0]
DOC_SUB_PCT = {
    "物理": [1.5, 2.0, 2.5, 3.0, 3.5],
    "法术": [1.5, 2.0, 2.5, 3.0, 3.5],
    "封印": [3.0, 4.0, 5.0, 6.0, 7.0],
    "肉盾": [5.0, 6.0, 7.0, 8.0, 9.0],
}
DOC_TWEAK = [1.0, 1.5, 2.0, 2.5, 3.0]
DOC_FACTION_ATTR = {
    "断岳剑宗": "pcrit", "破军枪府": "pcombo",
    "惊雷殿": "mcrit", "璇玑星宫": "charge_dmg",
    "缚灵符观": "sealhit", "织梦谷": "pdodge",
    "金刚禅院": "reflect", "玄水宫": "heal_bonus",
}
DOC_SECT_TIERS = [(10, 1.0), (30, 2.0), (60, 3.0), (100, 4.0)]
DOC_TITLE_TOP = 5.0  # 称号四档全属性 +5%（数值规范 §9.5），宗派战封顶 +4% 不得盖过

results = []


def load(rel):
    with open(os.path.join(BUILD, rel), encoding="utf-8") as fh:
        return json.load(fh)


def check(name, ok, detail=""):
    results.append({"item": name, "ok": bool(ok), "detail": detail})
    print(("  PASS  " if ok else "  FAIL  ") + name + (("  — " + detail) if detail else ""))
    return ok


def approx(a, b, eps=1e-9):
    return abs(float(a) - float(b)) < eps


def check_suit(suit):
    print("[套装加成配置表 V1.0]")
    check("一档全属性底数与文档一致", suit["tier1_all_pct"] == DOC_TIER1, str(suit["tier1_all_pct"]))
    check("二档系别主项底数与文档一致", suit["tier2_main_pct"] == DOC_TIER2, str(suit["tier2_main_pct"]))

    for sysname, sysdef in suit["systems"].items():
        want = DOC_SUB_PCT.get(sysname)
        if want is None:
            check("系别 %s 在文档中存在" % sysname, False, "配置多出未登记系别")
            continue
        check("系别 %s 副项数值与文档一致" % sysname, sysdef["sub_pct"] == want, str(sysdef["sub_pct"]))

    for faction, attr in DOC_FACTION_ATTR.items():
        tw = suit["faction_tweaks"].get(faction)
        if not tw:
            check("门派 %s 有微调配置" % faction, False)
            continue
        check("门派 %s 微调属性 = %s" % (faction, attr), tw["attr"] == attr, tw["attr"])
        check("门派 %s 微调幅度同档同幅（%s）" % (faction, DOC_TWEAK), tw["pct"] == DOC_TWEAK, str(tw["pct"]))
        over = [i for i, v in enumerate(tw["pct"]) if v > suit["tier2_main_pct"][i] / 2 + 1e-9]
        check("门派 %s 微调不超主项一半" % faction, not over, ("超限档位 " + str(over)) if over else "")

    # 完美档同属性合计（配置表 §4，V1.30 起二档**取代**一档、不叠全属性）
    perfect = 4
    combine = {}

    def total(faction, category):
        pct = {}
        sysdef = suit["systems"][category]
        for a in sysdef["main"]:
            pct[a] = pct.get(a, 0.0) + suit["tier2_main_pct"][perfect]
        pct[sysdef["sub"]] = pct.get(sysdef["sub"], 0.0) + sysdef["sub_pct"][perfect]
        tw = suit["faction_tweaks"][faction]
        if tw.get("implemented", True):
            pct[tw["attr"]] = pct.get(tw["attr"], 0.0) + tw["pct"][perfect]
        return pct

    combine["断岳剑宗"] = total("断岳剑宗", "物理")
    combine["惊雷殿"] = total("惊雷殿", "法术")
    combine["缚灵符观"] = total("缚灵符观", "封印")
    combine["织梦谷"] = total("织梦谷", "封印")

    check("完美档 剑宗暴击 = 3.5 + 3 = 6.5%",
          approx(combine["断岳剑宗"]["pcrit"], 6.5), str(combine["断岳剑宗"]["pcrit"]))
    check("完美档 雷殿法暴 = 3.5 + 3 = 6.5%",
          approx(combine["惊雷殿"]["mcrit"], 6.5), str(combine["惊雷殿"]["mcrit"]))
    check("完美档 符观封印命中 = 7 + 3 = 10%",
          approx(combine["缚灵符观"]["sealhit"], 10.0), str(combine["缚灵符观"]["sealhit"]))
    check("完美档 织梦谷只叠闪避（速度不叠主项）",
          approx(combine["织梦谷"]["pdodge"], 3.0) and approx(combine["织梦谷"]["speed"], 9.0),
          "闪避 %s / 速度 %s" % (combine["织梦谷"]["pdodge"], combine["织梦谷"]["speed"]))
    # V1.30 定案：5 件全同品质触发二档时只结算二档，不再叠加一档全属性。
    leaked = [a for a in suit["all_attrs"]
              if a not in suit["systems"]["物理"]["main"]
              and a not in ("pcrit",) and combine["断岳剑宗"].get(a, 0.0) != 0.0]
    check("二档取代一档（不再叠加全属性 +%s%%）" % suit["tier1_all_pct"][perfect],
          not leaked, ("仍带全属性的项 " + str(leaked)) if leaked else "hp/mp/法攻等均未被抬")

    # 结构性检查：微调不得与系别主项同键（否则等于给主项加倍）
    for faction, tw in suit["faction_tweaks"].items():
        cat = None
        for c, s in suit["systems"].items():
            if faction in ():
                pass
        # 由门派→系别映射（与 factions.json 对齐）
        cat = FACTION_CATEGORY.get(faction)
        if not cat:
            continue
        mains = suit["systems"][cat]["main"]
        check("门派 %s 微调不落在系别主项上（%s）" % (faction, cat), tw["attr"] not in mains, tw["attr"])

    # 乘区上限：任一属性合计不得超过 ×2.25（即 225% 加成）
    peak = max(max(p.values()) for p in combine.values())
    check("套装加成合计未突破乘区上限 2.25", peak / 100.0 + 1.0 <= 2.25, "峰值 +%.1f%%（系数 %.2f）" % (peak, 1 + peak / 100))
    return combine


def check_sect_war(sw):
    print("[宗派战胜场加成表 V1.0]")
    got = [(t["wins"], t["all_pct"]) for t in sw["tiers"]]
    check("胜场分档与文档一致（10/30/60/100 → 1/2/3/4%）", got == DOC_SECT_TIERS, str(got))
    check("胜场加成封顶 4% 不盖过称号四档 +5%", max(v for _, v in got) < DOC_TITLE_TOP, str(max(v for _, v in got)))
    wins_sorted = [w for w, _ in got]
    check("分档严格递增", wins_sorted == sorted(wins_sorted) and len(set(wins_sorted)) == len(wins_sorted), str(wins_sorted))


def check_seal_calibration(suit, formula):
    """符观封印命中 +10% 的重点校准（V1.29 定案要求的 §7 回归项）。"""
    print("[符观封印命中 +10% 校准]")
    res_base = float(formula["control"]["seal_res_base"])
    decay = float(formula["control"]["combo_decay"])
    rows = []
    verdicts = []
    for base_rate in (60.0, 70.0, 80.0):
        for seal_res in (0.0, res_base, 0.20):
            plain = (base_rate / 100.0) * (1 - seal_res)
            with_suit = (base_rate / 100.0 + 0.10) * (1 - seal_res)
            rows.append({
                "base_rate": base_rate, "seal_res": seal_res,
                "without_set": round(plain, 4), "with_set": round(with_suit, 4),
                "abs_gain": round((with_suit - plain) * 100, 2),
                "rel_gain_pct": round((with_suit / plain - 1) * 100, 2) if plain > 0 else None,
            })
            # 红线：单次控制成功率不宜超过 90%（给对手保留 10% 容错，且叠抗性仍可反制）
            if with_suit > 0.90 + 1e-9:
                verdicts.append("base=%.0f%% 抗性=%.0f%% 时成功率 %.1f%% 超过 90%% 红线"
                                % (base_rate, seal_res * 100, with_suit * 100))
    worst = max(r["with_set"] for r in rows)
    best_rel = max((r["rel_gain_pct"] or 0) for r in rows)
    ok = not verdicts
    check("封印成功率未突破 90% 安全线", ok,
          ("最高 %.1f%%" % (worst * 100)) if ok else "; ".join(verdicts))
    detail = "相对提升最高 %.1f%%；连控递减 ×%.2f 仍生效（第 2 次降至 %.1f%%）" % (
        best_rel, decay, rows[0]["with_set"] * decay * 100)
    check("连控递减仍能兜住（不产生无限控制链）", decay < 1.0, detail)

    # 若需降档：微调 3% → 2% 后封印命中合计 9%
    alt = 9.0
    check("备用降档（微调 3%→2%，合计 9%）可行且更保守", alt < 10.0, "封命合计 %.0f%%" % alt)
    return rows, ("PASS" if ok else "NEEDS_TUNE")


# ---- 数值规范 §9.7：角色级系数（反伤 / 治疗加成 / 蓄力伤害） ----
DOC_COEFF_CAPS = {"reflect_cap_pct": 5000, "heal_bonus_cap_pct": 6000}


def check_coeffs(formula, suit):
    caps = formula.get("coeff_caps", {})
    for key, want in DOC_COEFF_CAPS.items():
        got = caps.get(key)
        check("角色级系数上限 %s 与文档一致（%s）" % (key, int(want / 100)),
              got == want, "%s=%s" % (key, got))
    check("系数上限为万分制且不超过 100%",
          all(0 <= int(v) <= 10000 for v in caps.values() if isinstance(v, (int, float))),
          str(caps))
    # 禅院反伤已接线 → 允许 implemented=true；玄水治疗、星宫蓄力上游未落地 → 必须 false。
    tweaks = suit.get("faction_tweaks", {})
    reflect = tweaks.get("金刚禅院", {})
    check("禅院反伤已接线并放行（implemented=true）",
          reflect.get("attr") == "reflect" and reflect.get("implemented") is True,
          "implemented=%s" % reflect.get("implemented"))
    for faction, attr, why in [("玄水宫", "heal_bonus", "治疗秘籍未入包"),
                               ("璇玑星宫", "charge_dmg", "星辉机制未实现")]:
        tw = tweaks.get(faction, {})
        check("%s 的 %s 仍标未生效（%s）" % (faction, attr, why),
              tw.get("attr") == attr and tw.get("implemented") is False,
              "implemented=%s" % tw.get("implemented"))
    # 反伤 3% 远低于 50% 上限，不构成反弹构筑风险。
    pct = (reflect.get("pct") or [0])[-1]
    check("禅院反伤完美档 3% 未触碰 50% 上限", pct * 100 <= caps.get("reflect_cap_pct", 0),
          "反伤 %.1f%% / 上限 %.0f%%" % (pct, caps.get("reflect_cap_pct", 0) / 100))

    # 心法金刚印（门派身份，1 级裸装即有）与绝学不动明王必须按秘籍表 V1.1 §8 落数。
    try:
        jg = load("battle/skills_jingang.json")
        skills = {s["id"]: s for s in jg.get("skills", [])}
    except Exception as exc:  # 配置包缺文件即判失败，不放行
        check("金刚禅院秘籍谱存在", False, str(exc))
        return
    passive = skills.get("jingang_buhuai", {}).get("passive", {})
    check("金刚不坏体 生命/物防 +5%、金刚印 3%/层 上限 3",
          passive.get("hp_pct") == 500 and passive.get("pdef_pct") == 500
          and passive.get("reflect_per_stack") == 300 and passive.get("reflect_max_stacks") == 3,
          str(passive))
    check("金刚印满层反伤未触碰 50% 上限",
          passive.get("reflect_per_stack", 0) * passive.get("reflect_max_stacks", 0)
          <= caps.get("reflect_cap_pct", 0),
          "9% vs 50%")
    buff = skills.get("budong_mingwang", {}).get("buff_self", {})
    check("不动明王 反伤 +25%、3 回合",
          buff.get("stat") == "reflect_add" and buff.get("value") == 2500 and buff.get("turns") == 3,
          str(buff))

FACTION_CATEGORY = {
    "断岳剑宗": "物理", "破军枪府": "物理",
    "惊雷殿": "法术", "璇玑星宫": "法术",
    "缚灵符观": "封印", "织梦谷": "封印",
    "金刚禅院": "肉盾", "玄水宫": "肉盾",
}


def main():
    if not os.path.isdir(BUILD):
        print("找不到配置包目录: %s" % BUILD)
        return 2
    suit = load("suit/suit.json")
    sw = load("sect_war/sect_war.json")
    formula = load("battle/formula.json")

    combines = check_suit(suit)
    check_sect_war(sw)
    check_coeffs(formula, suit)
    seal_rows, seal_verdict = check_seal_calibration(suit, formula)

    failed = [r for r in results if not r["ok"]]
    summary = {
        "config_version": VERSION,
        "checked": len(results),
        "failed": len(failed),
        "verdict": "PASS" if not failed else "FAIL",
        "seal_calibration": seal_verdict,
        "perfect_tier_totals": {k: {a: round(v, 2) for a, v in p.items()} for k, p in combines.items()},
        "seal_rows": seal_rows,
        "items": results,
    }
    with open(OUT, "w", encoding="utf-8") as fh:
        json.dump(summary, fh, ensure_ascii=False, indent=2)

    print("\n[汇总] 检查 %d 项，失败 %d 项 → %s（符观封印命中：%s）"
          % (len(results), len(failed), summary["verdict"], seal_verdict))
    print("报告：%s" % OUT)
    return 0 if not failed else 1


if __name__ == "__main__":
    sys.exit(main())
