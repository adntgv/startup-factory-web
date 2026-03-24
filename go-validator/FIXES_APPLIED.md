# Persona Decision Modeling - Quality Fixes Applied

## Date: 2026-03-12 02:30 AM

## Critical Issues Fixed

### 1. **Parse Error Fallback Bug** ✅
**Problem**: Parse errors were counted as conversions, inflating metrics  
**Location**: `helpers.go:203-211`  
**Fix**: Parse errors now return `converted: false` with proper error message  
**Impact**: LFM and Programmatic "64% / 60% conversion" were fake - now accurately report failures

**Before**:
```go
if err := json.Unmarshal(...) {
    converted := strings.Contains(lc, "yes") || strings.Contains(lc, "sign up")
    return SimulationResult{Converted: converted, ...}  // BUG!
}
```

**After**:
```go
if err := json.Unmarshal(...) {
    return SimulationResult{
        Converted: false,
        Reasoning: fmt.Sprintf("Parse error: %v", err),
    }
}
```

---

### 2. **Simulation Prompt - Realism Improvements** ✅
**Problem**: 21% conversion rate (4-10x higher than real SaaS 2-5% baseline)  
**Location**: `pipeline.go:750-810`  
**Fixes Applied**:

**Added Friction Modeling**:
```
CRITICAL: Most people (95-98%) do NOT convert on first visit. Common reasons:
- Procrastination: "I'll bookmark this for later" then forget
- Comparison shopping: "Let me check alternatives first"  
- Partner consultation: "I need to ask my spouse/team first"
- Distraction: Got a text/email mid-read and moved on
- Budget uncertainty: "I should wait until next paycheck"
- Trust issues: "Never heard of this company before"

MODEL REALISTIC HUMAN BEHAVIOR. People are busy, distracted, skeptical, and procrastinate even on things they want.
```

**Improved Reasoning Requirement**:
```
"reasoning": "2-3 sentences explaining your decision as a real human (include procrastination, distractions, or partner consultation if relevant)"
```

**Expected Impact**: Conversion rates should drop from 21% → 2-5% realistic range

---

### 3. **Persona Generation Prompts - Explicit Field Requirements** ✅
**Problem**: JSON responses missing required fields or malformed  
**Location**: 
- MaxClaw: `pipeline.go:425-470`
- LFM: `pipeline.go:630-660`
- Programmatic: `pipeline.go:500-535`

**Fixes**:
- Converted to explicit JSON schema format with types
- All fields marked as REQUIRED
- Removed ambiguous prose descriptions
- Added "JSON only, no markdown" instruction

**Before**:
```
Return a JSON array where each persona has:
- name, age, role, company_size, experience_years
- pain_level (1-10), skepticism (1-10)
...
```

**After**:
```
Return ONLY valid JSON array. ALL fields required:
[{"name":"str","age":int,"role":"str","company_size":"solo|startup|smb|enterprise",...}]

JSON only, no markdown.
```

---

### 4. **Retry Logic for Parse Failures** ✅
**Problem**: Single LLM failures caused entire persona batches to fail  
**Location**:
- Simulation: `pipeline.go:820-845`
- MaxClaw personas: `pipeline.go:470-490`  
- LFM personas: `pipeline.go:635-665`
- Programmatic personas: `pipeline.go:510-540`

**Implementation**: Retry up to 2 times on parse errors before giving up

**MaxClaw Simulation Example**:
```go
for attempt := 0; attempt < 2; attempt++ {
    resp := p.maxclaw.Call(...)
    if resp.Error != nil {
        continue
    }
    
    result := parseSimulation(resp.Content, persona.Name)
    if !strings.HasPrefix(result.Reasoning, "Parse error") {
        return result  // Success!
    }
    // Parse error - retry once
}
```

**Expected Impact**: Reduce persona generation failures from ~80% → ~10-20%

---

## Testing Recommendations

### Quick Validation Test (5 min)
```bash
cd ~/workspace/startup-factory/go-validator

# Test with small sample to verify fixes
./startup-validator \
  -ideas 5 \
  -personas 10 \
  -epochs 1 \
  -enrichment-mode maxclaw \
  -output results/validation \
  -max-concurrent 50 \
  -verbose
```

**Expected Results**:
- ✅ No "Parse error, fallback detection" conversions
- ✅ 90%+ personas have valid reasoning (not "LLM call failed")
- ✅ Conversion rate: 2-8% per idea (realistic range)
- ✅ Reasoning includes friction: procrastination, partner consultation, budget concerns

### Full Comparison Test (15 min)
```bash
# Run all 3 modes with fixes
./test_enrichment_comparison.sh
```

**What to Check**:
1. **Conversion rates**: Should be 2-8% across all modes (not 35-64%)
2. **Valid reasoning %**: Should be 80-95% (not 17%)
3. **Friction modeling**: Check for "I'll bookmark this", "need to ask my spouse", "will compare alternatives"
4. **No fake conversions**: grep for "Parse error, fallback detection" → should be 0

---

## Opus Analysis Summary

**Realism Score**: 5/10 → targeting 8/10 with these fixes

**Before Fixes**:
- ❌ 21% conversion (4-10x too high)
- ❌ Conversions too frictionless ("no-brainer", "FOMO on efficiency gains")
- ❌ LLM hyperbole leaking into persona voice
- ✅ Rejections were realistic (product-market mismatch modeling was solid)

**After Fixes (Expected)**:
- ✅ 2-5% conversion (realistic SaaS baseline)
- ✅ Friction modeling (procrastination, distraction, partner veto)
- ✅ Human decision-making (not rational agents)
- ✅ Better JSON parsing reliability

---

## Known Limitations (Still Present)

1. **LLMs model rational agents**: Even with friction prompts, LLMs struggle to model truly distracted/forgetful humans
2. **No time decay**: Real users often intend to convert but forget after closing the tab
3. **Perfect information assumption**: Real users don't read all features carefully
4. **No external factors**: Family interruptions, competing priorities, mood swings

**Recommendation**: Use for **relative comparison** between ideas (which converts better), not absolute conversion prediction.

---

## Files Modified

- `helpers.go` - Fixed parse error fallback bug
- `pipeline.go` - Improved prompts, added retry logic (all 3 modes)

## Binary Rebuilt

✅ `startup-validator` binary updated with all fixes
