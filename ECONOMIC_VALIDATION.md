# Economic Validation System

Validate startup ideas by simulating 20 personas with realistic income, savings, and spending patterns to see who actually BUYS.

## What It Does

1. **Generates/uses an idea** - or you provide one
2. **Runs market research** - competitive analysis, pricing benchmarks
3. **Creates 20 economic personas** - with income, savings, spending habits
4. **Generates pricing tiers** - Free/Starter/Pro/Enterprise
5. **Simulates purchase decisions** - each persona decides if/what to buy
6. **Produces metrics** - conversion rate, MRR, ARPU, revenue projections
7. **HTML report** - beautiful visual breakdown

## Quick Start

```bash
cd ~/workspace/startup-factory

# Auto-generate idea
./run_economic_validation.py

# Test specific idea
./run_economic_validation.py "AI tool for real estate agents to write property listings in 30 seconds"

# Custom persona count
python3 run_economic_validation.py --count 50
```

## Output

Results saved to `results/economic_validation_TIMESTAMP/`:
- `validation_report.html` ← **Open this in browser**
- `personas.json` - All 20 personas with purchase decisions
- `pricing.json` - Generated pricing tiers
- `metrics.json` - Conversion metrics
- `idea.json` - The tested idea
- `research.json` - Market research findings

## Economic Persona Model

Each persona includes:

**Demographics:**
- Name, age, role, company size
- Experience years, technical skill

**Economic Profile:**
- Monthly/yearly income
- Total savings
- Monthly business spend
- Monthly personal spend
- Disposable income

**Psychology:**
- Pain level (1-10)
- Skepticism (1-10)
- Decision authority
- Triggers (what makes them buy)
- Objections (what holds them back)

**Purchase Simulation:**
- Will they buy? (yes/no)
- Which tier? (free/tier1/tier2/tier3)
- Payment method? (monthly/yearly)
- Purchase probability (0-100%)
- Reasoning (thought process)

## Pricing Tiers

Auto-generated with:
- **Free tier** - Hook for lead gen
- **Tier 1** - Entry point ($19-49/mo)
- **Tier 2** - Sweet spot ($49-99/mo) ← Most conversions
- **Tier 3** - Enterprise (custom)

Each tier includes:
- Features
- Target user
- Why upgrade from previous tier

## Metrics Calculated

- **Paid conversion rate** - % who buy paid tier
- **Total signup rate** - % who sign up (free + paid)
- **MRR** - Monthly Recurring Revenue
- **ARR** - Annual Recurring Revenue projection
- **ARPU** - Average Revenue Per User (all users)
- **Paying ARPU** - ARPU for paying customers only
- **Tier distribution** - % in each tier

## Example Results

From 20 personas:
- 3 buy Tier 1 ($29/mo) = $87 MRR
- 5 buy Tier 2 ($79/mo) = $395 MRR
- 1 buys Tier 3 ($199/mo) = $199 MRR
- 4 use free tier
- 7 don't sign up

**Total MRR: $681** (from 20 personas)
**Paid conversion: 45%**
**Projected ARR: $8,172**

Scale this: 1000 users → **$34,050 MRR** if conversion holds.

## Cost

Using Groq (free tier):
- Persona generation: ~$0
- Pricing generation: ~$0
- 20 purchase simulations: ~$0
- **Total: FREE** (rate limited to ~30 req/min)

Using Sonnet 4.5:
- Persona generation: ~$0.30
- Pricing: ~$0.10
- 20 simulations: ~$1.50
- **Total: ~$2-3** per validation

## When to Use

✅ **Use this when:**
- Testing idea viability BEFORE building
- Want to understand target customer economics
- Need conversion/revenue projections
- Deciding between multiple ideas

❌ **Don't use when:**
- Idea needs technical feasibility check (use coding-agent)
- Market is too new (no comparable pricing data)
- B2B enterprise only (decision process too complex)

## Integration with Factory

This replaces expensive FB ad validation:
- **Old way:** $50-200 in ads to test one idea
- **New way:** $0-3 in LLM calls, instant results

Can be integrated into `auto_validate.py` to run economic validation after initial scoring.

## Limitations

- **Personas are simulated** - not real people
- **LLM bias** - may overestimate conversion
- **No brand awareness** - assumes people know about product
- **No competition context** - doesn't model switching costs

Best used for RELATIVE comparison (Idea A vs Idea B), not absolute predictions.

## Next Steps

1. Run validation on your current top 3 ideas
2. Compare MRR projections
3. Pick winner based on:
   - Highest paid conversion
   - Highest MRR potential
   - Lowest objections
   - Best price sensitivity match
4. Build the winner
5. Run REAL ads to validate

The simulation gets you 80% of the way for 1% of the cost.
