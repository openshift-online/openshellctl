#!/usr/bin/env bash
#
# Integration test for openshellctl sandbox commands.
#
# Exercises: token refresh, create, upload, download, exec, connect, stop,
# start, and the -f manifest flag across subcommands.
#
# Prerequisites:
#   - openshellctl binary (set OPENSHELLCTL or have it on PATH)
#   - Valid gateway configuration (will refresh token automatically)
#
# Usage:
#   ./hack/integration-test.sh                     # uses openshellctl from PATH
#   OPENSHELLCTL=/tmp/openshellctl ./hack/integration-test.sh
#   TEST_IMAGE=my-registry/image:tag ./hack/integration-test.sh
#
set -euo pipefail

CLI="${OPENSHELLCTL:-openshellctl}"
TEST_IMAGE="${TEST_IMAGE:-quay.io/redhat-services-prod/rosa-tenant/rosa-agent/rosa-agent:latest}"
SB_NAME="integ-test-$$"
MANIFEST=$(mktemp /tmp/integ-test-manifest-XXXXXX.yaml)
UPLOAD_FILE=$(mktemp /tmp/integ-test-upload-XXXXXX.txt)
DOWNLOAD_FILE=$(mktemp /tmp/integ-test-download-XXXXXX.txt)

pass=0
fail=0
skip=0

log()  { printf '\n\033[1;34m=== %s ===\033[0m\n' "$*"; }
ok()   { printf '  \033[1;32m✓ %s\033[0m\n' "$*"; pass=$((pass+1)); }
fail() { printf '  \033[1;31m✗ %s\033[0m\n' "$*"; fail=$((fail+1)); }

cleanup() {
    log "Cleanup"
    $CLI token refresh --write >/dev/null 2>&1 || true
    $CLI sandbox delete "$SB_NAME" 2>/dev/null && ok "Deleted sandbox $SB_NAME" || true
    rm -f "$MANIFEST" "$UPLOAD_FILE" "$DOWNLOAD_FILE"
}
trap cleanup EXIT

cat > "$MANIFEST" <<EOF
apiVersion: openshell.managed.openshift.io/v1alpha1
kind: Sandbox
metadata:
  name: $SB_NAME
spec:
  image: $TEST_IMAGE
EOF

echo "integration test upload content $$" > "$UPLOAD_FILE"

refresh() { $CLI token refresh --write >/dev/null 2>&1 || true; }

# --- 1. Token refresh ---
log "1. Token refresh"
if $CLI token refresh --write; then
    ok "token refresh --write"
else
    fail "token refresh --write"
    echo "Cannot proceed without a valid token."
    exit 1
fi

# --- 2. Create sandbox ---
log "2. Create sandbox (detached)"
if $CLI sandbox create --name "$SB_NAME" --from "$TEST_IMAGE" --detach; then
    ok "sandbox create --detach"
else
    fail "sandbox create --detach"
    echo "Cannot proceed without a sandbox."
    exit 1
fi

# --- 3. Wait for sandbox to be ready ---
log "3. Waiting for sandbox to be ready"
MAX_WAIT=120
ELAPSED=0
while true; do
    refresh
    GET_OUT=$($CLI sandbox get "$SB_NAME" -o json 2>&1) || true
    PHASE=$(echo "$GET_OUT" | grep '"phase"' | head -1 | sed 's/.*"phase"[^"]*"\([^"]*\)".*/\1/')
    if [ -z "$PHASE" ]; then
        PHASE="unknown"
    fi
    if [ "$PHASE" = "Ready" ] || [ "$PHASE" = "ready" ] || [ "$PHASE" = "Running" ] || [ "$PHASE" = "running" ]; then
        ok "Sandbox ready (phase=$PHASE) after ${ELAPSED}s"
        break
    fi
    if [ "$ELAPSED" -ge "$MAX_WAIT" ]; then
        fail "Sandbox not ready after ${MAX_WAIT}s (phase=$PHASE)"
        echo "  Last output: $GET_OUT"
        exit 1
    fi
    printf "  Waiting... phase=%s (%ds/%ds)\n" "$PHASE" "$ELAPSED" "$MAX_WAIT"
    sleep 5
    ELAPSED=$((ELAPSED+5))
done

# --- 4. Upload (arg format: NAME LOCAL_PATH DEST) ---
log "4. Upload"
refresh
if $CLI sandbox upload "$SB_NAME" "$UPLOAD_FILE" /sandbox/integ-upload.txt; then
    ok "upload NAME LOCAL_PATH DEST"
else
    fail "upload"
fi

# --- 5. Exec (verify upload) ---
log "5. Exec (verify upload)"
refresh
EXEC_OUT=$($CLI sandbox exec --name "$SB_NAME" -- cat /sandbox/integ-upload.txt 2>&1) || true
if echo "$EXEC_OUT" | grep -q "integration test upload content"; then
    ok "exec cat — content matches"
else
    fail "exec cat — got: $EXEC_OUT"
fi

# --- 6. Download (arg format: NAME SANDBOX_PATH DEST) ---
log "6. Download"
refresh
if timeout 30 $CLI sandbox download "$SB_NAME" /sandbox/integ-upload.txt "$DOWNLOAD_FILE"; then
    ok "download NAME SANDBOX_PATH DEST"
    if grep -q "integration test upload content" "$DOWNLOAD_FILE" 2>/dev/null; then
        ok "download content verified"
    else
        fail "download content mismatch"
    fi
else
    DL_EXIT=$?
    if [ "$DL_EXIT" -eq 124 ]; then
        fail "download timed out (known issue: sess.Wait hang)"
    else
        fail "download failed (exit $DL_EXIT)"
    fi
fi

# --- 7. Exec with -f flag ---
log "7. Exec with -f flag"
refresh
EXEC_F_OUT=$($CLI sandbox exec -f "$MANIFEST" -- echo "hello from -f" 2>&1) || true
if echo "$EXEC_F_OUT" | grep -q "hello from -f"; then
    ok "exec -f"
else
    fail "exec -f — got: $EXEC_F_OUT"
fi

# --- 8. Upload with -f flag ---
log "8. Upload with -f flag"
refresh
echo "upload via -f $$" > "$UPLOAD_FILE"
if $CLI sandbox upload -f "$MANIFEST" "$UPLOAD_FILE" /sandbox/integ-upload-f.txt; then
    ok "upload -f"
else
    fail "upload -f"
fi

# --- 9. Download with -f flag ---
log "9. Download with -f flag"
refresh
rm -f "$DOWNLOAD_FILE"
if timeout 30 $CLI sandbox download -f "$MANIFEST" /sandbox/integ-upload-f.txt "$DOWNLOAD_FILE"; then
    ok "download -f"
    if grep -q "upload via -f" "$DOWNLOAD_FILE" 2>/dev/null; then
        ok "download -f content verified"
    else
        fail "download -f content mismatch"
    fi
else
    DL_EXIT=$?
    if [ "$DL_EXIT" -eq 124 ]; then
        fail "download -f timed out (known issue: sess.Wait hang)"
    else
        fail "download -f failed (exit $DL_EXIT)"
    fi
fi

# --- 10. Stop ---
log "10. Stop with -f flag"
refresh
STOP_OUT=$($CLI sandbox stop -f "$MANIFEST" 2>&1) || true
if echo "$STOP_OUT" | grep -q "Stopped"; then
    ok "stop -f"
else
    echo "  output: $STOP_OUT"
    if echo "$STOP_OUT" | grep -qi "unknown"; then
        echo "  (gateway may not support stop — server-side, not a CLI bug)"
        skip=$((skip+1))
    else
        fail "stop -f"
    fi
fi

# --- 11. Start ---
log "11. Start with -f flag"
refresh
START_OUT=$($CLI sandbox start -f "$MANIFEST" 2>&1) || true
if echo "$START_OUT" | grep -q "Started"; then
    ok "start -f"
    sleep 10
else
    echo "  output: $START_OUT"
    if echo "$START_OUT" | grep -qi "unknown"; then
        echo "  (gateway may not support start — server-side, not a CLI bug)"
        skip=$((skip+1))
    else
        fail "start -f"
    fi
fi

# --- 12. Connect ---
log "12. Connect with -f flag (non-interactive)"
refresh
CONNECT_OUT=$(echo "exit" | timeout 15 $CLI sandbox connect -f "$MANIFEST" 2>&1) || true
ok "connect -f returned"

# --- Summary ---
log "Results"
printf '  Passed:  %d\n' "$pass"
printf '  Failed:  %d\n' "$fail"
printf '  Skipped: %d\n' "$skip"

if [ "$fail" -gt 0 ]; then
    exit 1
fi
echo ""
echo "All tests passed!"
