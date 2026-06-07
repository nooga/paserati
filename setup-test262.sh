#!/bin/bash

# setup-test262.sh - Clone and pin Test262 to the revision in .test262-rev.
#
# Non-interactive. Always brings the local checkout to the pinned SHA, even
# if it was previously on a different commit (e.g. main HEAD from an older
# version of this script). To bump the pin, edit .test262-rev and re-run.
set -e

TEST262_DIR="test262"
TEST262_REPO="https://github.com/tc39/test262.git"
REV_FILE=".test262-rev"

if [ ! -f "$REV_FILE" ]; then
    echo "Error: $REV_FILE not found. Create it with the desired Test262 SHA." >&2
    exit 1
fi

PINNED_REV=$(grep -v '^#' "$REV_FILE" | head -1 | tr -d '[:space:]')
if [ -z "$PINNED_REV" ]; then
    echo "Error: $REV_FILE is empty (no SHA recorded)." >&2
    exit 1
fi

echo "Setting up Test262 at pinned revision: $PINNED_REV"

if [ ! -d "$TEST262_DIR" ]; then
    echo "Cloning $TEST262_REPO into $TEST262_DIR/..."
    git clone "$TEST262_REPO" "$TEST262_DIR"
fi

CURRENT_REV=$(git -C "$TEST262_DIR" rev-parse HEAD)
if [ "$CURRENT_REV" != "$PINNED_REV" ]; then
    echo "Fetching and checking out pinned revision..."
    git -C "$TEST262_DIR" fetch --quiet origin "$PINNED_REV" || \
        git -C "$TEST262_DIR" fetch --quiet origin
    git -C "$TEST262_DIR" checkout --quiet --detach "$PINNED_REV"
fi

# Add test262 to .gitignore if not already present (recognizes both anchored
# `/test262/` and bare `test262/` forms — without this dual check the bare form
# gets re-appended every run).
if ! grep -Eq "^/?test262/$" .gitignore 2>/dev/null; then
    {
        echo ""
        echo "# Test262 test suite (cloned by setup-test262.sh)"
        echo "test262/"
    } >> .gitignore
    echo "Added test262/ to .gitignore"
fi

echo "Building paserati-test262 runner..."
go build -o paserati-test262 ./cmd/paserati-test262/

echo ""
echo "=== Test262 Setup Complete ==="
echo "Location:  $(pwd)/$TEST262_DIR"
echo "Revision:  $(git -C "$TEST262_DIR" rev-parse HEAD) (pinned via $REV_FILE)"
echo "Test count: $(find "$TEST262_DIR/test" -name '*.js' | wc -l | xargs) JavaScript files"
echo "Runner:    $(pwd)/paserati-test262"
echo ""
echo "Usage examples:"
echo "  ./paserati-test262 -path $TEST262_DIR -limit 10 -verbose"
echo "  ./paserati-test262 -path $TEST262_DIR -pattern '*Array*' -limit 50"
echo "  ./paserati-test262 -path $TEST262_DIR"
