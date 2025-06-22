# Architecture Overview

## System Overview

The system follows a client-server architecture with multi-provider support for speech-to-text transcription. The main components are:

1. **Client** - Captures audio and displays transcriptions
2. **WebSocket Server** - Manages connections and coordinates providers
3. **Provider Selector** - Distributes audio and selects best transcription
4. **STT Providers** - Interface with external speech services (Google, Deepgram)

## Architecture Diagram

```
┌─────────────────┐    WebSocket     ┌─────────────────────────────────┐
│     Client      │◄────────────────►│         Server                  │
│                 │     JSON         │                                 │
│ - Audio Capture │     Messages     │ ┌─────────────────────────────┐ │
│ - Display Text  │                  │ │        WebConn              │ │
└─────────────────┘                  │ │                             │ │
                                     │ └─────────────┬───────────────┘ │
                                     └───────────────┼─────────────────┘
                                                     │
                                                     ▼
                                     ┌─────────────────────────────────┐
                                     │      ProviderSelector           │
                                     │                                 │
                                     │ ┌─────────────────────────────┐ │
                                     │ │    AudioDistributor         │ │
                                     │ └─────────────────────────────┘ │
                                     │ ┌─────────────────────────────┐ │
                                     │ │  TranscriptionCollector     │ │
                                     │ └─────────────────────────────┘ │
                                     └─────────────┬───────────────────┘
                                                   │
                                      ┌────────────┼────────────┐
                                      ▼            ▼            ▼
                               ┌──────────────┐ ┌────────────┐ ┌──────────────┐
                               │   Google     │ │  Deepgram  │ │   Future     │
                               │  Provider    │ │  Provider  │ │  Provider    │
                               │              │ │            │ │              │
                               └──────────────┘ └────────────┘ └──────────────┘
```

## Data Flow

### 1. Audio Input → Provider Distribution
- Client captures audio and sends via WebSocket to Server
- WebConn receives audio data and forwards to ProviderSelector
- ProviderSelector's AudioDistributor sends audio to all active providers in parallel

### 2. Provider Processing → Result Collection
- Each provider processes audio independently (Google via gRPC, Deepgram via WebSocket)
- Providers send transcription results back to ProviderSelector
- TranscriptionCollector implements selection logic to choose best result

### 3. Response Delivery
- Selected transcription is sent back through WebConn to Client
- Client displays the transcription result

## Provider Selector Logic

The ProviderSelector implements a **heuristic-based selection** strategy:

- **Audio Distribution**: Distributes each audio chunk to all providers simultaneously
- **Result Collection**: Collects transcription results from all providers
- **Active Provider Selection**: Dynamically switches to the provider with the most recent activity (lowest latency)
- **Missed Message Recovery**: When switching providers, sends any missed transcriptions from the new active provider

This approach optimizes for low latency while maintaining reliability through provider redundancy.