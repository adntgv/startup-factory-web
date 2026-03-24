#!/bin/bash
cd ~/workspace/startup-factory/go-validator
completed=$(grep -c "Checkpoint saved" validation_run.log 2>/dev/null || echo 0)
echo "Progress: $completed/100 ideas"
if [ -f checkpoint.json ]; then
    python3 -c "
import json
with open('checkpoint.json') as f:
    data = json.load(f)
successful = sum(1 for i in data['completed_ideas'] if i.get('score', 0) > 0)
total = len(data['completed_ideas'])
if total > 0:
    print(f'Success rate: {successful}/{total} ({successful/total*100:.0f}%)')
    succ_ideas = [i for i in data['completed_ideas'] if i.get('score', 0) > 0]
    if succ_ideas:
        avg_conv = sum(i.get('conversion_rate', 0) for i in succ_ideas) / len(succ_ideas) * 100
        print(f'Avg conversion: {avg_conv:.0f}%')
"
fi
