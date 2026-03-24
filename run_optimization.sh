#!/bin/bash
# Full meta-learning optimization runner
# Runtime: 8-10 hours
# Cost: $0 (local 9B model)

cd /home/aid/workspace/startup-factory

echo "========================================================================"
echo "STARTING FULL STARTUP IDEA OPTIMIZATION"
echo "========================================================================"
echo "Started: $(date)"
echo "Profile: profile.json (Aidyn Torgayev)"
echo "Iterations: 10"
echo "Ideas per iteration: 10"
echo "Total ideas to test: 100"
echo "Expected runtime: 8-10 hours"
echo "Cost: $0 (local 9B model)"
echo ""
echo "Output will be logged to: optimization_run.log"
echo "Monitor progress: tail -f /home/aid/workspace/startup-factory/optimization_run.log"
echo "========================================================================"
echo ""

# Run optimization with output logging
python3 auto_validate.py \
    --iterations 10 \
    --batch-size 10 \
    --min-score 0.35 \
    2>&1 | tee optimization_run.log

echo ""
echo "========================================================================"
echo "OPTIMIZATION COMPLETE"
echo "========================================================================"
echo "Finished: $(date)"
echo "Results saved to: results/auto_validation/"
echo "Top ideas in: results/auto_validation/final_report.json"
echo "========================================================================"
