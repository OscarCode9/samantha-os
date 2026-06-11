#!/bin/bash
# Test script for elementary-claw cron functionality

set -e

echo "=== Testing elementary-claw Cron Implementation ==="
echo ""

# 1. Inicializar workspace si no existe
echo "[1] Setting up workspace..."
claw setup --user testuser > /dev/null 2>&1 || true

# 2. Test: List jobs (debe estar vacía)
echo "[2] Testing cron list (should be empty)..."
claw cron list

# 3. Test: Create a cron job con schedule cron
echo ""
echo "[3] Creating a daily job (9 AM)..."
JOB_ID=$(claw cron create \
  --name "Daily Morning" \
  --schedule '{"kind":"cron","expr":"0 9 * * *"}' \
  --payload '{"kind":"systemEvent","data":{"text":"Good morning"}}' \
  --delivery announce | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

echo "Created job: $JOB_ID"

# 4. Test: Create un interval job
echo ""
echo "[4] Creating an interval job (every 5 minutes)..."
JOB_ID_2=$(claw cron create \
  --name "Health Check" \
  --schedule '{"kind":"interval","intervalMs":300000}' \
  --payload '{"kind":"systemEvent","data":{"text":"Checking system"}}' | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

echo "Created job: $JOB_ID_2"

# 5. Test: List jobs (debe mostrar 2)
echo ""
echo "[5] Listing jobs (should show 2)..."
claw cron list

# 6. Test: Get job details
echo ""
echo "[6] Getting job details..."
claw cron get "$JOB_ID" | head -20

# 7. Test: Cron status
echo ""
echo "[7] Checking cron status..."
claw cron status

# 8. Test: Run job manually
echo ""
echo "[8] Running a job manually..."
claw cron run "$JOB_ID"

# 9. Test: Update job (disable it)
echo ""
echo "[9] Disabling a job..."
claw cron update "$JOB_ID" --patch '{"enabled":false}'

# 10. Test: List jobs (first should be disabled)
echo ""
echo "[10] Listing jobs after disabling..."
claw cron list

# 11. Test: Delete job
echo ""
echo "[11] Deleting a job..."
claw cron delete "$JOB_ID_2"

# 12. Test: Final count
echo ""
echo "[12] Final job count (should be 1)..."
claw cron list

# 13. HTTP API Test
echo ""
echo "[13] Testing HTTP API (/cron/status)..."
# Nota: Esto funcionaría si tenemos el gateway ejecutándose
# curl -s http://localhost:4389/cron/status | head -20 || echo "Gateway not running"

echo ""
echo "=== All tests completed ==="
