$ErrorActionPreference = "Stop"

$targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64" },
    @{ GOOS = "linux";   GOARCH = "amd64" },
    @{ GOOS = "linux";   GOARCH = "arm64" },
    @{ GOOS = "darwin";  GOARCH = "amd64" },
    @{ GOOS = "darwin";  GOARCH = "arm64" }
)

foreach ($target in $targets) {
    $env:GOOS = $target.GOOS
    $env:GOARCH = $target.GOARCH

    $outputName = "build/${env:GOOS}_${env:GOARCH}"

    if ($env:GOOS -eq "windows") {
        $outputName += ".exe"
    }

    mkdir -Force -Path (Split-Path $outputName) | Out-Null

    Write-Host "Building for $($env:GOOS)/$($env:GOARCH)..."
    go build -o $outputName .

    if ($LASTEXITCODE -ne 0) {
        Write-Host "Build failed for $($env:GOOS)/$($env:GOARCH)"
    } else {
        Write-Host "Built: $outputName"
    }
}

