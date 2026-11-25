$ErrorActionPreference = 'Stop'
$version = '1.24.13'
$archive = "go$version.windows-amd64.zip"
$url = "https://go.dev/dl/$archive"
$expected = '40b16bc8f00540a2cb02dff4de72b73e966fdd8d65f95e33d8e4080b48a2459a'
$installRoot = Join-Path $env:LOCALAPPDATA "Programs\Go-$version"
$temp = Join-Path $env:TEMP $archive
Write-Host "Downloading Go $version..."
Invoke-WebRequest -Uri $url -OutFile $temp
$actual = (Get-FileHash -Path $temp -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $expected) { throw "Go archive checksum mismatch. Expected $expected, got $actual" }
if (Test-Path $installRoot) { Remove-Item -Recurse -Force $installRoot }
New-Item -ItemType Directory -Force -Path $installRoot | Out-Null
Expand-Archive -Path $temp -DestinationPath $installRoot -Force
$goBin = Join-Path $installRoot 'go\bin'
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$entries = @($userPath -split ';' | Where-Object { $_ -and $_ -ne $goBin })
[Environment]::SetEnvironmentVariable('Path', (@($goBin) + $entries -join ';'), 'User')
$env:Path = "$goBin;$env:Path"
& (Join-Path $goBin 'go.exe') version
Write-Host "Go $version installed. Restart terminals so the updated user PATH is loaded."
