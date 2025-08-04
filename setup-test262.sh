#!/bin/bash

# setup-test262.sh - Script to set up Test262 test suite for Paserati
set -e

TEST262_DIR="test262"
TEST262_REPO="https://github.com/tc39/test262.git"

echo "Setting up Test262 test suite for Paserati..."

# Check if test262 directory already exists
if [ -d "$TEST262_DIR" ]; then
    echo "Test262 directory already exists at $TEST262_DIR"
    read -p "Do you want to update it? (y/N): " -r
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "Updating existing Test262 repository..."
        cd "$TEST262_DIR"
        git pull origin main
        cd ..
        echo "Test262 updated successfully!"
    else
        echo "Using existing Test262 installation."
    fi
else
    echo "Cloning Test262 repository..."
    git clone "$TEST262_REPO" "$TEST262_DIR"
    echo "Test262 cloned successfully!"
fi

# Add test262 to .gitignore if not already present
if ! grep -q "^test262/$" .gitignore 2>/dev/null; then
    echo "Adding test262/ to .gitignore..."
    echo "" >> .gitignore
    echo "# Test262 test suite (cloned by setup-test262.sh)" >> .gitignore
    echo "test262/" >> .gitignore
    echo "Added test262/ to .gitignore"
else
    echo "test262/ already in .gitignore"
fi

# Build the test262 runner
echo "Building paserati-test262 runner..."
go build -o paserati-test262 ./cmd/paserati-test262/

# Show some info about the test suite
echo ""
echo "=== Test262 Setup Complete ==="
echo "Test suite location: $(pwd)/$TEST262_DIR"
echo "Test count: $(find $TEST262_DIR/test -name "*.js" | wc -l | xargs) JavaScript files"
echo "Runner binary: $(pwd)/paserati-test262"
echo ""
echo "Usage examples:"
echo "  # Run first 10 tests with verbose output"
echo "  ./paserati-test262 -path $TEST262_DIR -limit 10 -verbose"
echo ""
echo "  # Run tests matching a pattern"
echo "  ./paserati-test262 -path $TEST262_DIR -pattern '*Array*' -limit 50"
echo ""
echo "  # Run full test suite (this will take a while!)"
echo "  ./paserati-test262 -path $TEST262_DIR"
echo ""
echo "Happy testing! 🚀"