# filexfer

## Overview

`filexfer` is a multi-threaded file transfer application written in Go that supports both single-file and directory transfers over TCP/TLS connections. The application implements a custom binary protocol with features such as TLS encryption, SHA-256 checksums, progress tracking, configurable conflict-resolution strategies, and persistent connections for efficient directory transfers.

The utility operates through a client-server architecture:

1. **Client**: Initiates file/directory transfers with progress tracking and validation. Uses persistent connections for directory transfers to minimize latency.
2. **Server**: Receives and stores files with configurable conflict resolution (overwrite, rename, skip). Handles multiple file transfers on a single connection for directory transfers.
3. **Protocol**: Custom binary protocol with length-prefixed headers containing metadata and checksums. Supports long paths (up to 64KB) without fixed-size restrictions.
4. **Progress Tracking**: Real-time transfer progress with rate calculation.
5. **Security**: TLS encryption for end-to-end security, SHA-256 checksums for data integrity verification.

### Validation and Safety (code-level)

- Client: path validation with size limits (5GB default), empty/missing path checks, non-existent file handling, and explicit error surfacing for server responses.
- Server: header validation (message type, transfer type, filename length/nulls, checksum size), per-client directory size tracking (50GB default, configurable via `-max-dir-size`), file size cap (5GB), and path sanitization to prevent traversal.
- Protocol: length-prefixed headers and responses with max lengths (64KB names/paths/messages) to bound allocations and guard against malformed inputs.

### Conflict Resolution (server)

- Overwrite: remove and replace existing files.
- Rename: append numeric suffix to avoid conflicts (default).
- Skip: leave existing files untouched.

## Supported Transfer Types

### Single File Transfers

- **File validation**: Size limits (default 5GB), filename validation, path traversal protection.
- **Checksum verification**: SHA-256 checksums for data integrity.
- **Progress tracking**: Real-time progress bars with transfer rates.
- **Error handling**: Comprehensive error reporting and recovery.

### Directory Transfers

- **Recursive scanning**: Complete directory tree traversal.
- **Relative path preservation**: Maintains directory structure.
- **Size validation**: Configurable total directory size limits (default 50GB).
- **Per-client tracking**: Individual client directory transfer size monitoring.
- **File metadata**: Preserves file modes and timestamps.
- **Persistent connections**: Single TCP connection reused for all files in a directory transfer, eliminating connection overhead and reducing latency for large directory transfers.

## Project Structure

The codebase is organized into modular components:

- **cmd/client/**: Client application with transfer initiation and progress tracking.
- **cmd/server/**: Server application with file reception and conflict resolution.
- **protocol/**: Custom binary protocol implementation.
  - **header.go**: Transfer header with metadata and checksums.
  - **checksum.go**: SHA-256 checksum calculation and verification.
  - **directory.go**: Directory scanning and metadata handling.
  - **progress.go**: Progress tracking and rate calculation.

## Building and Usage

### Building the Applications

```bash
# Build both client and server.
make build

# Build only the client.
make client

# Build only the server.
make server

# Format, vet, and build everything.
make all

# View all available targets.
make help
```

### Running the Server

```bash
# Run server with default settings (plain TCP without encryption).
make run-server

# Run server with TLS encryption.
make run-server ARGS="-tls-cert server.crt -tls-key server.key -port 8080"

# Run server with custom arguments (example).
make run-server ARGS="-port 9090 -dir /path/to/dest -strategy overwrite"

# Run server with TLS and custom settings.
make run-server ARGS="-tls-cert server.crt -tls-key server.key -port 8443 -dir /secure/dest"
```

**Server Options:**

- `-port string`: Listening port (default "8080").
- `-dir string`: Destination directory for received files (default "test").
- `-strategy string`: File conflict-resolution strategy: overwrite, rename, or skip (default "rename").
- `-max-dir-size uint64`: Maximum directory transfer size in bytes (default 53687091200 = 50GB).
- `-tls-cert string`: Path to TLS certificate file (optional, enables TLS encryption when provided).
- `-tls-key string`: Path to TLS private key file (optional, required if `-tls-cert` is provided).

### Running the Client

```bash
# Run client with plain TCP connection (no encryption).
make run-client ARGS="-server localhost:8080 -file path/to/file"

# Run client with TLS encryption using CA certificate.
make run-client ARGS="-server localhost:8080 -file path/to/file -tls-ca ca.crt"

# Run client with TLS encryption (skip verification for testing only).
make run-client ARGS="-server localhost:8080 -file path/to/file -tls-skip-verify"
```

**Client Options:**

- `-server string`: Server address (IP:Port) (default "localhost:8080").
- `-file string`: File or directory to be transferred (required).
- `-tls-ca string`: Path to CA certificate file for TLS verification (optional, enables TLS when provided).
- `-tls-skip-verify`: Skip TLS certificate verification (insecure, for testing only).

### Auxiliary Makefile Targets

Run `make help` to see all available Makefile targets.

## TLS Encryption

The application supports optional TLS encryption to secure file transfers.
When TLS is enabled, all metadata (filenames) and file contents are encrypted in transit, protecting against interception and man-in-the-middle attacks.

### TLS Configuration

**Server-side:**

- Provide a TLS certificate and private key using `-tls-cert` and `-tls-key` flags to enable TLS encryption.
- The server will automatically use TLS when certificates are provided; otherwise, it falls back to plain TCP (with a warning).

**Client-side:**

- Use `-tls-ca` to provide a CA certificate for proper certificate verification.
- Use `-tls-skip-verify` to skip certificate verification (insecure, only for testing).

### Generating Certificates for Testing

For development and testing, we can generate self-signed certificates:

```bash
# Generate a self-signed certificate and key for the server.
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes -subj "/CN=localhost"

# Generate a CA certificate for client verification (optional).
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 365 -key ca.key -out ca.crt -subj "/CN=FileTransferCA"
```

**Note**: Self-signed certificates are suitable for testing only. Obtain certificates from a trusted Certificate Authority (CA) for production use.

### TLS Security Features

- **TLS 1.2+ required**: Only modern, secure TLS versions are supported.
- **Certificate verification**: Clients can verify server certificates using CA certificates.
- **Backward compatible**: Works without TLS if certificates aren't provided (with security warnings).

## Transfer Protocol

### Header Structure

The binary protocol uses a length-prefixed format for efficient bandwidth usage and support for long paths:

- **File size**: 8 bytes (uint64, big-endian).
- **Filename length**: 4 bytes (uint32, big-endian) - length prefix.
- **Filename**: Variable bytes (up to 64KB) - actual filename data.
- **SHA-256 checksum**: 32 bytes (fixed size).
- **Transfer type**: 1 byte (0=file, 1=directory).
- **Directory path length**: 4 bytes (uint32, big-endian) - length prefix.
- **Directory path**: Variable bytes (up to 64KB) - actual path data.

**Benefits of length-prefixed format:**

- **Bandwidth efficient**: Only sends actual data, no padding or wasted bytes.
- **Supports long paths**: Handles filenames and paths up to 64KB, suitable for environments with deep directory structures.
- **Flexible**: Accommodates short names (1 byte) to very long paths (64KB) without restrictions.

**Note**: The protocol uses a length-prefixed format (not fixed-size). Clients and servers must use compatible protocol versions. The current version supports variable-length filenames and paths, with a maximum limit of 64KB each.

### Transfer Process

**Single File Transfer:**

1. **Connection**: Client establishes a TCP/TLS connection to the server.
2. **Header transmission**: Client sends transfer header with metadata.
3. **Data transfer**: File content with progress tracking.
4. **Streaming architecture**: Server streams data directly to disk while calculating checksums on-the-fly (memory-efficient, no full-file buffering).
5. **Verification**: Server validates checksums and file integrity after transfer completes.
6. **Conflict resolution**: Applies configured strategy (overwrite/rename/skip).
7. **Response**: Server sends success/error response to client.
8. **Connection close**: Connection is closed after the transfer.

**Directory Transfer (Persistent Connection):**

1. **Connection**: Client establishes a single TCP/TLS connection to the server.
2. **Size validation**: Client sends directory size validation request (optional, separate connection).
3. **File loop**: For each file in the directory:
   - **Header transmission**: Client sends transfer header with metadata.
   - **Data transfer**: File content with progress tracking.
   - **Streaming architecture**: Server streams data directly to disk while calculating checksums on-the-fly.
   - **Verification**: Server validates checksums and file integrity.
   - **Conflict resolution**: Applies configured strategy (overwrite/rename/skip).
   - **Response**: Server sends success/error response to client.
   - **Continue**: Process repeats for the next file on the same connection.
4. **Connection close**: Client closes the connection after all files are transferred (server detects `io.EOF`).

## Features

### Security and Validation

- **TLS encryption**: Optional end-to-end encryption using TLS 1.2+ to protect metadata (filenames) and file contents from interception. Supports custom CA certificates for verifying the server's certificate.
- **Path traversal protection**: Prevents path traversal attacks.
- **Size limits**: Configurable maximum file (5GB) and directory (default 50GB) sizes.
- **Per-client directory limits**: Individual client directory transfer size tracking and validation.
- **Checksum verification**: SHA-256 checksums calculated during transfer and verified after completion; corrupted files are automatically deleted.
- **Input validation**: Comprehensive filename and path validation.
- **Protocol limits**: Maximum filename and directory path lengths (64KB each) to prevent abuse while supporting long paths.

### Progress Tracking

- **Real-time progress bars**: Visual progress indicators.
- **Transfer rate calculation**: MB/s rate display.
- **Duration tracking**: Transfer time measurement.
- **Size formatting**: User-readable file sizes (KB/MB).

### Conflict Resolution

- **Overwrite**: Replace existing files.
- **Rename**: Append numeric suffix to avoid conflicts.
- **Skip**: Skip files that already exist.

### Performance and Scalability

- **Memory-efficient streaming**: Files are streamed directly to disk without loading entire files into RAM, enabling efficient handling of large files (up to 5GB) and multiple concurrent transfers.
- **Optimized buffer size**: Uses 1MB buffers for `io.CopyBuffer` operations (v.s. 32KB by default), reducing system calls by ~97% and effectively improving throughput on high-bandwidth networks (where the total number of system calls = 2 * ceil(`header.FileSize`/`TransferBufferSize`)).
- **On-the-fly checksum calculation**: SHA-256 checksums are calculated during transfer using `io.TeeReader`, eliminating the need for double-pass file reading.
- **Persistent connections**: Directory transfers reuse a single TCP connection for all files, eliminating connection setup overhead and reducing latency for large directory transfers (e.g., 10,000 files = 1 connection instead of 10,000).
- **Concurrent transfers**: Server handles multiple client connections simultaneously using goroutines, with per-client resource tracking.
- **Scalable architecture**: Designed to handle large files, deep directory structures, and high concurrency without memory exhaustion or connection resource issues.
- **Efficient protocol**: Length-prefixed format minimizes bandwidth usage and supports long path lengths without artificial restrictions.

### Error Handling

- **Graceful shutdown**: Context-based cancellation support.
- **Connection timeouts**: Configurable read/write timeouts.
- **Comprehensive logging**: Structured logging with timestamps.
- **Error recovery**: Detailed error messages and recovery.
- **Corrupted file cleanup**: Automatically deletes files with checksum mismatches to prevent disk space waste.

## Testing

### Running the Test Suite

#### Unit tests (protocols and core logic)

```bash
# Run all the unit tests in the codebase.
make test
```

This command executes unit tests for:

- Protocol components: header encoding/decoding, checksum calculation, progress tracking.
- Client logic: path validation, file reading/writing, error handling.
- Server logic: file reception, conflict resolution, error handling.

#### Integration tests (end-to-end)

```bash
# Run the basic test script.
make test-sh
```

The basic test script performs:

- Single file transfers.
- Directory transfers with nested structures.
- Large file transfers (500MB+).
- Conflict resolution testing.
- Error condition testing.
- Different file types and edge cases.

```bash
# Run the large directory test script.
make test-large-directory-sh
```

The large directory test script creates a large directory structure with various file sizes and types to test performance and reliability under load.

```bash
# Run the directory size limit test script.
make test-directory-limit-sh
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
- **Client validation**: Path validation (empty/missing/non-existent), symlink handling, size enforcement (5GB default; testable override), context-aware writes, server response parsing.

## Development and Extensibility

### Debugging and Monitoring

- **Structured logging**: Timestamped logs with component prefixes.
- **Progress tracking**: Real-time transfer monitoring.
- **Error reporting**: Fine-grained error reporting with context.
- **Connection monitoring**: Connection duration and status tracking.

### Adding New Features

- **Protocol extensions**: Extend header structure in `protocol/header.go`.
- **Transfer types**: Add new transfer types in protocol constants in `protocol/header.go`.
- **Conflict-resolution strategies**: Implement new strategies in server logic in `cmd/server/main.go`.
- **Progress formats**: Customize progress display in `protocol/progress.go`.
