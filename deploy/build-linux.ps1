# Cross-compile all Go services for Linux (amd64), required before `docker compose build`.
# Run from repo root:  .\deploy\build-linux.ps1

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$services = @(
    "auth-service",
    "user-service",
    "venue-service",
    "booking-service",
    "review-service",
    "payment-service",
    "api-gateway"
)

$outDir = Join-Path $repoRoot "bin\linux"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

foreach ($svc in $services) {
    Write-Host "Building $svc for linux..."
    Push-Location (Join-Path $repoRoot "services\$svc")
    try {
        $env:CGO_ENABLED = "0"
        $env:GOOS = "linux"
        $env:GOARCH = "amd64"
        $out = Join-Path $outDir $svc
        go build -o $out ./cmd/
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed for $svc (exit $LASTEXITCODE)"
        }
    }
    finally {
        Remove-Item Env:GOOS -ErrorAction SilentlyContinue
        Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
        Pop-Location
    }
}

Write-Host "Done. Binaries in bin/linux/"
