// gameserver 为 M1 服务端装配（开发技术规范 §3：Gateway 与游戏服同进程不同包）：
//
//	go run ./cmd/gameserver -addr :7788 -build ./configs/build
//
// 装配顺序：配置包加载（失败即退出，不回退默认值）→ SQLite → KV（Redis/内存）
// → 会话 → 路由与网关 → 三域服务 → HTTP+WS 同端口 → 30s 位置 sweep。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"fantasy-jianghu/server/internal/account"
	"fantasy-jianghu/server/internal/battle"
	"fantasy-jianghu/server/internal/character"
	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/gateway"
	"fantasy-jianghu/server/internal/item"
	"fantasy-jianghu/server/internal/kv"
	"fantasy-jianghu/server/internal/npc"
	"fantasy-jianghu/server/internal/protocol"
	"fantasy-jianghu/server/internal/quest"
	"fantasy-jianghu/server/internal/router"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
	"fantasy-jianghu/server/internal/world"
)

// Options 为装配参数（main 的 flag 与集成测试共用）。
type Options struct {
	Addr      string
	DSN       string // SQLite 数据源；默认 ./data/game.db
	RedisAddr string // 空 = 内存 KV（开发默认），否则 Redis
	BuildRoot string // 配置包根目录（取最新版本）
	ConfigDir string // 精确配置包目录（优先于 BuildRoot）
}

// ServerHandle 为装配产物：测试可不起端口直接挂 Handler。
type ServerHandle struct {
	Handler http.Handler
	DB      *store.DB
	Stop    func()
}

func buildServer(opts Options) (*ServerHandle, error) {
	// 1. 配置包：加载与复核失败即退出（§7 配置非法不使用默认值回退）。
	dir := opts.ConfigDir
	if dir == "" {
		var err error
		dir, err = pickLatestConfigDir(opts.BuildRoot)
		if err != nil {
			return nil, fmt.Errorf("定位配置包: %w", err)
		}
	}
	data, err := config.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("配置加载失败: %w", err)
	}
	log.Printf("config loaded dir=%s version=%s scenes=%d", dir, data.Version, len(data.Scenes))

	// 2. 主库（§2.1 WAL 口径在 store.Open 内）。
	dsn := opts.DSN
	if dsn == "" {
		dsn = "./data/game.db"
		if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := store.Open(dsn)
	if err != nil {
		return nil, fmt.Errorf("打开主库: %w", err)
	}
	if err := db.Migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("迁移: %w", err)
	}

	// 3. KV：REDIS_ADDR/opts 指定则用 Redis，否则内存（§2.2 边界）。
	var st kv.Store
	if opts.RedisAddr != "" {
		st, err = kv.NewRedis(opts.RedisAddr)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("连接 Redis: %w", err)
		}
		log.Printf("kv redis addr=%s", opts.RedisAddr)
	} else {
		st = kv.NewMemory()
		log.Printf("kv memory")
	}

	// 4. 会话 / 路由 / 网关。
	sessions := session.NewManager(st, 24*time.Hour)
	var rt router.Router
	gw := gateway.NewServer(&rt, sessions, gateway.Cfg{
		HeartbeatSeconds: data.App.HeartbeatSeconds,
		MissLimit:        data.App.HeartbeatMissLimit,
	})

	// 5. 业务域服务并登记（主码登记制）。
	battleMgr := battle.NewManager(data.Battle, db.Battles)
	acct := account.NewService(db.Accounts, db.Characters, sessions, st, data)
	battleSvc := battle.NewService(battleMgr, db.Characters, data)
	chars := character.NewService(db.Characters, sessions, data)
	worldSvc := world.NewService(db.Characters, sessions, data, gw)
	// M2b 任务域：进度来源为战斗终局（kill）、NPC 对话（talk）、进场（visit）。
	questSvc := quest.NewService(db.Quests, db.Characters, db.Items, data, gw, protocol.SubQuestProgress)
	itemSvc := item.NewService(db.Items, data)
	npcSvc := npc.NewService(data, questSvc)
	worldSvc.OnEnter = func(charID int64, mapID string) {
		_ = questSvc.RecordProgress(context.Background(), charID, "visit", mapID, 1)
	}
	// 掉线清理：连接关闭（非正常 3/4 离场）时移除玩家并广播，防幽灵玩家。
	gw.OnClose = func(token string) {
		worldSvc.HandleDisconnect(context.Background(), token)
	}
	rt.Register(protocol.MainAccount, protocol.SubAuth, acct.HandleWSAuth)
	rt.Register(protocol.MainAccount, protocol.SubCharCreate, chars.HandleCreate)
	rt.Register(protocol.MainAccount, protocol.SubCharSelect, chars.HandleSelect)
	rt.Register(protocol.MainWorld, protocol.SubEnterScene, worldSvc.HandleEnter)
	rt.Register(protocol.MainWorld, protocol.SubMove, worldSvc.HandleMove)
	rt.Register(protocol.MainWorld, protocol.SubLeave, worldSvc.HandleLeave)
	rt.Register(protocol.MainWorld, protocol.SubLineList, worldSvc.HandleLineList)
	rt.Register(protocol.MainWorld, protocol.SubSwitchLine, worldSvc.HandleSwitchLine)
	rt.Register(protocol.MainCode4, protocol.SubBattleEnter, battleSvc.HandleEnter)
	rt.Register(protocol.MainCode4, protocol.SubBattleCmd, battleSvc.HandleCmd)
	rt.Register(protocol.MainCode4, protocol.SubBattleAck, battleSvc.HandleAck)
	rt.Register(protocol.MainCode4, protocol.SubBattleRestore, battleSvc.HandleRestore)
	rt.Register(protocol.MainCode5, protocol.SubBagList, itemSvc.HandleList)
	rt.Register(protocol.MainCode10, protocol.SubNpcTalk, npcSvc.HandleTalk)
	rt.Register(protocol.MainCode11, protocol.SubQuestList, questSvc.HandleList)
	rt.Register(protocol.MainCode11, protocol.SubQuestAccept, questSvc.HandleAccept)
	rt.Register(protocol.MainCode11, protocol.SubQuestSubmit, questSvc.HandleSubmit)
	rt.Register(protocol.MainCode11, protocol.SubQuestDaily, questSvc.HandleDaily)

	// 6. HTTP + WS 同端口。
	mux := http.NewServeMux()
	acct.MountHTTP(mux)
	mux.Handle("/ws", gw)

	// 帧广播：战斗房间事件 → 按参与者令牌推送（gateway Hub）。
	// 投递描述由 Room 在锁内快照生成，此处不再反查 Room/Participants，杜绝自死锁与 ABBA。
	battleMgr.SetFrameHook(func(d battle.FrameDelivery) {
		for _, token := range d.Tokens {
			gw.SendTo(token, protocol.MainCode4, protocol.SubBattleFrame, map[string]any{
				"turn_version": d.TurnVersion, "events": d.Events,
			})
		}
		// 终局另发 4/5（协议登记的子码，此前只有 end 事件内联在帧里）。
		if d.Result != nil {
			for _, token := range d.Tokens {
				gw.SendTo(token, protocol.MainCode4, protocol.SubBattleResult, d.Result)
			}
		}
		// 任务进度：按"本场阵亡的野怪"推进 kill 目标（M2b）。
		// 口径：切磋/演练场同归于尽也算击杀，否则 Lv1 角色打不过木桩人会永久卡任务。
		if d.Outcome != nil && len(d.Outcome.Killed) > 0 {
			for _, cid := range d.Outcome.CharIDs {
				for _, mid := range d.Outcome.Killed {
					_ = questSvc.RecordProgress(context.Background(), cid, "kill", mid, 1)
				}
			}
		}
	})
	// 4/4 确认落后 → 补发未确认帧（不重算）。
	battleMgr.SetResendHook(func(token string, turnVersion int, events []battle.Event) {
		gw.SendTo(token, protocol.MainCode4, protocol.SubBattleFrame, map[string]any{
			"turn_version": turnVersion, "events": events, "resend": true,
		})
	})

	// 7. 30s 位置 sweep（离场即写为主，此为兜底）+ 战斗托管 tick。
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		tk := time.NewTicker(30 * time.Second)
		defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
				if err := worldSvc.Sweep(context.Background()); err != nil {
					log.Printf("sweep error: %v", err)
				}
				battleMgr.Tick()
			}
		}
	}()

	return &ServerHandle{
		Handler: mux,
		DB:      db,
		Stop: func() {
			cancel()
			<-done
			db.Close()
		},
	}, nil
}

// pickLatestConfigDir 在 build 根下取最高版本号目录（简单数值逐段比较）。
func pickLatestConfigDir(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) == 0 {
		return "", fmt.Errorf("build 目录无版本: %s", root)
	}
	sort.Slice(dirs, func(i, j int) bool { return lessVersion(dirs[i], dirs[j]) })
	return filepath.Join(root, dirs[len(dirs)-1]), nil
}

func lessVersion(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		ai, _ := strconv.Atoi(as[i])
		bi, _ := strconv.Atoi(bs[i])
		if ai != bi {
			return ai < bi
		}
	}
	return len(as) < len(bs)
}

func main() {
	opts := Options{}
	flag.StringVar(&opts.Addr, "addr", ":7788", "监听地址（HTTP + WS 同端口）")
	flag.StringVar(&opts.DSN, "dsn", "", "SQLite 数据源（默认 ./data/game.db）")
	flag.StringVar(&opts.RedisAddr, "redis", os.Getenv("REDIS_ADDR"), "Redis 地址（默认内存 KV）")
	flag.StringVar(&opts.BuildRoot, "build", "./configs/build", "配置包根目录（取最新版本）")
	flag.StringVar(&opts.ConfigDir, "config", "", "精确配置包目录（优先）")
	flag.Parse()

	h, err := buildServer(opts)
	if err != nil {
		log.Fatalf("启动失败: %v", err)
	}
	defer h.Stop()
	log.Printf("gameserver listening %s", opts.Addr)
	if err := http.ListenAndServe(opts.Addr, h.Handler); err != nil {
		log.Fatalf("http: %v", err)
	}
}
