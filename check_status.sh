#!/bin/bash
# Check optimization status

cd /home/aid/workspace/startup-factory

echo "========================================================================"
echo "OPTIMIZATION STATUS CHECK"
echo "========================================================================"
echo ""

# Check if process is running
if pgrep -f "auto_validate.py" > /dev/null; then
    echo "✅ Optimization is RUNNING"
    echo ""
    
    # Show progress from log
    if [ -f optimization_run.log ]; then
        echo "📊 Recent Progress:"
        echo "──────────────────────────────────────────────────────────────────"
        tail -30 optimization_run.log | grep -E "(ITERATION|Testing Idea|Score:|✅|🏆)" | tail -20
        echo "──────────────────────────────────────────────────────────────────"
        echo ""
        echo "Full log: tail -f optimization_run.log"
    fi
else
    echo "❌ Optimization is NOT running"
    echo ""
    
    # Check if it completed
    if [ -f results/auto_validation/final_report.json ]; then
        echo "✅ Optimization COMPLETED - results available"
        echo ""
        echo "📊 Final Results:"
        echo "──────────────────────────────────────────────────────────────────"
        python3 -c '
import json
with open("results/auto_validation/final_report.json") as f:
    report = json.load(f)
    print(f"  Total tested: {report[\"total_tested\"]}")
    print(f"  Successful: {report[\"successful\"]}")
    print(f"  Runtime: {report[\"total_runtime_minutes\"]:.1f} minutes")
    print("")
    print("  Top 5 Ideas:")
    for i, idea in enumerate(report["top_10"][:5], 1):
        print(f"    {i}. Score: {idea[\"score\"]:.2f} | Conv: {idea[\"conversion_rate\"]*100:.1f}%")
    if report["winners"]:
        print("")
        print(f"  ✅ {len(report[\"winners\"])} winners worth real ad testing!")
'
        echo "──────────────────────────────────────────────────────────────────"
    elif [ -f optimization_run.log ]; then
        echo "⚠️  Process stopped - check log for errors"
        echo ""
        echo "Last 20 lines of log:"
        tail -20 optimization_run.log
    else
        echo "ℹ️  Not started yet"
    fi
fi

echo ""
echo "========================================================================"
echo "Results directory: results/auto_validation/"
echo "Iteration files: results/auto_validation/iteration_*.json"
echo "========================================================================"
