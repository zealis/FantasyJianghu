// Package store 为主库唯一访问入口（开发技术规范 §2.1）：全部 SQL 走仓储层，
// 业务层禁止拼写方言 SQL；表结构与查询只用 SQLite/PostgreSQL 兼容子集。
package store

import (
	"database/sql"

	_ "modernc.org/sqlite" // 纯 Go 驱动：本机无 CGO 工具链
)

type DB struct {
	sql        *sql.DB
	Accounts   *AccountsRepo
	Characters *CharactersRepo
	Currency   *CurrencyRepo
	Battles    *BattleLogsRepo
	Quests     *QuestRepo
	Items      *ItemsRepo
	Equips     *EquipsRepo
}

// Open 打开主库并按 §2.1 口径设 PRAGMA（WAL、busy_timeout、外键）。
// 连接池收敛为 1：写操作经数据访问层天然串行，避免 SQLITE_BUSY。
func Open(dsn string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	for _, p := range []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := sqlDB.Exec(p); err != nil {
			sqlDB.Close()
			return nil, err
		}
	}
	sqlDB.SetMaxOpenConns(1)
	db := &DB{sql: sqlDB}
	db.Accounts = &AccountsRepo{db: sqlDB}
	db.Characters = &CharactersRepo{db: sqlDB}
	db.Currency = &CurrencyRepo{db: sqlDB}
	db.Battles = &BattleLogsRepo{db: sqlDB}
	db.Quests = &QuestRepo{db: sqlDB}
	db.Items = &ItemsRepo{db: sqlDB}
	db.Equips = &EquipsRepo{db: sqlDB}
	return db, nil
}

func (d *DB) Close() error { return d.sql.Close() }

// SQL 暴露底层句柄（测试构造旧版本库用；业务层禁用——SQL 只走仓储层）。
func (d *DB) SQL() *sql.DB { return d.sql }
