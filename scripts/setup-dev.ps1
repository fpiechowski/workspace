[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

function Stop-Setup([string]$Message) {
    throw "workspace development setup: $Message"
}

if ($args.Count -ne 0) {
    Stop-Setup 'usage: scripts/setup-dev.ps1'
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if (-not (Test-Path -LiteralPath (Join-Path $repoRoot 'go.mod') -PathType Leaf) -or
    -not (Test-Path -LiteralPath (Join-Path $repoRoot 'cmd/workspace') -PathType Container)) {
    Stop-Setup "could not locate workspace repository from '$PSScriptRoot'"
}

$goCommand = Get-Command go -ErrorAction SilentlyContinue
if (-not $goCommand) { Stop-Setup 'Go 1.24 or newer is required' }
$goVersionOutput = (& go version 2>&1 | Out-String).Trim()
if ($LASTEXITCODE -ne 0 -or $goVersionOutput -notmatch 'go version go(?<major>\d+)\.(?<minor>\d+)') {
    Stop-Setup "could not determine Go version: $goVersionOutput"
}
$goMajor = [int]$Matches.major
$goMinor = [int]$Matches.minor
if ($goMajor -lt 1 -or ($goMajor -eq 1 -and $goMinor -lt 24)) {
    Stop-Setup "Go 1.24 or newer is required; found $goVersionOutput"
}

$goOS = (& go env GOOS 2>&1 | Out-String).Trim()
if ($LASTEXITCODE -ne 0) { Stop-Setup "could not determine Go's target OS: $goOS" }
if ($goOS -ne 'windows') { Stop-Setup "Go is configured for GOOS=$goOS on native Windows; unset GOOS or set it to windows" }

if (Test-Path Env:WORKSPACE_DEV_BUILD_DIR) {
    if ([string]::IsNullOrWhiteSpace($env:WORKSPACE_DEV_BUILD_DIR)) { Stop-Setup 'WORKSPACE_DEV_BUILD_DIR must not be empty' }
    $buildDirRaw = $env:WORKSPACE_DEV_BUILD_DIR
} else {
    $buildDirRaw = Join-Path $repoRoot 'bin'
}
if (Test-Path Env:WORKSPACE_DEV_INSTALL_DIR) {
    if ([string]::IsNullOrWhiteSpace($env:WORKSPACE_DEV_INSTALL_DIR)) { Stop-Setup 'WORKSPACE_DEV_INSTALL_DIR must not be empty' }
    $installDirRaw = $env:WORKSPACE_DEV_INSTALL_DIR
} else {
    $installDirRaw = Join-Path $env:USERPROFILE 'bin'
}

function Resolve-SetupDirectory([string]$Path) {
    if (-not [IO.Path]::IsPathRooted($Path)) { $Path = Join-Path $repoRoot $Path }
    New-Item -ItemType Directory -Force -Path $Path | Out-Null
    return (Resolve-Path -LiteralPath $Path).Path
}

$buildDir = Resolve-SetupDirectory $buildDirRaw
$installDir = Resolve-SetupDirectory $installDirRaw
$artifactPath = Join-Path $buildDir 'workspace.exe'
$installPath = Join-Path $installDir 'workspace.cmd'

if (Test-Path -LiteralPath $installPath) {
    $existing = Get-Item -LiteralPath $installPath -Force
    $expectedShim = "@echo off`r`n`"$artifactPath`" %*`r`n"
    if ($existing.PSIsContainer -or $existing.LinkType -or
        (Get-Content -LiteralPath $installPath -Raw) -cne $expectedShim) {
        Stop-Setup "refusing to replace existing command '$installPath'; choose another WORKSPACE_DEV_INSTALL_DIR"
    }
}

$tempBinary = Join-Path $buildDir ('.workspace.dev.' + [guid]::NewGuid().ToString('N') + '.exe')
try {
    Push-Location $repoRoot
    try {
        & go build -trimpath -o $tempBinary ./cmd/workspace
        if ($LASTEXITCODE -ne 0) { Stop-Setup "development build failed; existing artifact '$artifactPath' was left unchanged" }
    } finally { Pop-Location }
    if (-not (Test-Path -LiteralPath $tempBinary -PathType Leaf)) {
        Stop-Setup "Go did not produce a development binary; existing artifact '$artifactPath' was left unchanged"
    }
    Move-Item -LiteralPath $tempBinary -Destination $artifactPath -Force
} finally {
    if (Test-Path -LiteralPath $tempBinary) { Remove-Item -LiteralPath $tempBinary -Force }
}

if (-not (Test-Path -LiteralPath $installPath)) {
    $shim = "@echo off`r`n`"$artifactPath`" %*`r`n"
    [IO.File]::WriteAllText($installPath, $shim, [Text.Encoding]::ASCII)
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$pathEntries = @($userPath -split ';' | Where-Object { $_ })
if (-not ($pathEntries | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') })) {
    $newUserPath = (@($pathEntries) + $installDir) -join ';'
    [Environment]::SetEnvironmentVariable('Path', $newUserPath, 'User')
    $env:Path = "$installDir;$env:Path"
    Write-Output "Added development command directory to the user PATH: $installDir (open a new terminal to use it there)"
}

Write-Output "Built development binary: $artifactPath"
Write-Output "Installed development command: $installPath -> $artifactPath"
Write-Output "Run 'workspace version' in a new PowerShell or Command Prompt window."
