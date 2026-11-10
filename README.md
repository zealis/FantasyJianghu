# 幻想江湖 Fantasy Jianghu

横版侧视武侠 MMO 原型。客户端 **Godot 4.7（GDScript）**，服务端 **Go 1.22 + SQLite**，  
自定义 WebSocket 二进制协议（主码/子码分层，单源登记 + 双端生成）。

当前版本 **0.7.0-beta**，完整更新记录见 [CHANGELOG.md](CHANGELOG.md)。

## 已实现

- 账号注册登录、令牌会话、心跳与幂等、账号 ≤3 角色、创角八派与性别
- 世界进场快照、防瞬移钳制、10Hz 位置上报与权威纠偏、场景分线与切线
- 战斗：公式引擎（伤害/命中/控制/长局衰减）、帧流房间、30s 托管、断线恢复、  
  1v1 与 4v4、两段式出招、倍速与自动、V1.28 战斗飘字
- 任务：主线链 + 三类日常、接取/对话/击杀推进、交付发奖单事务、背包容量预检
- 表现：16 套门派×性别行走动画、按地点切换战斗背景、同屏玩家精灵与名牌

## 目录

```
client/            Godot 客户端（scripts / scenes / assets / tests）
server/            Go 服务端（cmd / internal / configs）
packages/          协议登记表与生成产物
数值模拟器/        战斗数值回归脚本
start-game.bat     一键启动（服务端 + 客户端，幂等，不重启已运行进程）
start-game.ps1     启动脚本实现（支持 -CheckOnly / -ServerOnly / -ClientOnly）
```

## 运行

前置：Go 1.22+、Godot 4.7（`start-game.ps1` 顶部 `$GodotExe` 指向本机可执行文件）。

```bash
# 服务端（首次需生成配置包）
cd server && go run ./cmd/gameserver -addr :7788 -build ./configs/build

# 客户端
godot --path client
```

Windows 直接双击 `start-game.bat`：幂等探测端口与窗口，已在运行的一端会跳过。  
服务端优先使用 `server/bin/gameserver.exe`（不存在或源码更新时才回落 `go run`）。

## 测试

```bash
# 服务端（内存紧张时限制并行度）
cd server && GOMAXPROCS=2 go test -p 1 ./...

# 客户端冒烟（真实渲染）
godot --path client res://tests/battle_smoke.tscn
godot --path client res://tests/walk_smoke.tscn
godot --path client res://tests/remote_smoke.tscn
godot --path client res://tests/quest_smoke.tscn
godot --path client res://tests/settings_smoke.tscn
```

## 仓库政策

不收录：美术素材库（`资产素材/`）、第三方原作（`原作/`）、内部文档与项目记录、  
数据库文件、编译产物、测试截图与输出。`client/assets/` 仅保留游戏运行必需的资源。
