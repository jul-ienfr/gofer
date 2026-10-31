param(
    [string]$OutputDir = "dist",
    [string]$Go = "go",
    [string]$Templ = "templ"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    if (-not (Get-Command $Go -ErrorAction SilentlyContinue)) {
        throw "Go was not found: $Go"
    }
    if (-not (Get-Command $Templ -ErrorAction SilentlyContinue) -and -not (Test-Path $Templ)) {
        $localTempl = Join-Path $root ".tools\bin\templ.exe"
        if (Test-Path $localTempl) {
            $Templ = $localTempl
        } else {
            throw "templ was not found; install it with: go install github.com/a-h/templ/cmd/templ@v0.3.1001"
        }
    }

    & $Templ generate --ignore-pattern='(^|/)dist(/|$)'
    if ($LASTEXITCODE -ne 0) { throw "templ generate failed" }

    $tailwind = Join-Path $root "node_modules\@tailwindcss\cli\dist\index.mjs"
    if (-not (Test-Path $tailwind)) {
        throw "Tailwind CLI was not found; run: npm install --no-save @tailwindcss/cli@4.3.3"
    }
    & node $tailwind -i (Join-Path $root "assets\css\input.css") -o (Join-Path $root "assets\css\output.css") --minify
    if ($LASTEXITCODE -ne 0) { throw "Tailwind CSS build failed" }

    $scripts = Get-ChildItem (Join-Path $root "assets\js\*.js") |
        Where-Object { $_.Name -notmatch '\.min\.js$' -and $_.Name -ne "sw.js" } |
        ForEach-Object { $_.FullName }
    if ($scripts.Count -eq 0) { throw "no JavaScript sources found" }
    $jsOutput = Join-Path $root "assets\min\js"
    & $Go run github.com/evanw/esbuild/cmd/esbuild@v0.28.2 @scripts --minify "--outdir=$jsOutput" --log-level=warning
    if ($LASTEXITCODE -ne 0) { throw "JavaScript minification failed" }

    $output = $OutputDir
    if (-not [System.IO.Path]::IsPathRooted($output)) {
        $output = Join-Path $root $output
    }
    New-Item -ItemType Directory -Force -Path $output | Out-Null
    & $Go build -tags embedded_assets -trimpath -o (Join-Path $output "gofer.exe") .
    if ($LASTEXITCODE -ne 0) { throw "Go build failed" }

    Get-Item (Join-Path $output "gofer.exe") | Select-Object FullName, Length, LastWriteTime
} finally {
    Pop-Location
}
