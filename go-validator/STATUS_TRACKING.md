# Status Tracking - Conversion/Rejection/Error Breakdown

## Date: 2026-03-12 02:38 AM

## What Changed

Added explicit status tracking to distinguish between:
1. **✓ Converted**: Valid reasoning, persona decided to sign up
2. **✗ Rejected**: Valid reasoning, persona decided NOT to sign up  
3. **⚠ Error**: Parse error or LLM failure (invalid data)

## Implementation

### 1. Added Status Field
**File**: `types.go:256`

```go
type SimulationResult struct {
    ...
    Converted bool   `json:"converted"`
    Status    string `json:"status"`  // NEW: "converted", "rejected", "error"
    ...
}
```

### 2. Updated Metrics
**File**: `types.go:54`

```go
type SimulationMetrics struct {
    ...
    Conversions   int `json:"conversions"`
    Rejections    int `json:"rejections"`   // NEW
    Errors        int `json:"errors"`        // NEW
    TotalPersonas int `json:"total_personas"`
    ValidPersonas int `json:"valid_personas"` // NEW: conversions + rejections
}
```

### 3. Status Assignment Logic
**File**: `helpers.go:203-220`

**Parse Error** → Status: "error"
```go
if err := json.Unmarshal(...) {
    return SimulationResult{
        Status: "error",
        Converted: false,
        Reasoning: "Parse error: ...",
    }
}
```

**Valid Response** → Status: "converted" or "rejected"
```go
status := "rejected"
if simData.Converted {
    status = "converted"
}
```

**LLM Failure** → Status: "error"
```go
if resp.Error != nil {
    return SimulationResult{
        Status: "error",
        Converted: false,
        Reasoning: "LLM call failed: ...",
    }
}
```

### 4. Scoring Calculation
**File**: `scoring.go:19-60`

**Conversion rate now excludes errors:**
```go
validPersonas := conversions + rejections
convRate := float64(conversions) / float64(validPersonas)  // Not /total
```

**Backward compatibility:**
```go
switch r.Status {
case "converted":
    conversions++
case "rejected":
    rejections++
case "error":
    errors++
default:
    // Fallback for old data without Status field
    if r.Converted {
        conversions++
    } else {
        rejections++
    }
}
```

### 5. Enhanced Reporting
**File**: `report.go:171-250`

**Overall Summary:**
```
Persona Decisions:
  Valid: 45 / 50 (90%)
    Converted: 3 (6.7% of valid)
    Rejected:  42 (93.3% of valid)
  Errors: 5 (parse/LLM failures)
```

**Per-Idea Breakdown:**
```
#1 Score:0.355 Conv:7% | 3✓ 42✗ 5⚠ | Finance → Creators | Ai Llms
     Strong market: creators in global
```

Legend:
- `3✓` = 3 converted
- `42✗` = 42 rejected
- `5⚠` = 5 errors

## Benefits

### 1. **Accurate Conversion Rates**
**Before**: Errors mixed with rejections → inflated denominators  
**After**: `convRate = conversions / (conversions + rejections)` → excludes errors

**Example:**
- Old: 3 converted / 50 total = 6%
- New: 3 converted / 45 valid = 6.7% (5 errors excluded)

### 2. **Data Quality Visibility**
Can now see at a glance:
- How many personas had valid reasoning (converted + rejected)
- How many failed due to technical issues (errors)
- Whether high error rates indicate prompt/parsing problems

### 3. **Better Debugging**
When error rate is high (>20%), you know to:
- Check prompt formatting
- Verify JSON schema compliance
- Look at retry logic effectiveness

### 4. **Realistic Baselines**
With error tracking:
- 90%+ valid personas = good data quality
- 2-5% conversion (of valid) = realistic SaaS baseline
- <10% errors = acceptable noise

## Example Output

### Good Run (10 ideas × 10 personas)
```
Persona Decisions:
  Valid: 88 / 100 (88%)
    Converted: 4 (4.5% of valid)
    Rejected:  84 (95.5% of valid)
  Errors: 12 (parse/LLM failures)

Top 10 Ideas:
  #1 Score:0.412 Conv:10% | 1✓ 9✗ 0⚠ | Finance → Creators
  #2 Score:0.201 Conv:0%  | 0✓ 8✗ 2⚠ | Education → Students
  ...
```

**Interpretation:**
- ✅ 88% valid decision rate (good)
- ✅ 4.5% conversion (realistic)
- ✅ Idea #1 has clean data (no errors)
- ⚠️ Idea #2 has 2 errors (20% error rate - investigate)

### Bad Run (before fixes)
```
Persona Decisions:
  Valid: 20 / 100 (20%)
    Converted: 3 (15% of valid)
    Rejected:  17 (85% of valid)
  Errors: 80 (parse/LLM failures)
```

**Interpretation:**
- ❌ 20% valid decision rate (terrible)
- ❌ 80% errors (prompts broken or rate-limited)
- ⚠️ 15% conversion might be artifact of small valid sample (only 20)

## Testing

### Quick Validation
```bash
cd ~/workspace/startup-factory/go-validator

./startup-validator \
  -ideas 5 \
  -personas 10 \
  -epochs 1 \
  -enrichment-mode maxclaw \
  -output results/status-test \
  -max-concurrent 50 \
  -verbose
```

**Check output for:**
1. ✓ Overall valid % (target: >80%)
2. ✓ Conversion % of valid (target: 2-8%)  
3. ✓ Error count (target: <20%)
4. ✓ Per-idea breakdown shows `X✓ Y✗ Z⚠` format

### JSON Verification
```bash
cat results/status-test/*.json | jq '.scored_ideas[0].simulations[] | {name, status, converted}'
```

**Expected output:**
```json
{"name": "Alice", "status": "rejected", "converted": false}
{"name": "Bob", "status": "converted", "converted": true}
{"name": "Carol", "status": "error", "converted": false}
```

## Files Modified

- `types.go` - Added Status field + metrics
- `helpers.go` - Status assignment in parseSimulation
- `pipeline.go` - Status="error" on LLM failures
- `scoring.go` - Count conversions/rejections/errors separately
- `report.go` - Display breakdown in summary + per-idea

## Binary

✅ `startup-validator` rebuilt with status tracking
