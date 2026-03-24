#!/bin/bash
# 3-way enrichment mode comparison: maxclaw vs programmatic vs llm-lfm
# 10 ideas × 10 personas × 1 epoch

echo "======================================================================="
echo "ENRICHMENT MODE COMPARISON TEST"
echo "======================================================================="
echo "Test: 10 ideas × 10 personas × 1 epoch"
echo "Modes: maxclaw | programmatic | llm-lfm"
echo "Expected: ~5-10 minutes total (all parallel)"
echo "======================================================================="
echo ""

# Create output directories
mkdir -p results/maxclaw results/programmatic results/lfm

# Run all 3 in parallel
echo "Starting 3 parallel runs..."
echo ""

./startup-validator \
  -ideas 10 \
  -personas 10 \
  -epochs 1 \
  -enrichment-mode maxclaw \
  -output results/maxclaw \
  -max-concurrent 100 \
  -verbose &
PID_MAXCLAW=$!

./startup-validator \
  -ideas 10 \
  -personas 10 \
  -epochs 1 \
  -enrichment-mode programmatic \
  -output results/programmatic \
  -max-concurrent 100 \
  -verbose &
PID_PROG=$!

./startup-validator \
  -ideas 10 \
  -personas 10 \
  -epochs 1 \
  -enrichment-mode llm-lfm \
  -output results/lfm \
  -max-concurrent 100 \
  -verbose &
PID_LFM=$!

echo "PIDs: MaxClaw=$PID_MAXCLAW | Programmatic=$PID_PROG | LFM=$PID_LFM"
echo ""

# Wait for all to complete
wait $PID_MAXCLAW
MAXCLAW_EXIT=$?

wait $PID_PROG
PROG_EXIT=$?

wait $PID_LFM
LFM_EXIT=$?

echo ""
echo "======================================================================="
echo "RESULTS"
echo "======================================================================="
echo "MaxClaw:      exit code $MAXCLAW_EXIT"
echo "Programmatic: exit code $PROG_EXIT"
echo "LFM:          exit code $LFM_EXIT"
echo ""

# Show HTML reports
echo "HTML Reports:"
echo "  MaxClaw:      $(ls -1t results/maxclaw/*.html | head -1)"
echo "  Programmatic: $(ls -1t results/programmatic/*.html | head -1)"
echo "  LFM:          $(ls -1t results/lfm/*.html | head -1)"
echo ""

echo "Compare conversion rates and learning outcomes in the HTML reports!"
echo "======================================================================="
