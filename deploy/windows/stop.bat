@echo off
taskkill /IM keysprint.exe /F >nul 2>&1
if errorlevel 1 (
  echo 没有找到正在运行的 KeySprint 服务。
) else (
  echo KeySprint 服务已停止。
)
pause
