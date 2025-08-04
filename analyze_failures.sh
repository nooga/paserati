#!/bin/bash

# Script to analyze test262 failure patterns
# Usage: ./analyze_failures.sh language_result.txt

if [ $# -eq 0 ]; then
    echo "Usage: $0 <result_file>"
    echo "Example: $0 language_result.txt"
    exit 1
fi

RESULT_FILE="$1"

if [ ! -f "$RESULT_FILE" ]; then
    echo "Error: File '$RESULT_FILE' not found"
    exit 1
fi

echo "=== Test262 Failure Analysis ==="
echo "File: $RESULT_FILE"
echo

# Count total failures
TOTAL_FAILURES=$(grep -c "^FAIL" "$RESULT_FILE")
echo "Total failures: $TOTAL_FAILURES"
echo

# Extract and count error patterns
echo "=== Top 20 Error Patterns ==="
grep "test failed:" "$RESULT_FILE" | \
    sed 's/.*test failed: //' | \
    sed 's/Runtime Error at [0-9]*:[0-9]*: //' | \
    sed 's/Syntax Error at [0-9]*:[0-9]*: //' | \
    sed 's/Compile Error at [0-9]*:[0-9]*: //' | \
    sort | uniq -c | sort -nr | head -20

echo
echo "=== Syntax Error Patterns ==="
grep "Syntax Error" "$RESULT_FILE" | \
    sed 's/.*Syntax Error at [0-9]*:[0-9]*: //' | \
    sort | uniq -c | sort -nr | head -10

echo
echo "=== Runtime Error Patterns ==="
grep "Runtime Error" "$RESULT_FILE" | \
    sed 's/.*Runtime Error at [0-9]*:[0-9]*: //' | \
    sort | uniq -c | sort -nr | head -10

echo
echo "=== Test262Error Patterns ==="
grep "Test262Error:" "$RESULT_FILE" | \
    sed 's/.*Test262Error: //' | \
    sort | uniq -c | sort -nr | head -10

echo
echo "=== Cannot Patterns ==="
grep "Cannot" "$RESULT_FILE" | \
    sed 's/.*test failed: Runtime Error at [0-9]*:[0-9]*: //' | \
    sort | uniq -c | sort -nr | head -10

echo
echo "=== Expected Token Patterns ==="
grep "expected.*token.*got" "$RESULT_FILE" | \
    sed 's/.*expected/expected/' | \
    sort | uniq -c | sort -nr | head -10

echo
echo "=== Summary by Test Directory ==="
grep "^FAIL" "$RESULT_FILE" | \
    sed 's|.*test262/test/language/||' | \
    sed 's|/.*||' | \
    sort | uniq -c | sort -nr | head -15