<#
.SYNOPSIS
  Build the Windows portable package (Docker-free LAN deployment).

.DESCRIPTION
  Cross-compiles keysprint.exe for windows/amd64 (CGO disabled, pure Go) and
  bundles it with the prebuilt frontend dist plus the deploy/windows scripts.

  Build the frontend first:  cd frontend && npm ci && npm run build

.EXAMPLE
  ./scripts/package-windows.ps1
  ./scripts/package-windows.ps1 -Version 1.8.0 -Commit abcdef1
#>
param(
  [string]$Version = '',
  [string]$Commit = '',
  [string]$OutputDirectory = ''
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

if (-not $Version) { $Version = (Get-Content -Raw (Join-Path $root 'VERSION')).Trim() }
if (-not $Commit) {
  $Commit = (& git -C $root rev-parse --short=7 HEAD) 2>$null
  if (-not $Commit) { $Commit = 'unknown' }
}
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $root 'dist-windows' }

$dist = Join-Path $root 'frontend/dist'
if (-not (Test-Path (Join-Path $dist 'index.html'))) {
  throw "Missing frontend build output: $dist. Run: cd frontend && npm ci && npm run build"
}

$staging = Join-Path $OutputDirectory 'KeySprint-Windows-x64'
if (Test-Path $staging) { Remove-Item -Recurse -Force $staging }
New-Item -ItemType Directory -Force -Path $staging | Out-Null

Write-Host "==> building keysprint.exe (version=$Version, commit=$Commit)"
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
Push-Location (Join-Path $root 'backend')
try {
  & go build -mod=readonly -buildvcs=false -trimpath `
      -ldflags="-s -w -buildid= -X main.version=$Version -X main.commit=$Commit" `
      -o (Join-Path $staging 'keysprint.exe') .
  if ($LASTEXITCODE -ne 0) { throw "go build failed (exit $LASTEXITCODE)" }
} finally {
  Pop-Location
  Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

Write-Host '==> copying frontend assets to web/'
Copy-Item -Path $dist -Destination (Join-Path $staging 'web') -Recurse

Write-Host '==> copying Windows deploy scripts'
Copy-Item -Path (Join-Path $root 'deploy/windows/*') -Destination $staging -Recurse

# Console scripts are encoded for the Chinese Windows code page (GBK) because
# cmd.exe reads .bat files in the OEM code page; PowerShell scripts get a UTF-8
# BOM so Windows PowerShell 5.1 parses them correctly. Line endings are CRLF.
$gbk = [System.Text.Encoding]::GetEncoding(936)
$utf8Bom = New-Object System.Text.UTF8Encoding($true)
$utf8 = New-Object System.Text.UTF8Encoding($false)
$editable = @('.bat', '.ps1', '.txt', '.md')
foreach ($file in Get-ChildItem $staging -File | Where-Object { $_.Extension -in $editable }) {
  $text = [System.IO.File]::ReadAllText($file.FullName, $utf8)
  $text = $text -replace "`r?`n", "`r`n"
  if ($file.Extension -eq '.bat') {
    [System.IO.File]::WriteAllText($file.FullName, $text, $gbk)
  } elseif ($file.Extension -eq '.ps1') {
    [System.IO.File]::WriteAllText($file.FullName, $text, $utf8Bom)
  } elseif ($file.Extension -in @('.txt', '.md')) {
    [System.IO.File]::WriteAllText($file.FullName, $text, $utf8Bom)
  }
}

New-Item -ItemType Directory -Force -Path (Join-Path $staging 'data') | Out-Null
$versionText = "KeySprint $Version`r`ncommit $Commit`r`nbuilt $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')`r`n"
[System.IO.File]::WriteAllText((Join-Path $staging 'VERSION.txt'), $versionText, $utf8Bom)

$zip = Join-Path $OutputDirectory "keysprint-$Version-windows-x64.zip"
if (Test-Path $zip) { Remove-Item -Force $zip }
Write-Host "==> packaging $zip"
# Flat archive: entries start at the zip root (keysprint.exe, web/...), so the
# teacher unzips exactly once instead of unpacking nested wrapper folders.
Add-Type -AssemblyName System.IO.Compression.FileSystem
Add-Type -AssemblyName System.IO.Compression | Out-Null
$archive = [System.IO.Compression.ZipFile]::Open($zip, 'Create')
try {
  foreach ($dir in Get-ChildItem $staging -Recurse -Directory) {
    $relative = $dir.FullName.Substring($staging.Length + 1).Replace([System.IO.Path]::DirectorySeparatorChar, '/')
    if ($relative) { [void]$archive.CreateEntry($relative + '/') }
  }
  foreach ($file in Get-ChildItem $staging -Recurse -File) {
    $relative = $file.FullName.Substring($staging.Length + 1).Replace([System.IO.Path]::DirectorySeparatorChar, '/')
    [void][System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive, $file.FullName, $relative, [System.IO.Compression.CompressionLevel]::Optimal)
  }
} finally {
  $archive.Dispose()
}

$size = [math]::Round((Get-Item $zip).Length / 1MB, 1)
Write-Host "done: $zip ($size MB)"
