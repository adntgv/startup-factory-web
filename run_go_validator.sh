#!/bin/bash
#
# Run Go validator with real ideas from Python generator
#

cd ~/workspace/startup-factory

echo "🎲 Generating ideas with Python..."

python3 << 'PYTHON'
import json
from idea_generator import IdeaGenerator

gen = IdeaGenerator('profile.json')
ideas = gen.generate_ideas(count=10, min_score=0.35)

# Export to JSON for Go
ideas_data = []
for idea in ideas:
    ideas_data.append({
        'description': gen.describe_idea(idea),
        'coordinates': idea.coordinates,
        'fit_score': idea.fit_score
    })

with open('go-validator/ideas_input.json', 'w') as f:
    json.dump(ideas_data, f, indent=2)

print(f"✅ Generated {len(ideas)} ideas")
PYTHON

if [ $? -ne 0 ]; then
    echo "❌ Idea generation failed"
    exit 1
fi

echo ""
echo "🚀 Running Go validator..."
echo ""

cd go-validator
./validator

echo ""
echo "✅ Validation complete!"
echo ""
echo "Results saved to: results/"
ls -lh results/ | tail -5
