#!/bin/bash
# Build script for reminder app

set -e

echo "📦 Installing dependencies..."
go mod download

echo "🔨 Building..."
go build -o reminder .

echo "✅ Build complete! Run with: ./reminder"
echo ""
echo "Options:"
echo "  ./reminder --port 8080 --db reminder.db"
echo "  ./reminder --port 8080 --base-url http://yourserver.com"
