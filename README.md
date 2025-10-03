# filexfer

## Overview
This project is a multi-threaded file transfer application written in Go that supports both single-file and directory transfers over TCP connections. The application implements a custom binary protocol with user-friendly features, including SHA256 checksums, progress tracking, and configurable conflict resolution strategies.

The utility operates through a client-server architecture:

1. **Client**: Initiates file/directory transfers with progress tracking and validation.
2. **Server**: Receives and stores files with configurable conflict resolution (overwrite, rename, skip).
3. **Protocol**: Custom binary protocol with headers containing metadata and checksums.
4. **Security**: SHA256 checksums for data integrity verification.
5. **Progress Tracking**: Real-time transfer progress with rate calculation.

## Supported Transfer Types

### Single File Transfers
- **File validation**: Size limits (default 5GB), filename validation, path traversal protection.
- **Checksum verification**: SHA256 checksums for data integrity.
- **Progress tracking**: Real-time progress bars with transfer rates.
- **Error handling**: Comprehensive error reporting and recovery.

### Directory Transfers
- **Recursive scanning**: Complete directory tree traversal.
- **Relative path preservation**: Maintains directory structure.
- **Size validation**: Configurable total directory size limits (default 50GB).
- **Per-client tracking**: Individual client directory transfer size monitoring.
- **File metadata**: Preserves file modes and timestamps.

## Project Structure
The codebase is organized into modular components:
- **cmd/client/**: Client application with transfer initiation and progress tracking.
- **cmd/server/**: Server application with file reception and conflict resolution.
- **protocol/**: Custom binary protocol implementation.
  - **header.go**: Transfer header with metadata and checksums.
  - **checksum.go**: SHA256 checksum calculation and verification.
  - **directory.go**: Directory scanning and metadata handling.
  - **progress.go**: Progress tracking and rate calculation.

## Building and Usage

### Building the Applications
```
# Build both client and server.
go build -o ./bin/client ./cmd/client/main.go
go build -o ./bin/server ./cmd/server/main.go
```

**Note**: For easier build management, users can also use the supported `Makefile`, which includes additional targets like `make build`, `make client`, `make server`, `make clean`, and more. Run `make help` to see all available targets.

### Running the Server
```
./bin/server [-port <port>] [-dir <directory>] [-strategy <strategy>] [-max-dir-size <bytes>]

  -port string
        Listening port (default "8080").
  -dir string
        Destination directory for received files (default "test").
  -strategy string
        File conflict strategy: overwrite, rename, or skip (default "rename").
  -max-dir-size uint64
        Maximum directory transfer size in bytes (default 53687091200 = 50GB).
```

### Running the Client
```
./bin/client [-server <server>] [-file <file>]

  -server string
        Server address (IP:Port) (default "localhost:8080").
  -file string
        File or directory to be transferred (required).
```

## Transfer Protocol

### Header Structure
The binary protocol uses a fixed-size header (329 bytes) containing:
- **File size**: 8 bytes (big-endian).
- **Filename**: 256 bytes (null-padded).
- **SHA256 checksum**: 32 bytes.
- **Transfer type**: 1 byte (0=file, 1=directory).
- **Directory path**: 256 bytes (for directory transfers).

### Transfer Process
1. **Header transmission**: Client sends transfer header with metadata.
2. **Data transfer**: File/directory content with progress tracking.
3. **Verification**: Server validates checksums and file integrity.
4. **Conflict resolution**: Applies configured strategy (overwrite/rename/skip).
5. **Response**: Server sends success/error response to client.

## Features

### Security and Validation
- **Path traversal protection**: Prevents path traversal attacks.
- **Size limits**: Configurable maximum file (5GB) and directory (default 50GB) sizes.
- **Per-client directory limits**: Individual client directory transfer size tracking and validation.
- **Checksum verification**: SHA256 checksums for data integrity.
- **Input validation**: Comprehensive filename and path validation.

### Progress Tracking
- **Real-time progress bars**: Visual progress indicators.
- **Transfer rate calculation**: MB/s rate display.
- **Duration tracking**: Transfer time measurement.
- **Size formatting**: User-readable file sizes (KB/MB).

### Conflict Resolution
- **Overwrite**: Replace existing files.
- **Rename**: Append numeric suffix to avoid conflicts.
- **Skip**: Skip files that already exist.

### Error Handling
- **Graceful shutdown**: Context-based cancellation support.
- **Connection timeouts**: Configurable read/write timeouts.
- **Comprehensive logging**: Structured logging with timestamps.
- **Error recovery**: Detailed error messages and recovery.

## Testing

### Running the Test Suite
```
# Execute the comprehensive test script.
chmod +x ./test.sh
./test.sh
```

The test script performs:
- Single file transfers.
- Directory transfers with nested structures.
- Large file transfers (500MB+).
- Conflict resolution testing.
- Error condition testing.
- Different file types and edge cases.

```
# Execute the large directory test script.
chmod +x ./test_large_directory.sh
./test_large_directory.sh
```

The large directory test script creates a large directory structure with various file sizes and types to test performance and reliability under load.

```
# Execute the directory size limit test script.
chmod +x ./test_directory_limit.sh
./test_directory_limit.sh
```

The directory size limit test script specifically tests the 50GB directory size limit functionality:
- **Test 1**: Directory transfer at/below 50GB limit (should succeed)
- **Test 2**: Directory transfer exceeding 50GB limit (should fail)

This script validates that the `-max-dir-size` parameter properly enforces directory size limits by testing both the "allow" and "reject" behaviors at the boundary.

### Test Coverage
- **File transfers**: Various file sizes and types.
- **Directory transfers**: Nested directory structures.
- **Conflict handling**: Multiple transfer strategies.
- **Error conditions**: Invalid files, network issues.
- **Performance**: Large file transfer timing.
- **Size limits**: Directory size limit enforcement (50GB boundary testing).

## Development and Extensibility

### Debugging and Monitoring
- **Structured logging**: Timestamped logs with component prefixes.
- **Progress tracking**: Real-time transfer monitoring.
- **Error reporting**: Fine-grained error reporting with context.
- **Connection monitoring**: Connection duration and status tracking.

### Adding New Features
- **Protocol extensions**: Extend header structure in `protocol/header.go`.
- **Transfer types**: Add new transfer types in protocol constants in `protocol/header.go`.
- **Conflict strategies**: Implement new strategies in server logic in `cmd/server/main.go`.
- **Progress formats**: Customize progress display in `protocol/progress.go`.
