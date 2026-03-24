#!/bin/bash
#
# Restart optimization with all optimizations enabled:
# - 6x parallel LLM calls
# - Multi-provider fallback (OpenRouter→Groq→Cerebras→Local)
# - 10-20x faster (1-2 hours vs 20+ hours)
#

cd ~/workspace/startup-factory

echo "🛑 Stopping old process..."
pkill -f "python3.*auto_validate.py"
pkill -f "python3.*run_20_experiments.py"
sleep 2

echo "🚀 Starting OPTIMIZED validation..."
echo ""
echo "✨ Optimizations enabled:"
echo "  • 6x parallel LLM calls"
echo "  • Multi-provider fallback (OpenRouter→Groq→Cerebras→Local)"
echo "  • Estimated time: 1-2 hours (was 20+ hours)"
echo ""

nohup python3 auto_validate.py --iterations 10 --batch-size 10 --min-score 0.35 > optimization_run_v2.log 2>&1 &

NEW_PID=$!

echo "✅ Started! PID: $NEW_PID"
echo ""
echo "📊 Monitor:"
echo "  tail -f ~/workspace/startup-factory/optimization_run_v2.log"
echo ""
echo "🔍 Check status:"
echo "  bash ~/workspace/startup-factory/check_status.sh"
echo ""

sleep 5
echo "First few log lines:"
tail -20 optimization_run_v2.log
