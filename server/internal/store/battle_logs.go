package store

import (
	"context"
	"database/sql"
)

// BattleLogsRepo 战斗日志（种子/指令流/结果——复盘口径，技术规范 §8.2）。
type BattleLogsRepo struct{ db *sql.DB }

func (r *BattleLogsRepo) AppendBattle(ctx context.Context, seed int64, participants, cmdStream, result string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO battle_logs (seed, participants, cmd_stream, result, created_at) VALUES (?, ?, ?, ?, strftime('%s','now'))`,
		seed, participants, cmdStream, result)
	return err
}

func (r *BattleLogsRepo) GetLatest(ctx context.Context) (seed int64, participants, cmdStream, result string, err error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT seed, participants, cmd_stream, result FROM battle_logs ORDER BY id DESC LIMIT 1`)
	var cs, res string
	err = row.Scan(&seed, &participants, &cs, &res)
	return seed, participants, cs, res, err
}
