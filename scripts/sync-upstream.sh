#!/bin/bash
# Tetora Upstream Sync Script
# Usage: ./scripts/sync-upstream.sh

set -e

echo "🔄 Starting upstream synchronization..."

# 1. Switch to main branch
echo "📍 Switching to main branch..."
git checkout main

# 2. Fetch latest changes from upstream
echo "📡 Fetching from upstream..."
git fetch upstream

# 3. Merge upstream/main into local main
echo "🔄 Merging upstream/main into main..."
git merge upstream/main

# 4. Push updated main to origin
echo "📤 Pushing to origin/main..."
git push origin main

echo "✅ Main branch is now synced with upstream!"
