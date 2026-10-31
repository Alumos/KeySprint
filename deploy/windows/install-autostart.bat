@echo off
net session >nul 2>&1
if errorlevel 1 (
  echo [需要管理员权限] 请右键本文件，选择“以管理员身份运行”。
  pause
  exit /b 1
)
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0install-autostart.ps1"
if errorlevel 1 (
  echo 注册失败，请查看上方错误信息。
) else (
  echo.
  echo 已注册开机自启：Windows 启动后自动运行 KeySprint，无需登录。
  echo 如需取消，运行 remove-autostart.bat。
)
pause
