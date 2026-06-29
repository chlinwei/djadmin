# Start APScheduler in the background for Windows

# Set the working directory
Set-Location $PSScriptRoot

$projectDir = Join-Path $PSScriptRoot "djadmin"
$venvPython = Join-Path $PSScriptRoot ".venv\Scripts\python.exe"

if (-not (Test-Path $projectDir)) {
	Write-Error "Project directory not found: $projectDir"
	exit 1
}

Set-Location $projectDir

Write-Host "========================================" -ForegroundColor Green
Write-Host "Starting APScheduler" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "This will run in the background collecting host information every 15 minutes" -ForegroundColor Yellow
Write-Host ""
Write-Host "To stop the scheduler, press Ctrl+C" -ForegroundColor Yellow
Write-Host ""

if (Test-Path $venvPython) {
	& $venvPython .\start_scheduler.py
} else {
	python .\start_scheduler.py
}
