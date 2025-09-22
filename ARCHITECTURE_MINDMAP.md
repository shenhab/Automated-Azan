# 🕌 Automated Azan Application - Architecture Mind Map

## Main Application Overview

```mermaid
mindmap
  root((🕌 Automated Azan))
    🎯 Core Modules
      📅 Scheduling
        athan_scheduler.py
        Prayer Time Calculation
        Scheduled Playback
        Automatic Retry
        Status Tracking
      🎵 Audio Management
        🆕 Modular System
          manager.py
          discovery.py
          connection.py
          playback.py
          circuit_breaker.py
        📜 Legacy
          chromecast_manager.py
      ⏰ Time Sync
        time_sync.py
        NTP Synchronization
        Clock Validation
        Drift Detection
      🕐 Prayer Times
        prayer_times_fetcher.py
        Multiple APIs
        Local Caching
        Location Based
        Auto Updates
    🌐 Web Interface
      🖥️ Frontend
        web_interface.py
        Dashboard
        Settings Pages
        Status Monitor
        Manual Controls
      📡 REST API
        web_interface_api.py
        Device Endpoints
        Playback Control
        Configuration API
        Status Endpoints
      🎨 Templates
        index.html
        settings.html
        chromecasts.html
        test.html
    📊 Configuration
      📋 Config Management
        config_manager.py
        File Configuration
        Runtime Settings
        Environment Variables
      🏭 Chromecast Config
        chromecast_config.py
        Centralized Constants
        Environment Overrides
        Type Safety
      📝 Logging
        logging_setup.py
        Structured Logging
        Log Rotation
        Performance Metrics
      ⚠️ Exception Handling
        chromecast_exceptions.py
        prayer_times_exceptions.py
        Specific Error Types
        Debug Information
    🔧 Development
      🧪 Testing
        test_chromecast_improved.py
        test_basic_functionality.py
        test_prayer_times_fetcher.py
        test_integration.py
        18+ Test Cases
      🐳 Deployment
        Dockerfile
        docker-compose.yml
        portainer-stack.yml
        SystemD Service
      📦 Dependencies
        Pipfile
        uv.lock
        requirements.txt
```

## Detailed Component Architecture

```mermaid
graph TB
    subgraph "🕌 Main Application"
        MAIN[main.py<br/>🚀 Bootstrap]
    end

    subgraph "📅 Scheduling System"
        SCHED[athan_scheduler.py<br/>📅 Prayer Scheduler]
        PRAYER[prayer_times_fetcher.py<br/>🕐 Prayer Times]
        PCONFIG[prayer_times_config.py<br/>⚙️ Prayer Config]
    end

    subgraph "🎵 Audio Management (Improved)"
        MANAGER[manager.py<br/>🎯 Main Manager]
        DISCOVERY[discovery.py<br/>🔍 Device Discovery]
        CONNECTION[connection.py<br/>🔌 Connection Pool]
        PLAYBACK[playback.py<br/>🎵 Media Control]
        CIRCUIT[circuit_breaker.py<br/>⚡ Circuit Breaker]
        LEGACY[chromecast_manager.py<br/>📜 Legacy System]
    end

    subgraph "⏰ Time Management"
        TIMESYNC[time_sync.py<br/>⏰ NTP Sync]
    end

    subgraph "🌐 Web Interface"
        WEB[web_interface.py<br/>🖥️ Frontend]
        API[web_interface_api.py<br/>📡 REST API]
        TEMPLATES[templates/<br/>🎨 HTML Templates]
    end

    subgraph "📊 Configuration & Support"
        CONFIG[config_manager.py<br/>📋 Config Manager]
        CCONFIG[chromecast_config.py<br/>🏭 Chrome Config]
        LOGGING[logging_setup.py<br/>📝 Logging]
        EXCEPTIONS[*_exceptions.py<br/>⚠️ Error Handling]
    end

    subgraph "🧪 Testing & Deployment"
        TESTS[tests/<br/>🧪 Test Suite]
        DOCKER[Docker Files<br/>🐳 Containers]
        SERVICE[azan.service<br/>🔧 SystemD]
    end

    %% Main connections
    MAIN --> SCHED
    MAIN --> MANAGER
    MAIN --> WEB
    MAIN --> CONFIG
    MAIN --> LOGGING

    %% Scheduling connections
    SCHED --> PRAYER
    SCHED --> PCONFIG
    SCHED --> MANAGER
    SCHED --> TIMESYNC

    %% Audio system connections
    MANAGER --> DISCOVERY
    MANAGER --> CONNECTION
    MANAGER --> PLAYBACK
    CONNECTION --> CIRCUIT
    PLAYBACK --> CIRCUIT

    %% Web interface connections
    WEB --> API
    WEB --> TEMPLATES
    API --> MANAGER
    API --> SCHED

    %% Configuration connections
    CONFIG --> CCONFIG
    CONFIG --> LOGGING
    CCONFIG --> MANAGER
    LOGGING --> EXCEPTIONS

    %% Testing connections
    TESTS -.-> MANAGER
    TESTS -.-> SCHED
    TESTS -.-> CONFIG

    %% Data flow styling
    classDef coreModule fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef webModule fill:#f3e5f5,stroke:#4a148c,stroke-width:2px
    classDef configModule fill:#e8f5e8,stroke:#1b5e20,stroke-width:2px
    classDef devModule fill:#fff3e0,stroke:#e65100,stroke-width:2px

    class MAIN,SCHED,MANAGER,TIMESYNC,PRAYER coreModule
    class WEB,API,TEMPLATES webModule
    class CONFIG,CCONFIG,LOGGING,EXCEPTIONS configModule
    class TESTS,DOCKER,SERVICE devModule
```

## Data Flow Architecture

```mermaid
flowchart TD
    subgraph "🌐 External Sources"
        NTP[🕐 NTP Servers]
        APIS[📡 Prayer APIs<br/>NAAS, ICCI]
        USER[👤 User Interface]
    end

    subgraph "📊 Data Processing"
        SYNC[⏰ Time Sync<br/>Validation]
        FETCH[📥 Prayer Fetcher<br/>Caching & Updates]
        CONFIG[⚙️ Configuration<br/>Management]
    end

    subgraph "🎯 Core Logic"
        SCHEDULER[📅 Athan Scheduler<br/>Prayer Timing]
        AUDIO[🎵 Audio Manager<br/>Playback Control]
    end

    subgraph "🔊 Output Devices"
        CHROME[📡 Chromecast<br/>Devices]
        SPEAKERS[🔊 Smart Speakers<br/>Google Nest]
    end

    subgraph "🖥️ Monitoring"
        WEB[🌐 Web Dashboard<br/>Status & Control]
        LOGS[📝 Logs & Metrics<br/>Health Monitoring]
    end

    %% Data flow connections
    NTP --> SYNC
    APIS --> FETCH
    USER --> CONFIG
    USER --> WEB

    SYNC --> SCHEDULER
    FETCH --> SCHEDULER
    CONFIG --> SCHEDULER
    CONFIG --> AUDIO

    SCHEDULER --> AUDIO
    AUDIO --> CHROME
    CHROME --> SPEAKERS

    SCHEDULER --> WEB
    AUDIO --> WEB
    WEB --> LOGS

    %% Feedback loops
    WEB -.-> SCHEDULER
    WEB -.-> AUDIO
    LOGS -.-> CONFIG

    %% Styling
    classDef external fill:#ffebee,stroke:#c62828,stroke-width:2px
    classDef processing fill:#e3f2fd,stroke:#1565c0,stroke-width:2px
    classDef core fill:#e8f5e8,stroke:#2e7d32,stroke-width:2px
    classDef output fill:#fff3e0,stroke:#f57c00,stroke-width:2px
    classDef monitoring fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px

    class NTP,APIS,USER external
    class SYNC,FETCH,CONFIG processing
    class SCHEDULER,AUDIO core
    class CHROME,SPEAKERS output
    class WEB,LOGS monitoring
```

## Improved vs Legacy Architecture

```mermaid
graph LR
    subgraph "📜 Legacy System"
        L1[chromecast_manager.py<br/>66KB Monolith]
        L2[❌ No Type Safety]
        L3[❌ No Circuit Breaker]
        L4[❌ No Connection Pool]
        L5[❌ No Health Checks]
        L6[❌ Generic Exceptions]
    end

    subgraph "🆕 Improved System"
        N1[manager.py<br/>14KB Orchestrator]
        N2[✅ Full Type Safety]
        N3[✅ Circuit Breaker]
        N4[✅ Connection Pooling]
        N5[✅ Health Monitoring]
        N6[✅ Specific Exceptions]

        subgraph "🔧 Modular Components"
            M1[discovery.py<br/>🔍 Device Discovery]
            M2[connection.py<br/>🔌 Connection Pool]
            M3[playback.py<br/>🎵 Media Control]
            M4[circuit_breaker.py<br/>⚡ Failure Protection]
        end
    end

    L1 -.->|"Refactored into"| N1
    N1 --> M1
    N1 --> M2
    N1 --> M3
    N1 --> M4

    %% Improvements
    L2 -.->|"Improved to"| N2
    L3 -.->|"Added"| N3
    L4 -.->|"Added"| N4
    L5 -.->|"Added"| N5
    L6 -.->|"Enhanced to"| N6

    classDef legacy fill:#ffebee,stroke:#d32f2f,stroke-width:2px
    classDef improved fill:#e8f5e8,stroke:#388e3c,stroke-width:2px
    classDef modular fill:#e3f2fd,stroke:#1976d2,stroke-width:2px

    class L1,L2,L3,L4,L5,L6 legacy
    class N1,N2,N3,N4,N5,N6 improved
    class M1,M2,M3,M4 modular
```

## System Statistics & Metrics

```mermaid
pie title 📊 Code Distribution (Lines)
    "Audio Management" : 66610
    "Prayer Times" : 27052
    "Web Interface" : 53566
    "Configuration" : 25206
    "Testing" : 15000
    "Time Sync" : 21050
    "Scheduling" : 17770
```
