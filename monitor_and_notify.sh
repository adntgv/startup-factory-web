#!/bin/bash
# Monitor optimization progress and notify at milestones

cd /home/aid/workspace/startup-factory

STATE_FILE="optimization_monitor_state.txt"

# Get current state
if [ -f "$STATE_FILE" ]; then
    LAST_NOTIFIED=$(cat "$STATE_FILE")
else
    LAST_NOTIFIED="0"
fi

# Check if process is running
if ! pgrep -f "auto_validate.py" > /dev/null; then
    # Process finished or crashed
    if [ -f results/auto_validation/final_report.json ]; then
        # Successfully completed
        if [ "$LAST_NOTIFIED" != "complete" ]; then
            # Send completion notification via OpenClaw
            echo "complete" > "$STATE_FILE"
            # Notification will be sent by cron system
            exit 0
        fi
    else
        # Crashed or stopped
        if [ "$LAST_NOTIFIED" != "crashed" ]; then
            echo "crashed" > "$STATE_FILE"
            exit 1
        fi
    fi
    exit 0
fi

# Count completed iterations
ITERATIONS=$(ls results/auto_validation/iteration_*.json 2>/dev/null | wc -l)

# Notify at milestones
if [ "$ITERATIONS" -ge 1 ] && [ "$LAST_NOTIFIED" = "0" ]; then
    echo "1" > "$STATE_FILE"
    echo "Iteration 1 complete"
elif [ "$ITERATIONS" -ge 5 ] && [ "$LAST_NOTIFIED" = "1" ]; then
    echo "5" > "$STATE_FILE"
    echo "Halfway point - Iteration 5 complete"
elif [ "$ITERATIONS" -ge 10 ] && [ "$LAST_NOTIFIED" = "5" ]; then
    echo "10" > "$STATE_FILE"
    echo "All 10 iterations complete - finalizing"
fi
