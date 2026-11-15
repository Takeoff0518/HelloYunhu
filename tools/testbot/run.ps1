# Start the NoneBot2 test bot.
# First run creates a virtual environment, installs dependencies and copies .env.
# Usage: .\run.ps1  (requires Python 3.10 or later)

$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot

function Get-PythonLauncher {
    if (Get-Command py -ErrorAction SilentlyContinue) {
        return 'py'
    }
    if (Get-Command python -ErrorAction SilentlyContinue) {
        return 'python'
    }
    throw 'Python 3.10 or later is required but was not found.'
}

$launcher = Get-PythonLauncher

if (-not (Test-Path -LiteralPath '.venv')) {
    Write-Host "Creating virtual environment with $launcher"
    & $launcher -m venv .venv
}

$venvPython = Join-Path $PSScriptRoot '.venv\Scripts\python.exe'
if (-not (Test-Path -LiteralPath $venvPython)) {
    throw 'Failed to create the virtual environment.'
}

if (-not (Test-Path -LiteralPath '.venv\Lib\site-packages\nonebot')) {
    Write-Host 'Installing dependencies, this may take a few minutes'
    & $venvPython -m pip install --upgrade pip --quiet
    & $venvPython -m pip install -r requirements.txt
}

if (-not (Test-Path -LiteralPath '.env')) {
    Copy-Item -LiteralPath '.env.example' -Destination '.env'
    Write-Host 'Created .env from .env.example'
}

& $venvPython bot.py
