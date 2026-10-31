@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"

if not exist "%~dp0keysprint.exe" (
  echo [错误] 找不到 keysprint.exe，请先把 zip 完整解压后再运行。
  pause
  exit /b 1
)
if not exist "%~dp0web\index.html" (
  echo [错误] 找不到 web\index.html，请先把 zip 完整解压后再运行。
  pause
  exit /b 1
)

if not exist "%~dp0config.env" (
  echo 首次运行：正在生成配置 config.env ...
  for /f "usebackq delims=" %%k in (`powershell -NoProfile -Command "$r=[System.Security.Cryptography.RandomNumberGenerator]::Create();$b=New-Object byte[] 32;$r.GetBytes($b);-join ($b|ForEach-Object {$_.ToString('x2')})"`) do set "GENERATED=%%k"
  if not defined GENERATED set "GENERATED=please-change-this-secret"
  > "%~dp0config.env" echo # KeySprint 局域网配置（首次运行自动生成；修改后重启生效）
  >>"%~dp0config.env" echo JWT_SECRET=!GENERATED!
  >>"%~dp0config.env" echo ADDR=:8080
  >>"%~dp0config.env" echo TZ=Asia/Shanghai
  echo 已生成 config.env
)

for /f "usebackq tokens=1,* delims==" %%a in ("%~dp0config.env") do (
  set "KEY=%%a"
  if defined KEY if not "!KEY:~0,1!"=="#" set "!KEY!=%%b"
)

if not defined ADDR set "ADDR=:8080"
if not defined JWT_SECRET set "JWT_SECRET=please-change-this-secret"
if not defined TZ set "TZ=Asia/Shanghai"
if not defined DB_PATH set "DB_PATH=%~dp0data\keysprint.db"
if not defined PUBLIC_DIR set "PUBLIC_DIR=%~dp0web"

if not exist "%~dp0data" mkdir "%~dp0data"
if not exist "%~dp0logs" mkdir "%~dp0logs"

echo.
echo   KeySprint 打字竞赛 - 局域网服务
echo   本机访问:   http://127.0.0.1%ADDR%
for /f "tokens=2 delims=:" %%i in ('ipconfig ^| findstr /c:"IPv4"') do (
  set "IP=%%i"
  set "IP=!IP: =!"
  echo   学生访问:   http://!IP!%ADDR%
)
echo.
echo   关闭本窗口即停止服务；日志: logs\keysprint.log
echo.

"%~dp0keysprint.exe" >> "%~dp0logs\keysprint.log" 2>&1
echo.
echo 服务已退出（退出码 %ERRORLEVEL%），详情见 logs\keysprint.log
if defined SESSIONNAME pause
