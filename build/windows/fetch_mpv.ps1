param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('amd64', 'arm64')]
    [string]$Arch
)

$ErrorActionPreference = 'Stop'

$assets = @{
    amd64 = @{
        Name = 'mpv-x86_64-20260903-git-69e63f425a.7z'
        Sha256 = '418dbfb5feb851cbed33d6c05d8481ba71802621bfd6efe8974522b28d42ac97'
    }
    arm64 = @{
        Name = 'mpv-aarch64-20260903-git-69e63f425a.7z'
        Sha256 = '1ac2e56fdc990db5d7d448432e95e653c622fd518250fb3f7d8876d4b4f4459f'
    }
}

$asset = $assets[$Arch]
$url = "https://github.com/shinchiro/mpv-winbuild-cmake/releases/download/20260903/$($asset.Name)"
$root = $PSScriptRoot
$archive = Join-Path $root 'mpv.7z'
$output = Join-Path $root 'mpv'
$sevenZip = 'C:\Program Files\7-Zip\7z.exe'

if (-not (Test-Path -LiteralPath $sevenZip -PathType Leaf)) {
    throw "7-Zip was not found at $sevenZip"
}

if (Test-Path -LiteralPath $archive) {
    Remove-Item -LiteralPath $archive -Force
}
if (Test-Path -LiteralPath $output) {
    Remove-Item -LiteralPath $output -Recurse -Force
}
New-Item -ItemType Directory -Path $output -Force | Out-Null

# GitHub 下载在 Windows 运行器上连续报「connection closed unexpectedly」：
# PowerShell 5.1 的 Invoke-WebRequest 默认只协商 TLS1.0/1.1，而 GitHub 要求 TLS1.2，
# 握手被拒表现为连接被关闭。显式 TLS1.2 + 带退避的 3 次重试兜底；全失败才抛错。
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$downloadError = $null
for ($attempt = 1; $attempt -le 3; $attempt++) {
    try {
        Invoke-WebRequest -Uri $url -OutFile $archive -UseBasicParsing -MaximumRedirection 10
        $downloadError = $null
        break
    } catch {
        $downloadError = $_
        Write-Host "下载 mpv 归档失败（第 $attempt/3 次）：$($_.Exception.Message)"
        if ($attempt -lt 3) { Start-Sleep -Seconds (2 * $attempt) }
    }
}
if ($downloadError) { throw "下载 mpv 归档失败：$downloadError" }

$actualHash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash
if ($actualHash -ine $asset.Sha256) {
    throw "SHA256 mismatch for $($asset.Name): expected $($asset.Sha256), got $actualHash"
}

& $sevenZip x -y "-o$output" $archive
if ($LASTEXITCODE -ne 0) {
    throw "7-Zip failed to extract $($asset.Name) with exit code $LASTEXITCODE"
}

$entries = @(Get-ChildItem -LiteralPath $output -Force)
$topLevelDirectories = @($entries | Where-Object { $_.PSIsContainer })
if ($topLevelDirectories.Count -eq 1) {
    $topLevel = $topLevelDirectories[0].FullName
    Get-ChildItem -LiteralPath $topLevel -Force | ForEach-Object {
        Move-Item -LiteralPath $_.FullName -Destination $output -Force
    }
    Remove-Item -LiteralPath $topLevel -Recurse -Force
}

if (-not (Test-Path -LiteralPath (Join-Path $output 'mpv.exe') -PathType Leaf)) {
    throw "Extracted archive does not contain $output\mpv.exe"
}

Remove-Item -LiteralPath $archive -Force
