@echo off
net session >nul 2>&1
if errorlevel 1 (
  echo [需要管理员权限] 请右键本文件，选择“以管理员身份运行”。
  pause
  exit /b 1
)
schtasks /delete /tn "KeySprint" /f >nul 2>&1
if errorlevel 1 (
  echo 未找到开机自启任务（可能尚未安装）。
) else (
  echo 已移除开机自启任务。
)
pause
