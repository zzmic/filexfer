#!/bin/bash

echo "=== filexfer large directory test script ==="
echo "Testing large directory transfers..."

# Create a large test directory structure.
echo "Creating large test directory structure..."
mkdir -p ./large_test_dir/{docs,images,videos,data,backup}

# Create various file sizes to simulate more significant directory sizes.
echo "Creating files of varying sizes..."

# Small files (1KB - 1MB).
for i in {1..100}; do
    dd if=/dev/urandom of=./large_test_dir/docs/document_"$i".txt bs=1K count=$((RANDOM % 1000 + 1)) 2>/dev/null
done

# Medium files (1MB - 10MB).
for i in {1..50}; do
    dd if=/dev/urandom of=./large_test_dir/images/image_"$i".jpg bs=1M count=$((RANDOM % 10 + 1)) 2>/dev/null
done

# Large files (10MB - 100MB).
for i in {1..20}; do
    dd if=/dev/urandom of=./large_test_dir/videos/video_"$i".mp4 bs=1M count=$((RANDOM % 90 + 10)) 2>/dev/null
done

# Even larger files (100MB - 500MB).
for i in {1..5}; do
    dd if=/dev/urandom of=./large_test_dir/data/dataset_"$i".bin bs=1M count=$((RANDOM % 400 + 100)) 2>/dev/null
done

# Create nested directories with files.
mkdir -p ./large_test_dir/backup/{daily,weekly,monthly}
for i in {1..30}; do
    dd if=/dev/urandom of=./large_test_dir/backup/daily/backup_"$i".tar.gz bs=1M count=$((RANDOM % 50 + 5)) 2>/dev/null
done

for i in {1..10}; do
    dd if=/dev/urandom of=./large_test_dir/backup/weekly/weekly_"$i".tar.gz bs=1M count=$((RANDOM % 100 + 20)) 2>/dev/null
done

for i in {1..5}; do
    dd if=/dev/urandom of=./large_test_dir/backup/monthly/monthly_"$i".tar.gz bs=1M count=$((RANDOM % 200 + 50)) 2>/dev/null
done

# Calculate the total size of the directory.
echo "Calculating directory size..."
total_size=$(du -sk ./large_test_dir | cut -f1)
total_size_bytes=$((total_size * 1024))
echo "Total directory size: $((total_size_bytes / 1024 / 1024)) MB"

# Build the applications.
echo "Building applications..."
go build -o ./bin/client ./cmd/client/main.go && go build -o ./bin/server ./cmd/server/main.go

# Check if the port is already in use.
if lsof -Pi :8080 -sTCP:LISTEN -t >/dev/null ; then
    echo "Port 8080 is already in use. Killing the existing process..."
    kill "$(lsof -Pi :8080 -sTCP:LISTEN -t)"
    sleep 3
fi

# Start the server.
echo "Starting server..."
./bin/server -port 8080 -dir ./large_test_output &
SERVER_PID=$!
sleep 3

# Test large directory transfer.
echo -e "\n=== Testing Large Directory Transfer ==="
echo "Transferring large directory (${total_size} bytes)..."
time ./bin/client -server localhost:8080 -file ./large_test_dir

# Verify the transfer.
echo -e "\n=== Verification ==="
echo "Checking transferred files..."
ls -la ./large_test_output/large_test_dir/ 2>/dev/null || echo "Directory not found"

# Count files in original vs transferred.
original_count=$(find ./large_test_dir -type f | wc -l)
transferred_count=$(find ./large_test_output -type f 2>/dev/null | wc -l)

echo "Original files: $original_count"
echo "Transferred files: $transferred_count"

if [ "$original_count" -eq "$transferred_count" ]; then
    echo "SUCCESS: All files transferred correctly (no duplicates and no missing files)"
else
    echo "FAILURE: File count mismatch (duplicates or missing files)"
fi

# Clean up.
echo -e "\n=== Cleaning up ==="
kill $SERVER_PID 2>/dev/null
rm -rf ./large_test_dir
rm -rf ./large_test_output

echo "=== Large directory test completed! ==="
