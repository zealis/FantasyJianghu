package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadRegistry 读真实的单源登记表，测试与生成器共用同一份数据。
func loadRegistry(t *testing.T) Registry {
	t.Helper()
	root := projectRoot(t)
	reg, err := LoadRegistry(filepath.Join(root, "packages", "protocol", "protocol.json"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	return reg
}

func projectRoot(t *testing.T) string {
	t.Helper()
	// 测试运行于 server/cmd/protogen，仓库根在其上三级。
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "packages", "protocol", "protocol.json")); err != nil {
		t.Fatalf("仓库根定位失败: %v", err)
	}
	return root
}

func TestGenerateGDScriptMatchesClientFile(t *testing.T) {
	reg := loadRegistry(t)
	got, err := GenerateGDScript(reg)
	if err != nil {
		t.Fatalf("GenerateGDScript: %v", err)
	}
	goldenPath := filepath.Join(projectRoot(t), "client", "scripts", "net", "protocol.gd")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("读 golden 文件: %v", err)
	}
	norm := func(b []byte) string { return strings.ReplaceAll(string(b), "\r\n", "\n") }
	if norm([]byte(got)) != norm(want) {
		t.Fatalf("生成的 GDScript 与 client/scripts/net/protocol.gd 不一致（golden 漂移）")
	}
}

func TestGenerateGDScriptSubCodes(t *testing.T) {
	reg := loadRegistry(t)
	got, err := GenerateGDScript(reg)
	if err != nil {
		t.Fatalf("GenerateGDScript: %v", err)
	}
	// 登记表对客户端必须有约束：子码常量同源生成，不再靠硬编码数字。
	for _, s := range []string{
		"const SUB_BATTLE_ENTER := 1", "const SUB_BATTLE_CMD := 2",
		"const SUB_BATTLE_FRAME := 3", "const SUB_BATTLE_ACK := 4",
		"const SUB_BATTLE_RESULT := 5", "const SUB_BATTLE_RESTORE := 7",
		"const SUB_ENTER_SCENE := 1", "const SUB_MOVE := 2", "const SUB_KICK := 1",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("生成的 GDScript 缺子码常量 %q", s)
		}
	}
}

func TestGenerateGoRegistry(t *testing.T) {
	reg := loadRegistry(t)
	got, err := GenerateGo(reg)
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}
	for _, s := range []string{
		"MainSystem = 1", "MainAccount = 2", "MainWorld = 3",
		"HeartbeatSeconds = 15", "HeartbeatMissLimit = 3",
		"SubPing = 0", "SubKick = 1", "SubAuth = 1",
		"SubCharCreate = 2", "SubCharSelect = 3",
		"SubEnterScene = 1", "SubMove = 2", "SubMoveBroadcast = 3",
		"SubLeave = 4", "SubLeaveBroadcast = 5", "SubPlayerJoin = 6",
		"SubLineList = 7", "SubSwitchLine = 8",
		"SubBattleEnter = 1", "SubBattleCmd = 2", "SubBattleFrame = 3",
		"SubBattleAck = 4", "SubBattleResult = 5", "SubBattleLeave = 6", "SubBattleRestore = 7",
		`"系统/心跳/踢下线"`, `"抽奖"`,
	} {
		if !strings.Contains(got, s) {
			t.Errorf("生成的 Go 登记表缺少 %q", s)
		}
	}
	if len(reg.MainCodes) != 20 {
		t.Errorf("主码登记数 = %d, 期望 20", len(reg.MainCodes))
	}
}
