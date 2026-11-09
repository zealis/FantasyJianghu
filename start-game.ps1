<#
  幻想江湖 · 一键启动（服务端 + 客户端）
  - 双击 start-game.bat 即可；本文件也可直接运行。
  - 幂等：已在运行的服务端/客户端不会重复启动。
  - 绝不停止、杀死或重启任何已有进程（遵守「不要关闭服务端」约定）。
  可选参数：
    -CheckOnly    只检测并打印状态，不启动任何进程
    -ServerOnly   只补启动服务端
    -ClientOnly   只补启动客户端
#>
param(
    [switch]$CheckOnly,
    [switch]$ServerOnly,
    [switch]$ClientOnly
)

$ErrorActionPreference = 'Continue'
$Root      = Split-Path -Parent $PSCommandPath
$ServerDir = Join-Path $Root 'server'
$ClientDir = Join-Path $Root 'client'
$GodotExe  = 'D:\sdk\Godot\Godot_v4.7.2-stable_win64.exe'
$Port      = 7788

# 用真实 TCP 连接判断服务是否可用（比只查进程/端口表更可靠）。
function Test-GamePort {
    $c = New-Object System.Net.Sockets.TcpClient
    try {
        $ar = $c.BeginConnect('127.0.0.1', $Port, $null, $null)
        if ($ar.AsyncWaitHandle.WaitOne(600, $false) -and $c.Connected) { return $true }
        return $false
    } catch {
        return $false
    } finally {
        $c.Close()
    }
}

# 以「GUI 窗口标题」判断游戏客户端：比 CIM 命令行过滤更可靠。
# 游戏窗口 = 幻想江湖 Fantasy Jianghu (DEBUG)；编辑器标题带 Godot Engine，headless 无窗口。
function Get-GameClient {
    return @(Get-Process -Name 'Godot_v4.7.2-stable_win64' -ErrorAction SilentlyContinue |
        Where-Object {
            $_.MainWindowTitle -like '*Fantasy Jianghu*' -and
            $_.MainWindowTitle -notlike '*Godot Engine*'
        })
}

$serverUp = Test-GamePort
$clientUp = (Get-GameClient).Count -gt 0

Write-Host '=============================================='
Write-Host '  Fantasy-Jianghu 一键启动'
Write-Host '=============================================='
Write-Host "  工作区 : $Root"
Write-Host "  服务端 : $(if ($serverUp) { '运行中（跳过，不重启/不关闭）' } else { '未运行' })"
Write-Host "  客户端 : $(if ($clientUp) { '窗口已开' } else { '未打开' })"
Write-Host '----------------------------------------------'

if ($CheckOnly) {
    Write-Host '  CheckOnly：未启动任何进程'
    exit 0
}

# --- 服务端：仅在端口未监听时启动，独立控制台窗口，关闭该窗口才会停服务 ---
if (-not $serverUp -and -not $ClientOnly) {
    if (-not (Test-Path $ServerDir)) {
        Write-Host "  [错误] 找不到服务端目录：$ServerDir"
        exit 1
    }
    # 优先用预编译二进制：go run 会多一层 cmd 子进程，进程树被宿主回收时服务端会跟着退出。
    # 仅当二进制不存在、或 Go 源码比它新时才回落 go run（保证代码最新）。
    $serverExe = Join-Path $ServerDir 'bin\gameserver.exe'
    $useExe = $false
    if (Test-Path $serverExe) {
        $exeTime = (Get-Item $serverExe).LastWriteTime
        $staleGo = @(Get-ChildItem -Path $ServerDir -Recurse -Filter *.go -ErrorAction SilentlyContinue |
            Where-Object { $_.LastWriteTime -gt $exeTime })
        if ($staleGo.Count -eq 0) { $useExe = $true }
    }
    if ($useExe) {
        Start-Process -FilePath $serverExe -ArgumentList @('-addr', ':7788', '-build', './configs/build') -WorkingDirectory $ServerDir | Out-Null
        Write-Host '  服务端启动中（预编译二进制，新控制台窗口，请勿关闭）...'
    } else {
        $cmdLine = '/k cd /d "' + $ServerDir + '" && go run ./cmd/gameserver -addr :7788 -build ./configs/build'
        Start-Process -FilePath 'cmd.exe' -ArgumentList $cmdLine -WorkingDirectory $ServerDir | Out-Null
        Write-Host '  服务端启动中（go run：二进制缺失或源码已更新，新控制台窗口，请勿关闭）...'
    }
    for ($i = 0; $i -lt 40 -and -not (Test-GamePort); $i++) {
        Start-Sleep -Milliseconds 500
    }
    if (Test-GamePort) {
        Write-Host '  服务端已就绪 :7788'
    } else {
        Write-Host '  [警告] 20 秒内未监听 :7788，请看服务端窗口日志'
    }
}

# --- 客户端：仅在窗口未打开时启动 ---
if (-not $clientUp -and -not $ServerOnly) {
    if (-not (Test-Path $GodotExe)) {
        Write-Host "  [错误] 找不到 Godot：$GodotExe（请修改脚本顶部 `$GodotExe）"
        exit 1
    }
    $script:clientProc = Start-Process -FilePath $GodotExe -ArgumentList @('--path', $ClientDir) -WorkingDirectory $ClientDir -PassThru
    Write-Host '  客户端窗口启动中...'
}

# 等待客户端进程稳定（最多 8 秒）再报告，避免刚 Start-Process 就判断成"未检测到"。
for ($i = 0; $i -lt 16; $i++) {
    if ((Get-GameClient).Count -gt 0) { break }
    Start-Sleep -Milliseconds 500
}
$clientOk = (Get-GameClient).Count -gt 0 -or ($script:clientProc -and -not $script:clientProc.HasExited)
Write-Host '----------------------------------------------'
Write-Host "  最终状态：服务端 $(if (Test-GamePort) {'OK'} else {'未就绪'}) / 客户端 $(if ($clientOk) {'已启动'} else {'未检测到（请确认 Godot 路径）'})"
