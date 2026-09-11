param(
    [ValidateSet('start', 'load', 'stop', 'test', 'config')]
    [string]$Action = 'start'
)
$ErrorActionPreference = 'Stop'
$dockerCommand = Get-Command docker -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
if ($dockerCommand) {
    $calculatorDocker = $dockerCommand.Source
} else {
    $candidates = @(
        (Join-Path $env:LOCALAPPDATA 'Programs/DockerDesktop/resources/bin/docker.exe'),
        (Join-Path $env:ProgramFiles 'Docker/Docker/resources/bin/docker.exe')
    )
    $calculatorDocker = $candidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
}
if (-not $calculatorDocker) { throw 'Docker Desktop was not found. Install and start Docker Desktop first.' }
Push-Location $PSScriptRoot
try {
    switch ($Action) {
        'start' { & $calculatorDocker compose up --build -d calculator }
        'load' { & $calculatorDocker compose --profile load run --rm generator }
        'stop' { & $calculatorDocker compose down }
        'test' { & $calculatorDocker build --target test -t go-calculator-test . }
        'config' { & $calculatorDocker compose config --quiet }
    }
    if ($LASTEXITCODE -ne 0) { throw "Docker exited with code $LASTEXITCODE" }
} finally {
    Pop-Location
}
