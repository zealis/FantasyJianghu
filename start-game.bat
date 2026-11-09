@echo off
chcp 65001 >nul
title Fantasy-Jianghu Launcher
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-game.ps1" %*
echo.
echo Done. You can close this launcher window.
pause >nul
