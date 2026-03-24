#!/bin/bash
# Monitor validation progress

cd ~/workspace/startup-factory/go-validator

echo "🔍 Go Validator Status"
echo "====================="
echo ""

# Check process
if ps aux | grep -q "[8]90039"; then
    echo "✅ Process alive (PID 890039)"
else
    echo "❌ Process died!"
    exit 1
fi

echo ""

# Check checkpoint
if [ -f checkpoint.json ]; then
    python3 << 'EOF'
import json
with open('checkpoint.json') as f:
    data = json.load(f)

completed = len(data['completed_ideas'])
print(f"📊 Progress: {completed}/100 ideas ({completed}%)")
print(f"⏰ Timestamp: {data['timestamp']}")

# Success rate
successful = sum(1 for i in data['completed_ideas'] if i.get('score', 0) > 0)
if completed > 0:
    success_rate = successful / completed * 100
    print(f"✅ Success rate: {successful}/{completed} ({success_rate:.0f}%)")

print("\n🏆 Top 3 by score:")
sorted_ideas = sorted(data['completed_ideas'], key=lambda x: x.get('score', 0), reverse=True)
for i, idea in enumerate(sorted_ideas[:3], 1):
    score = idea.get('score', 0)
    conv = idea.get('conversion_rate', 0) * 100
    desc = idea.get('description', 'Unknown')[:60]
    print(f"{i}. {score:.2f} | {conv:.0f}% | {desc}...")

print("\n📈 Average metrics (successful ideas only):")
successful_ideas = [i for i in data['completed_ideas'] if i.get('score', 0) > 0]
if successful_ideas:
    avg_score = sum(i.get('score', 0) for i in successful_ideas) / len(successful_ideas)
    avg_conv = sum(i.get('conversion_rate', 0) for i in successful_ideas) / len(successful_ideas) * 100
    print(f"   Avg score: {avg_score:.2f}")
    print(f"   Avg conversion: {avg_conv:.0f}%")
EOF
else
    echo "⚠️  No checkpoint yet"
fi

echo ""
echo "📝 Last 10 log lines:"
tail -10 validation_run.log

echo ""
echo "⏱️  ETA: ~$(python3 -c "c=\$(grep -c 'Checkpoint saved' validation_run.log 2>/dev/null || echo 0); r=\$((100-c)); m=\$((r*2)); h=\$((m/60)); m=\$((m%60)); echo \${h}h \${m}m")"
