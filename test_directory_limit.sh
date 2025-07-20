#!/bin/bash

echo "=== filexfer directory size limit test script ==="
echo "Testing directory size limit functionality..."

# Create a test directory with files that would exceed the limit.
echo "Creating test directory structure..."
mkdir -p ./test_dir_limit/{small,medium,large}

# Create 10 small files (1MB each) => 10MB.
echo "Creating small files..."
for i in {1..10}; do
    dd if=/dev/urandom of=./test_dir_limit/small/file_"$i".txt bs=1M count=1 2>/dev/null
done

# Create 10 medium files (10MB each) => 100MB.
echo "Creating medium files..."
for i in {1..10}; do
    dd if=/dev/urandom of=./test_dir_limit/medium/file_"$i".dat bs=1M count=10 2>/dev/null
done

# Create 10 large files (100MB each) => 1GB.
echo "Creating large files..."
for i in {1..10}; do
    dd if=/dev/urandom of=./test_dir_limit/large/file_"$i".bin bs=1M count=100 2>/dev/null
done

# Calculate the total size of the directory.
echo "Calculating directory size..."
total_size=$(du -sk ./test_dir_limit | cut -f1)
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

# Test 1: Directory transfer at/below 50GB limit (should succeed).
echo -e "\n=== Test 1: Directory Transfer At/Below 50GB Limit ==="
echo "Creating directory structure (~45GB)..."
mkdir -p ./test_dir_45gb/{huge1,huge2,huge3,huge4}

# Create 4 directories with ~11.25GB each to stay under 50GB total.
echo "Creating huge files (~11.25GB each directory)..."
for dir in huge1 huge2 huge3 huge4; do
    echo "Creating files in $dir..."
    for i in {1..11}; do
        dd if=/dev/urandom of=./test_dir_45gb/$dir/file_"$i".bin bs=1M count=1000 2>/dev/null
    done
    # Add one more file with ~250MB to get closer to 11.25GB.
    dd if=/dev/urandom of=./test_dir_45gb/$dir/file_12.bin bs=1M count=250 2>/dev/null
done

# Calculate the total size of the directory.
echo "Calculating directory size..."
total_size=$(du -sk ./test_dir_45gb | cut -f1)
total_size_bytes=$((total_size * 1024))
echo "Directory size: $((total_size_bytes / 1024 / 1024 / 1024)) GB"

echo "Starting server with 50GB limit..."
./bin/server -port 8080 -dir ./test_output_1 -max-dir-size 53687091200 &
SERVER_PID_1=$!
sleep 3

echo "Transferring directory (should succeed)..."
if ./bin/client -server localhost:8080 -file ./test_dir_45gb; then
    echo "SUCCESS: Directory transfer completed successfully"
else
    echo "FAILURE: Directory transfer failed unexpectedly"
fi

kill $SERVER_PID_1 2>/dev/null
sleep 3

# Test 2: Directory transfer exceeding 50GB limit (should fail).
echo -e "\n=== Test 2: Directory Transfer Exceeding 50GB Limit ==="
echo "Creating large directory structure (~55GB)..."
mkdir -p ./test_dir_55gb/{huge1,huge2,huge3,huge4,huge5}

# Create 5 directories with ~11GB each to exceed 50GB total.
echo "Creating huge files (~11GB each directory)..."
for dir in huge1 huge2 huge3 huge4 huge5; do
    echo "Creating files in $dir..."
    for i in {1..11}; do
        dd if=/dev/urandom of=./test_dir_55gb/$dir/file_"$i".bin bs=1M count=1000 2>/dev/null
    done
done

# Calculate the total size of the large directory.
echo "Calculating large directory size..."
large_total_size=$(du -sk ./test_dir_55gb | cut -f1)
large_total_size_bytes=$((large_total_size * 1024))
echo "Large directory size: $((large_total_size_bytes / 1024 / 1024 / 1024)) GB"

echo "Starting server with 50GB limit..."
./bin/server -port 8080 -dir ./test_output_2 -max-dir-size 53687091200 &
SERVER_PID_2=$!
sleep 3

echo "Transferring large directory (should fail due to 50GB limit)..."
if ./bin/client -server localhost:8080 -file ./test_dir_55gb; then
    echo "FAILURE: Large directory transfer succeeded when it should have failed"
else
    echo "SUCCESS: Large directory transfer correctly rejected due to 50GB limit"
fi

kill $SERVER_PID_2 2>/dev/null

# Clean up.
echo -e "\n=== Cleaning up ==="
rm -rf ./test_dir_45gb
rm -rf ./test_dir_55gb
rm -rf ./test_output_1
rm -rf ./test_output_2
rm -rf ./test_dir_limit

echo "=== Directory size limit test completed! ==="
