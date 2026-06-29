#!/bin/bash
# Start APScheduler in the background

cd "$(dirname "$0")"

if [ ! -d "djadmin" ]; then
    echo "Project directory not found: $(pwd)/djadmin"
    exit 1
fi

cd djadmin

echo "Starting APScheduler..."
echo "This will run in the background collecting host information every 15 minutes"
echo ""
echo "To stop the scheduler, press Ctrl+C"
echo ""

if [ -x "../.venv/bin/python" ]; then
    ../.venv/bin/python ./start_scheduler.py
elif [ -x "../.venv/Scripts/python.exe" ]; then
    ../.venv/Scripts/python.exe ./start_scheduler.py
else
    python ./start_scheduler.py
fi
