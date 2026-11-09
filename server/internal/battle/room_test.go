package battle

import (
	"context"
	"testing"
	"time"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/store"
)

func roomCfg() config.BattleData {
	return config.BattleData{Formula: formulaCfg()}
}

type captured struct {
	turnVersion int
	events      []Event
}

func newTestRoom(t *testing.T, now *fakeClock) (*Room, *Manager, *[]captured, *store.DB) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	m := NewManager(roomCfg(), db.Battles)
	m.now = func() time.Time { return now.Now() }
	var caps []captured
	m.onFrame = func(d FrameDelivery) {
		caps = append(caps, captured{d.TurnVersion, d.Events})
	}
	a := mkUnit(1, "甲", 0, 1422, 376, 5000)
	b := mkUnit(2, "乙", 1, 1000, 376, 5000)
	room := m.Create(42, []*Unit{a}, []*Unit{b}, map[string]int64{"tokA": 1, "tokB": 2})
	return room, m, &caps, db
}

func TestBothCmdsResolveImmediately(t *testing.T) {
	now := &fakeClock{now: time.Now()}
	room, _, caps, _ := newTestRoom(t, now)
	room.OnCmd("tokA", Cmd{Type: "attack"})
	if len(*caps) != 0 {
		t.Fatal("单方指令不应结算")
	}
	room.OnCmd("tokB", Cmd{Type: "attack"})
	if len(*caps) != 1 {
		t.Fatalf("收齐指令应立即结算广播: %d 帧", len(*caps))
	}
	if (*caps)[0].turnVersion != 1 || len((*caps)[0].events) == 0 {
		t.Fatalf("帧内容不完整: %+v", (*caps)[0])
	}
}

func TestTimeoutHostedAttack(t *testing.T) {
	now := &fakeClock{now: time.Now()}
	room, m, caps, _ := newTestRoom(t, now)
	room.OnCmd("tokA", Cmd{Type: "attack"})
	// 30 秒后 Tick → 乙方托管普攻（Focus 5）。
	now.Advance(31 * time.Second)
	m.Tick()
	if len(*caps) != 1 {
		t.Fatalf("超时后应结算: %d 帧", len(*caps))
	}
	hosted := false
	for _, e := range (*caps)[0].events {
		if e.Type == "fallback" && e.Actor == 2 {
			hosted = true
		}
	}
	if !hosted {
		t.Fatalf("乙方应被托管普攻并标记: %+v", (*caps)[0].events)
	}
}

func TestAckBehindResendsWithoutReresolve(t *testing.T) {
	now := &fakeClock{now: time.Now()}
	room, _, caps, _ := newTestRoom(t, now)
	// 回合 1。
	room.OnCmd("tokA", Cmd{Type: "attack"})
	room.OnCmd("tokB", Cmd{Type: "attack"})
	// 甲确认回合 1；乙全程未确认（补发口径验证）。
	// 回合 2 结算（乙确认过 0 直接补指令——模拟乙收不到帧仍出指令）。
	room.OnCmd("tokA", Cmd{Type: "attack"})
	room.OnCmd("tokB", Cmd{Type: "attack"})
	if len(*caps) != 2 {
		t.Fatalf("两回合两帧: %d", len(*caps))
	}
	events2 := append(append([]Event{}, (*caps)[0].events...), (*caps)[1].events...)
	before := len(*caps)
	// 乙全程未确认 → pendingFor 应返回全部累计事件（补发不重算：caps 数量不变）。
	if len(*caps) != before {
		t.Fatal("补发不得重新结算")
	}
	resent := room.pendingFor("tokB")
	if len(resent) != len(events2) {
		t.Fatalf("乙应收到 %d 事件补发, 收 %d", len(events2), len(resent))
	}
	for i := range resent {
		if resent[i].Type != events2[i].Type || resent[i].Seq != events2[i].Seq ||
			resent[i].Value != events2[i].Value || resent[i].Turn != events2[i].Turn {
			t.Fatalf("补发事件序列不一致 @%d", i)
		}
	}
}

func TestRestoreSnapshot(t *testing.T) {
	now := &fakeClock{now: time.Now()}
	room, m, _, _ := newTestRoom(t, now)
	room.OnCmd("tokA", Cmd{Type: "attack"})
	room.OnCmd("tokB", Cmd{Type: "attack"})
	room.OnCmd("tokA", Cmd{Type: "attack"})
	room.OnCmd("tokB", Cmd{Type: "attack"})
	snap, pending, ok := m.Restore("tokA")
	if !ok {
		t.Fatal("断线恢复失败")
	}
	if snap["battle_id"] != room.ID {
		t.Fatalf("快照缺 battle_id: %+v", snap)
	}
	if len(pending) == 0 {
		t.Fatal("恢复应携带未确认帧")
	}
}

func TestBattleLogsWritten(t *testing.T) {
	now := &fakeClock{now: time.Now()}
	room, _, _, db := newTestRoom(t, now)
	room.OnCmd("tokA", Cmd{Type: "attack"})
	room.OnCmd("tokB", Cmd{Type: "attack"})
	seed, participants, cmdStream, _, err := db.Battles.GetLatest(context.Background())
	if err != nil {
		t.Fatalf("battle_logs 未落库: %v", err)
	}
	if seed != 42 {
		t.Fatalf("seed = %d, 期望 42", seed)
	}
	if participants == "" || cmdStream == "" {
		t.Fatal("participants/cmd_stream 为空")
	}
}

// ---- 独立评审回归：生产帧投递 / 补发 / 房间生命周期 ----

// TestProductionFrameHookNoDeadlock 复现旧生产装配的 hook 形态：
// hook 内反查 Manager.Room(roomID).Participants()。旧实现在 OnCmd 持有 Room.mu 时
// 同步调 hook → Participants() 二次抢同一把非重入锁 → 第一帧即永久死锁。
// 修复后投递在锁外，且 hook 直接拿 Tokens，必须正常返回。
func TestProductionFrameHookNoDeadlock(t *testing.T) {
	now := &fakeClock{now: time.Now()}
	room, m, caps, _ := newTestRoom(t, now)
	room.onFrameHook = func(d FrameDelivery) {
		r := m.Room(d.RoomID)
		if r != nil {
			_ = r.Participants() // 旧生产 hook 的反查动作
		}
		*caps = append(*caps, captured{d.TurnVersion, d.Events})
	}
	done := make(chan struct{})
	go func() {
		room.OnCmd("tokA", Cmd{Type: "attack"})
		room.OnCmd("tokB", Cmd{Type: "attack"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("帧广播 hook 反查 Room 仍会自死锁（生产装配第一帧即挂死）")
	}
	if len(*caps) != 1 {
		t.Fatalf("应广播 1 帧, 实得 %d", len(*caps))
	}
}

// TestFrameDeliveryCarriesTokensAndResult 终局帧必须自带参与者与结果，
// 装配层据此推 4/5，无需再反查 Room。
func TestFrameDeliveryCarriesTokensAndResult(t *testing.T) {
	now := &fakeClock{now: time.Now()}
	room, _, _, _ := newTestRoom(t, now)
	var got []FrameDelivery
	room.onFrameHook = func(d FrameDelivery) { got = append(got, d) }
	room.B.SetRng(flatRng(0.0))
	room.B.Units[1].HP = 1 // 一击致命，触发终局
	room.OnCmd("tokA", Cmd{Type: "attack"})
	room.OnCmd("tokB", Cmd{Type: "attack"})
	if len(got) != 1 {
		t.Fatalf("应广播 1 帧, 实得 %d", len(got))
	}
	if len(got[0].Tokens) != 2 {
		t.Fatalf("投递应自带 2 个参与者令牌: %+v", got[0].Tokens)
	}
	if got[0].Result == nil || got[0].Result["winner"] != 0 {
		t.Fatalf("终局投递缺结果: %+v", got[0].Result)
	}
}

// TestAckBehindTriggersResend 4/4 确认落后 → 返回未确认帧并触发补发 hook（不重算）。
func TestAckBehindTriggersResend(t *testing.T) {
	now := &fakeClock{now: time.Now()}
	room, m, caps, _ := newTestRoom(t, now)
	room.OnCmd("tokA", Cmd{Type: "attack"})
	room.OnCmd("tokB", Cmd{Type: "attack"})
	room.OnCmd("tokA", Cmd{Type: "attack"})
	room.OnCmd("tokB", Cmd{Type: "attack"})
	before := len(*caps)

	cur, pending := room.OnAck("tokB", 0)
	if len(pending) == 0 {
		t.Fatal("确认落后应返回待补发事件")
	}
	var resent int
	m.SetResendHook(func(_ string, _ int, events []Event) { resent = len(events) })
	m.Resend("tokB", cur, pending)
	if resent != len(pending) {
		t.Fatalf("补发事件数 %d != 待补发 %d", resent, len(pending))
	}
	if len(*caps) != before {
		t.Fatal("补发不得重新结算")
	}
	// 旧确认号不得回退覆盖新进度。
	room.OnAck("tokB", cur)
	_, _ = room.OnAck("tokB", 0)
	if room.acked["tokB"] != cur {
		t.Fatal("旧确认号回退了 ack 进度")
	}
	_, none := room.OnAck("tokB", cur)
	if len(none) != 0 {
		t.Fatal("已确认当前回合后不应再补发")
	}
}

// TestCreateClosesOldRoom 同一令牌重复开局不得留下旧房（否则 4/2 打进已结束战斗）。
func TestCreateClosesOldRoom(t *testing.T) {
	now := &fakeClock{now: time.Now()}
	old, m, _, _ := newTestRoom(t, now)
	fresh := m.Create(43, []*Unit{mkUnit(3, "丙", 0, 1000, 376, 5000)},
		[]*Unit{mkUnit(4, "丁", 1, 1000, 376, 5000)},
		map[string]int64{"tokA": 3, "tokB": 4})
	if !old.closed {
		t.Fatal("旧房应被关闭")
	}
	if got := m.RoomByToken("tokA"); got != fresh {
		t.Fatal("RoomByToken 应命中新房")
	}
	if m.Room(old.ID) != nil {
		t.Fatal("旧房应从集合移除")
	}
}

// TestRoomReapedAfterFinish 终局房间在确认窗口后回收，避免 rooms 只增不减。
func TestRoomReapedAfterFinish(t *testing.T) {
	now := &fakeClock{now: time.Now()}
	room, m, _, _ := newTestRoom(t, now)
	room.B.SetRng(flatRng(0.0))
	room.B.Units[1].HP = 1
	room.OnCmd("tokA", Cmd{Type: "attack"})
	room.OnCmd("tokB", Cmd{Type: "attack"})
	if !room.B.Over {
		t.Fatal("战斗应已终局")
	}
	// 终局后继续收指令不得再结算。
	room.OnCmd("tokA", Cmd{Type: "attack"})
	if room.B.Turn != 1 {
		t.Fatalf("终局后不应继续结算: turn=%d", room.B.Turn)
	}
	now.Advance(3 * time.Minute)
	m.Tick()
	if !room.closed || m.Room(room.ID) != nil {
		t.Fatal("终局房间应被回收")
	}
}
