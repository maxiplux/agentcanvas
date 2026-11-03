# AgentCanvas installer for Windows (PowerShell 5.1+ or PowerShell 7).
#
#   irm https://maxiplux.github.io/agentcanvas/install.ps1 | iex
#
# Installs the latest agent-canvas release, verified against SHA256SUMS, into
# %LOCALAPPDATA%\Programs\agent-canvas and adds that folder to your user PATH.
# Run it again at any time to update; `agent-canvas update` does the same.
# Never prompts and needs no administrator rights.
#
# Environment (all optional):
#   AGENT_CANVAS_VERSION      release tag to install, e.g. v0.1.1 (default: latest)
#   AGENT_CANVAS_INSTALL_DIR  directory for agent-canvas.exe

& {
  $ErrorActionPreference = 'Stop'
  $ProgressPreference = 'SilentlyContinue'
  [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

  $repo = 'maxiplux/agentcanvas'
  $version = if ($env:AGENT_CANVAS_VERSION) { $env:AGENT_CANVAS_VERSION } else { 'latest' }
  $isArm = ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') -or ($env:PROCESSOR_ARCHITEW6432 -eq 'ARM64')
  $arch = if ($isArm) { 'arm64' } else { 'amd64' }
  $asset = "agent-canvas-windows-$arch.exe"
  $base = if ($version -eq 'latest') { "https://github.com/$repo/releases/latest/download" } else { "https://github.com/$repo/releases/download/$version" }
  $dir = if ($env:AGENT_CANVAS_INSTALL_DIR) { $env:AGENT_CANVAS_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\agent-canvas' }

  $tmp = Join-Path ([IO.Path]::GetTempPath()) ('agent-canvas-' + [Guid]::NewGuid())
  New-Item -ItemType Directory -Force -Path $tmp | Out-Null
  try {
    Write-Host "agent-canvas installer: downloading $asset ($version)"
    Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset)
    Invoke-WebRequest -UseBasicParsing -Uri "$base/SHA256SUMS" -OutFile (Join-Path $tmp 'SHA256SUMS')

    $line = Get-Content (Join-Path $tmp 'SHA256SUMS') | Where-Object { ($_ -split '\s+')[1] -eq $asset } | Select-Object -First 1
    if (-not $line) { throw "agent-canvas installer: $asset is not listed in SHA256SUMS" }
    $want = ($line -split '\s+')[0].ToLower()
    $got = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $tmp $asset)).Hash.ToLower()
    if ($want -ne $got) { throw "agent-canvas installer: checksum mismatch for $asset (want $want, got $got)" }

    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    $dest = Join-Path $dir 'agent-canvas.exe'
    # A running .exe cannot be overwritten on Windows, but it can be renamed.
    if (Test-Path $dest) {
      $old = "$dest.old"
      if (Test-Path $old) { Remove-Item -Force $old -ErrorAction SilentlyContinue }
      Rename-Item -Path $dest -NewName 'agent-canvas.exe.old' -Force
    }
    Move-Item -Force -Path (Join-Path $tmp $asset) -Destination $dest
    Remove-Item -Force "$dest.old" -ErrorAction SilentlyContinue

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not (($userPath -split ';') -contains $dir)) {
      $newPath = if ($userPath) { "$userPath;$dir" } else { $dir }
      [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
      Write-Host "agent-canvas installer: added $dir to your user PATH (new terminals pick it up)"
    }
    if (-not (($env:Path -split ';') -contains $dir)) { $env:Path = "$env:Path;$dir" }

    $installed = & $dest version
    Write-Host "agent-canvas installer: installed $installed to $dest"
    Write-Host 'agent-canvas installer: start the studio in a project folder with: agent-canvas'
  }
  finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
  }
}
