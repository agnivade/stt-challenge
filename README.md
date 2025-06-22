# Speech-to-Text Challenge

A real-time speech-to-text transcription system with support for multiple providers (Google Cloud Speech-to-Text and Deepgram). The system consists of a WebSocket server that processes audio streams and a client that captures audio from microphone or file input.

## Features

- **Multi-provider support**: Google Cloud Speech-to-Text and Deepgram
- **Real-time transcription**: WebSocket-based streaming audio processing

## Directory Structure

```
stt-challenge/
├── cmd/
│   ├── client/           # Client application
│   │   ├── main.go       # Client entry point
│   │   ├── microphone.go # Microphone audio capture
│   │   └── *_test.go     # Client tests
│   └── server/           # Server application
│       └── main.go       # Server entry point
├── providers/            # Speech provider implementations
│   ├── provider.go       # Provider interfaces
│   ├── google/           # Google Speech-to-Text provider
│   ├── deepgram/         # Deepgram provider
│   └── mocks/            # Generated mocks for testing
├── server.go             # HTTP server and connection management
├── websocket.go          # WebSocket connection handling
├── provider_selector.go  # Multi-provider coordination
├── *_test.go            # Test files
├── Makefile             # Build and run commands
└── README.md            # This file
```

## Prerequisites

- Go 1.24+
- PortAudio library (for microphone support)
- API credentials for at least one provider:
  - **Google Cloud**: `GOOGLE_APPLICATION_CREDENTIALS` environment variable
  - **Deepgram**: `DEEPGRAM_API_KEY` environment variable

### Installing PortAudio

**Ubuntu/Debian:**
```bash
sudo apt-get install portaudio19-dev
```

**macOS:**
```bash
brew install portaudio
```

## Quick Start

### Using Makefile

```bash
# Run server (both providers enabled by default)
make run-server

# Run client (in another terminal)
make run-client

# Run tests
make test

# Generate mocks
make mocks
```

### Manual Commands

```bash
# Start server
go run ./cmd/server

# Start client (in another terminal)
go run ./cmd/client
```

## Usage

### Server

Start the server with different provider configurations:

```bash
# Run with both providers (default)
go run ./cmd/server

# Run with only Google provider
go run ./cmd/server -deepgram=false

# Run with only Deepgram provider
go run ./cmd/server -google=false

# Run on custom port
go run ./cmd/server -port=8080
```

#### Server Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-google` | bool | `true` | Enable Google Speech-to-Text provider |
| `-deepgram` | bool | `true` | Enable Deepgram provider |
| `-port` | string | `"8081"` | Server port |

#### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GOOGLE_APPLICATION_CREDENTIALS` | For Google provider | Path to Google Cloud service account JSON file |
| `DEEPGRAM_API_KEY` | For Deepgram provider | Deepgram API key |

### Client

Connect to the server and start transcribing:

```bash
# Basic usage (microphone input)
go run ./cmd/client

# Specify server URL
go run ./cmd/client -url="ws://localhost:8081/ws"

# Save transcriptions to file
go run ./cmd/client -output="transcript.txt"

# Use audio file as input (for testing)
go run ./cmd/client -input="audio.raw"
```

#### Client Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-url` | string | `ws://localhost:8081/ws` | WebSocket server URL |
| `-output` | string | `""` | Output file path for transcriptions (optional) |
| `-input` | string | `""` | Input audio file path (useful for testing) |
| `-buffer-size` | int | `10` | Number of recent messages to keep for deduplication |
| `-similarity-threshold` | float64 | `0.8` | Similarity threshold for deduplication (0.0-1.0) |

## API Reference

### WebSocket Protocol

**Client → Server (Audio Data):**
```json
{
  "buf": "<base64-encoded-audio-bytes>"
}
```

**Server → Client (Transcription Result):**
```json
{
  "sentence": "transcribed text",
  "confidence": 0.95
}
```

## Development

### Running Tests

```bash
# Run all tests with race detection
make test

# Or manually
go test -v -race ./...
```

### Generating Mocks

```bash
# Generate mocks for testing
make mocks

# Or manually
go run -modfile=go.tools.mod github.com/vektra/mockery/v2
```

## Architecture

For a detailed explanation of the system architecture and data flow, see [ARCHITECTURE.md](ARCHITECTURE.md).

## Contributing

1. Run tests before submitting: `make test`
2. Generate mocks if interfaces change: `make mocks`
3. Follow Go best practices and existing code style
4. Add tests for new functionality

## License

This project is part of a coding challenge and is for demonstration purposes.
