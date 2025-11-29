#!/bin/bash

echo "=== filexfer test script ==="
echo "Starting testing..."

# Create the test files and directories.
echo "Creating test files..."
mkdir -p ./test/subdir1/subdir2
echo "./test/root_file.txt content" > ./test/root_file.txt
echo "./test/subdir1/file1.txt content" > ./test/subdir1/file1.txt
echo "./test/subdir1/subdir2/file2.txt content" > ./test/subdir1/subdir2/file2.txt
echo "./test/file2.txt content" > ./test/file2.txt
echo "./test/file3.txt content" > ./test/file3.txt

# Create more test files of different scenarios.
echo "./test/file0.txt content" > ./test/file0.txt
echo '{"key": "value"}' > ./test/file1.txt
echo '{"another_key": "another_value"}' > ./test.json
touch ./empty_file.txt
dd if=/dev/zero of=./large_file.dat bs=1M count=500 2>/dev/null # Create a large file of size 500MB which is simply zero-filled.

# Build the applications.
echo "Building applications..."
go build -o ./bin/client ./cmd/client/main.go && go build -o ./bin/server ./cmd/server/main.go

# Check if the port is already in use.
# If it is, kill the existing process.
if lsof -Pi :8080 -sTCP:LISTEN -t >/dev/null ; then
    echo "Port 8080 is already in use. Killing the existing process..."
    kill "$(lsof -Pi :8080 -sTCP:LISTEN -t)"
    sleep 3
fi

# Start the server.
echo "Starting server..."
./bin/server -port 8080 -dir ./test_output &
SERVER_PID=$!
sleep 3 # Give the server some time to start.

# Test 1: Single file transfer.
echo "Test 1: Single file transfer"
./bin/client -server localhost:8080 -file ./test/file0.txt
echo "Result:"
ls -la ./test_output/
rm -f ./test_output/file0.txt # Remove this particular file for later tests.

# Test 2: Directory transfer.
echo -e "\nTest 2: Directory transfer"
./bin/client -server localhost:8080 -file ./test
echo "Result:"
find ./test_output -type f
rm -f ./test_output/file1.txt # Remove this particular file for later tests.

# Test 3: Different file types.
echo -e "\nTest 3: Different file types"
./bin/client -server localhost:8080 -file ./empty_file.txt
./bin/client -server localhost:8080 -file ./test.json
echo "Result:"
ls -la ./test_output/

# Test 4: Large file.
echo -e "\nTest 4: Large file transfer (500MB)"
time ./bin/client -server localhost:8080 -file ./large_file.dat
echo "Large file transfer completed"

# Test 5: File conflict handling for the default "rename" strategy.
echo -e "\nTest 5: File conflict handling for the default 'rename' strategy"
./bin/client -server localhost:8080 -file ./test/file1.txt
./bin/client -server localhost:8080 -file ./test/file1.txt
echo "Result:"
ls -la ./test_output/

# Test 6: Invalid file (should fail).
echo -e "\nTest 6: Invalid file (should fail)"
./bin/client -server localhost:8080 -file ./nonexistent_file.txt

# Log the final results.
echo -e "\n=== Final Results ==="
echo "All files in test_output:"
find ./test_output -type f
echo -e "\nDirectory structure:"
tree ./test_output 2>/dev/null || find ./test_output -type f

echo -e "\nCleaning up..."
# Kill the server process and wait for it to terminate gracefully.
if kill $SERVER_PID 2>/dev/null; then
    sleep 2
    kill -9 $SERVER_PID 2>/dev/null
fi
# Kill any remaining process on port 8080 if it is still in use.
if lsof -Pi :8080 -sTCP:LISTEN -t >/dev/null 2>&1; then
    kill "$(lsof -Pi :8080 -sTCP:LISTEN -t)" 2>/dev/null
    sleep 1
fi
# Remove the test output directory.
rm -rf ./test_output
# Remove the test directories created in the script.
rm -rf ./test
rm -f ./test.json ./empty_file.txt ./large_file.dat

echo "=== Test completed! ==="
