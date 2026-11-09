package store

import (
	"fmt"
	"time"
)

// migrations 为版本化建表清单，只追加不修改（§6.1/§6.2/§6.3 口径）。
var migrations = []string{
	// v1: M1 基础表
	`CREATE TABLE IF NOT EXISTS accounts (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		username      TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		vip_level     INTEGER NOT NULL DEFAULT 0,
		ban_reason    TEXT NOT NULL DEFAULT '',
		ban_until     INTEGER NOT NULL DEFAULT 0,
		created_at    INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS characters (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL REFERENCES accounts(id),
		name       TEXT NOT NULL UNIQUE,
		faction    TEXT NOT NULL,
		level      INTEGER NOT NULL DEFAULT 1,
		exp        INTEGER NOT NULL DEFAULT 0,
		map_id     TEXT NOT NULL DEFAULT '',
		pos_x      REAL NOT NULL DEFAULT 400,
		pos_y      REAL NOT NULL DEFAULT 900,
		created_at INTEGER NOT NULL,
		deleted_at INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS currency_logs (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		character_id  INTEGER NOT NULL REFERENCES characters(id),
		currency      TEXT NOT NULL,
		delta         INTEGER NOT NULL,
		balance_after INTEGER NOT NULL,
		reason        TEXT NOT NULL,
		ref_id        TEXT NOT NULL DEFAULT '',
		created_at    INTEGER NOT NULL
	)`,
	// v2: M1.5 创角两步流——角色性别（男/女，默认男兼容旧数据）、账号昵称（§9 注册界面）。
	`ALTER TABLE characters ADD COLUMN gender TEXT NOT NULL DEFAULT '男'`,
	`ALTER TABLE accounts ADD COLUMN nickname TEXT NOT NULL DEFAULT ''`,
	// v4: M2a 战斗日志（种子/指令流/结果——复盘口径 §8.2）。
	`CREATE TABLE IF NOT EXISTS battle_logs (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		seed         INTEGER NOT NULL,
		participants TEXT NOT NULL,
		cmd_stream   TEXT NOT NULL,
		result       TEXT NOT NULL DEFAULT '',
		created_at   INTEGER NOT NULL
	)`,
	// v7: M2b 任务存档（日常可重复接取 → 每次接取一行）。
	`CREATE TABLE IF NOT EXISTS character_quests (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		character_id INTEGER NOT NULL REFERENCES characters(id),
		quest_id     TEXT NOT NULL,
		kind         TEXT NOT NULL,
		state        TEXT NOT NULL DEFAULT 'active',
		progress     TEXT NOT NULL DEFAULT '[]',
		accepted_at  INTEGER NOT NULL,
		completed_at INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE INDEX IF NOT EXISTS ix_quests_char_state ON character_quests(character_id, state)`,
	`CREATE INDEX IF NOT EXISTS ix_quests_char_quest ON character_quests(character_id, quest_id)`,
	// v8: M2b 背包格子（一行一格，单格上限取配置 stack；装备实例属性 M2c）。
	`CREATE TABLE IF NOT EXISTS character_items (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		character_id INTEGER NOT NULL REFERENCES characters(id),
		item_id      TEXT NOT NULL,
		count        INTEGER NOT NULL,
		created_at   INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS ix_items_char ON character_items(character_id, item_id)`,
	// v9: M2b 钱包余额（变动同时写 currency_logs，§6.2 双份审计）。
	`CREATE TABLE IF NOT EXISTS character_wallets (
		character_id INTEGER PRIMARY KEY REFERENCES characters(id),
		gold         INTEGER NOT NULL DEFAULT 0,
		xiuwei       INTEGER NOT NULL DEFAULT 0,
		yuanbao      INTEGER NOT NULL DEFAULT 0,
		updated_at   INTEGER NOT NULL
	)`,
	// v10: M2b 三类日常计数（按服务器日期 UTC+8 分桶，GDD §9.2）。
	`CREATE TABLE IF NOT EXISTS character_daily (
		character_id INTEGER NOT NULL REFERENCES characters(id),
		day          TEXT NOT NULL,
		category     TEXT NOT NULL,
		count        INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (character_id, day, category)
	)`,
}

// Migrate 依次应用未执行的迁移；重复执行幂等。
func (d *DB) Migrate() error {
	if _, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS migrations (
		version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	var current int
	if err := d.sql.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM migrations`).Scan(&current); err != nil {
		return err
	}
	for i, m := range migrations {
		v := i + 1
		if v <= current {
			continue
		}
		if _, err := d.sql.Exec(m); err != nil {
			return fmt.Errorf("迁移 v%d: %w", v, err)
		}
		if _, err := d.sql.Exec(`INSERT INTO migrations (version, applied_at) VALUES (?, ?)`, v, nowUnix()); err != nil {
			return err
		}
	}
	return nil
}

func timeNow() time.Time { return time.Now() }

func nowUnix() int64 { return timeNow().Unix() }
