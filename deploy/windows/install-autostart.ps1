# 注册 KeySprint 开机自启（计划任务，SYSTEM 权限、无需登录）
$ErrorActionPreference = 'Stop'
$dir = Split-Path -Parent $MyInvocation.MyCommand.Path
if (-not $dir.EndsWith('\')) { $dir += '\' }

$action = New-ScheduledTaskAction -Execute 'cmd.exe' -Argument ('/c "' + $dir + 'start.bat"')
$trigger = New-ScheduledTaskTrigger -AtStartup
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
Register-ScheduledTask -TaskName 'KeySprint' -Action $action -Trigger $trigger -Settings $settings -Description 'KeySprint 打字竞赛局域网服务' -RunLevel Highest -User 'SYSTEM' -Force | Out-Null
Write-Host "已注册计划任务 KeySprint，服务目录：$dir"
