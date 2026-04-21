#!/bin/bash
set -e

echo "=== Stash Memory System Test ==="
echo "This script tests the core memory functionality with a real embedder."
echo "Make sure your .env file is configured correctly."
echo

# Build fresh binary
echo "1. Building CLI..."
go build -o stash ./cmd/cli
echo "✓ Built"

# Clear any existing context
echo "2. Clearing previous context..."
PGPASSWORD=postgres psql -h localhost -p 5432 -U postgres -d stash -c "DELETE FROM records WHERE id = '_memory.context';" 2>/dev/null || true
echo "✓ Cleared"

# Test remember
echo "3. Testing 'remember'..."
./stash remember "test event: discussed project architecture with team" --metadata '{"meeting_type":"planning"}' || {
    echo "✗ remember failed"
    exit 1
}
echo "✓ remember works"

# Test recall
echo "4. Testing 'recall'..."
./stash recall "project architecture discussion" --limit 1 >/dev/null || {
    echo "✗ recall failed"
    exit 1
}
echo "✓ recall works"

# Test context update
echo "5. Testing 'context --update'..."
./stash context --update "architecture planning" >/dev/null || {
    echo "✗ context update failed"
    exit 1
}
echo "✓ context update works"

# Test context view
echo "6. Testing 'context' view..."
./stash context | grep -q "Focus:" || {
    echo "✗ context view failed"
    exit 1
}
echo "✓ context view works"

# Test list
echo "7. Testing 'list'..."
./stash list --limit 1 >/dev/null || {
    echo "✗ list failed"
    exit 1
}
echo "✓ list works"

# Test delete (soft delete)
echo "8. Testing 'delete'..."
# First get an event ID
EVENT_ID=$(./stash list --limit 1 --json 2>/dev/null | grep -o '"ID":"[^"]*"' | head -1 | cut -d'"' -f4)
if [ -n "$EVENT_ID" ]; then
    ./stash delete "$EVENT_ID" >/dev/null 2>&1 || {
        echo "✗ delete failed"
        exit 1
    }
    echo "✓ delete works"
else
    echo "⚠ No event to delete (skipping)"
fi

# Test env
echo "9. Testing 'env'..."
./stash env | grep -q "STASH_" || {
    echo "✗ env failed"
    exit 1
}
echo "✓ env works"

echo
echo "=== All Tests Passed ==="
echo "Memory system is functional with real embedder."
echo
echo "Quick verification:"
echo "  Total records: $(PGPASSWORD=postgres psql -h localhost -p 5432 -U postgres -d stash -t -c "SELECT COUNT(*) FROM records;" 2>/dev/null || echo "N/A")"
echo "  Context focus: $(PGPASSWORD=postgres psql -h localhost -p 5432 -U postgres -d stash -t -c "SELECT metadata->'_memory'->>'focus' FROM records WHERE id = '_memory.context';" 2>/dev/null || echo "N/A")"