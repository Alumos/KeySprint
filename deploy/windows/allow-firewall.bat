@echo off
net session >nul 2>&1
if errorlevel 1 (
  echo [需要管理员权限] 请右键本文件，选择“以管理员身份运行”。
  pause
  exit /b 1
)
set "PORT=8080"
netsh advfirewall firewall delete rule name="KeySprint (TCP %PORT%)" >nul 2>&1
netsh advfirewall firewall add rule name="KeySprint (TCP %PORT%)" dir=in action=allow protocol=TCP localport=%PORT% profile=any >nul
if errorlevel 1 (
  echo 放行失败，请确认脚本以管理员身份运行。
) else (
  echo 已放行 TCP %PORT%，学生电脑现在可以访问本机。
)
pause
