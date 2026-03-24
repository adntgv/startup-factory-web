# Profile Impact Guide

## How Each Question Affects Idea Filtering

### Section 1: Technical Capabilities

| Question | Impact | What It Filters |
|----------|--------|-----------------|
| **Programming Languages** | HIGH | • Eliminates entire technology stacks<br>• Flutter → mobile apps viable<br>• No Swift → iOS apps filtered out |
| **Frontend Capabilities** | HIGH | • Chrome extension → browser extension ideas prioritized<br>• No framework → complex web apps filtered out |
| **Backend/Infrastructure** | HIGH | • No Docker → complex deployments filtered out<br>• Has payment processing → subscription models viable |
| **AI/ML Capabilities** | CRITICAL | • API only → all AI ideas viable<br>• Can train models → opens ML product space<br>• None → AI ideas completely filtered out |
| **What You CAN'T Do** | HIGH | • Hard constraints that eliminate entire categories<br>• No iOS → half of mobile market gone<br>• No multiplayer → gaming ideas limited |

### Section 2: Resources & Constraints

| Question | Impact | What It Filters |
|----------|--------|-----------------|
| **Available Capital** | CRITICAL | • $0-500 → organic-only growth strategies<br>• $10k+ → paid ads viable<br>• Determines MVP complexity |
| **Time to First Revenue** | CRITICAL | • 1-2 months → only quick monetization ideas<br>• 12+ months → long validation cycles OK |
| **Weekly Time** | HIGH | • 5-10 hours → simple tools only<br>• 60+ hours → complex platforms viable |
| **Team Situation** | HIGH | • Solo → no network effects products<br>• Technical co-founder → splits workload |
| **MVP Timeline** | HIGH | • 1 week → template-based only<br>• 6 months → custom architecture OK |

### Section 3: Market Position

| Question | Impact | What It Filters |
|----------|--------|-----------------|
| **Industry Experience** | MEDIUM | • Finance background → fintech ideas prioritized<br>• No industry → generic tools only |
| **Existing Audience** | HIGH | • 10k followers → audience-first ideas viable<br>• Cold start → marketplace strategies needed |
| **Geographic Advantages** | MEDIUM | • Muslim community → halal finance tools<br>• Emerging market → local payment ideas |
| **Unique Assets** | MEDIUM | • Chrome extension published → browser tools prioritized<br>• No assets → competitive spaces avoided |

### Section 4: Preferences & Goals

| Question | Impact | What It Filters |
|----------|--------|-----------------|
| **Revenue Goal** | CRITICAL | • $1k/mo → prosumer/SMB focus<br>• $25k/mo → enterprise or high-volume needed |
| **Target Customer** | HIGH | • B2C → high-volume, low-price<br>• Enterprise → low-volume, high-touch |
| **Business Model** | MEDIUM | • Subscription → recurring value required<br>• One-time → standalone products |
| **Product Complexity** | MEDIUM | • Simple → single-feature tools<br>• Complex → platform ideas viable |
| **Marketing Preference** | MEDIUM | • Organic-only → SEO/content ideas<br>• Paid ads → quick testing cycles |
| **Must-Avoid** | HIGH | • Halal → alcohol/gambling filtered out<br>• Ethical → exploitative models removed |

## Real Examples

### Example 1: Solo Founder, $500, 2-Month Runway

**Profile Effect:**
```
Capital: $500 → Eliminates paid ads
Time: 2 months → Only fast-monetization ideas
Team: Solo → No marketplace/network effects
```

**Ideas Generated:**
✅ Browser extension for specific workflow ($29/mo)
✅ Paid Notion template ($99 one-time)
✅ API wrapper for niche use case ($49/mo)

**Ideas Filtered Out:**
❌ Marketplace platform (needs critical mass)
❌ Ad-supported social app (needs scale)
❌ Enterprise SaaS (6-month sales cycle)

### Example 2: Technical Co-founder, $10k, 6-Month Runway

**Profile Effect:**
```
Capital: $10k → Paid validation viable
Time: 6 months → Complex products OK
Team: 2 people → Can split frontend/backend
```

**Ideas Generated:**
✅ B2B SaaS with trials
✅ Niche marketplace (can bootstrap liquidity)
✅ Platform with network effects

**Ideas Filtered Out:**
❌ Simple tools (below your capacity)
❌ Consumer apps (need massive scale)

### Example 3: Muslim Market, AI Expertise, Chrome Extension Experience

**Profile Effect:**
```
Market: Muslim → underserved niches
Tech: AI APIs → AI-powered ideas prioritized
Asset: Chrome extension → browser tools scored high
```

**Ideas Generated:**
✅ AI Quran study tool (Chrome extension)
✅ Halal investment screener (AI-powered)
✅ Arabic content creator tools

**Ideas Filtered Out:**
❌ Generic productivity tools (no advantage)
❌ Non-AI products (missing unique strength)

## Why Accuracy Matters

### Scenario: Wrong Profile

```json
{
  "ai_capability": "train_models",  // WRONG - you can't
  "available_capital": "$10000",     // WRONG - you have $500
  "team": "technical_cofounder"      // WRONG - you're solo
}
```

**Result:**
- System suggests ML-powered recommendation engine (can't build)
- Suggests running $5k ad campaigns (can't afford)
- Suggests marketplace requiring 2-person team (can't execute)

→ **100% of suggestions are unbuildable**

### Scenario: Accurate Profile

```json
{
  "ai_capability": "api_only",      // CORRECT
  "available_capital": "$500",      // CORRECT
  "team": "solo"                    // CORRECT
}
```

**Result:**
- System suggests API-wrapper tools (buildable)
- Suggests organic growth strategies (affordable)
- Suggests single-feature products (executable solo)

→ **80%+ of suggestions are viable**

## Profile Evolution

Your profile should evolve as you:
- ✅ Learn new skills (add programming languages)
- ✅ Build assets (add existing products/audience)
- ✅ Gain capital (update available funds)
- ✅ Prove capabilities (past successes)

**Update frequency:** After major changes (new skill, shipped product, raised funds, etc.)

## Profile Quality Checklist

Before running the system, verify:

- [ ] All "Can Build" claims are proven (shipped production code)
- [ ] All "Can't Build" admissions are honest (not aspirational)
- [ ] Capital amount is liquid and accessible NOW
- [ ] Time commitment is realistic (not "I'll find time")
- [ ] Revenue goal matches your actual needs
- [ ] Constraints are non-negotiable (not preferences)
- [ ] Unique advantages are defensible (not "I'm interested in X")

**Remember:** The system can only be as good as the profile. Garbage in = garbage out.

## Next Steps

1. **Answer the questionnaire** (honest, detailed)
2. **Review the generated profile.json** (verify accuracy)
3. **Run test idea generation** (see if results make sense)
4. **Iterate on profile** (refine based on test results)
5. **Run full optimization** (once profile is accurate)
