# Start Celery worker + beat for Windows

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
Write-Host "Starting Celery Worker + Beat" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "No need to open two terminals." -ForegroundColor Yellow
Write-Host ""
Write-Host "To stop the scheduler, press Ctrl+C" -ForegroundColor Yellow
Write-Host ""

if (Test-Path $venvPython) {
	& $venvPython .\start_scheduler.py
} else {
	python .\start_scheduler.py
}
