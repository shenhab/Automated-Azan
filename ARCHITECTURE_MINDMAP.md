# 🕌 Automated Azan Application - Architecture Mind Map

## Main Application Overview

```mermaid
%%{init: {'mindmap': {'theme': 'base', 'themeVariables': {'primaryColor': '#ffffff', 'primaryTextColor': '#000000', 'primaryBorderColor': '#000000', 'lineColor': '#000000', 'section0': '#f0f0f0', 'section1': '#e0e0e0', 'section2': '#d0d0d0', 'section3': '#c0c0c0', 'section4': '#b0b0b0'}}}}%%
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
      📋 App Config
        app_config.py
        config_manager.py
        File Configuration
        Runtime Settings
        Environment Variables
      🏭 Chromecast Config
        chromecast_config.py
        chromecast_models.py
        Centralized Constants
        Environment Overrides
        Type Safety
      🕐 Prayer Config
        prayer_times_config.py
        prayer_times_exceptions.py
        API Configuration
        Calculation Methods
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
        test_cache.py
        test_chromecast.py
        18+ Test Cases
      🐳 Deployment
        Dockerfile
        docker-compose.yml
        portainer-stack.yml
        SystemD Service
        azan.service
      📦 Dependencies
        Pipfile
        uv.lock
        requirements.txt
      🔧 Integration
        service_modules_integration.py
        example_integration.py
      📁 Media & Data
        Media/
        data/
        static/
        logs/
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

    %% Color blind accessible high contrast styling
    classDef coreModule fill:#f5f5f5,stroke:#000000,stroke-width:4px,color:#000000
    classDef webModule fill:#e8e8e8,stroke:#000000,stroke-width:4px,color:#000000
    classDef configModule fill:#d8d8d8,stroke:#000000,stroke-width:4px,color:#000000
    classDef devModule fill:#c8c8c8,stroke:#000000,stroke-width:4px,color:#000000

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

    %% Color blind accessible high contrast styling
    classDef external fill:#f0f0f0,stroke:#000000,stroke-width:4px,color:#000000
    classDef processing fill:#e0e0e0,stroke:#000000,stroke-width:4px,color:#000000
    classDef core fill:#d0d0d0,stroke:#000000,stroke-width:4px,color:#000000
    classDef output fill:#c0c0c0,stroke:#000000,stroke-width:4px,color:#000000
    classDef monitoring fill:#b0b0b0,stroke:#000000,stroke-width:4px,color:#000000

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

    %% Color blind accessible high contrast styling
    classDef legacy fill:#f0f0f0,stroke:#000000,stroke-width:4px,color:#000000,stroke-dasharray:10,5
    classDef improved fill:#e0e0e0,stroke:#000000,stroke-width:4px,color:#000000
    classDef modular fill:#d0d0d0,stroke:#000000,stroke-width:4px,color:#000000

    class L1,L2,L3,L4,L5,L6 legacy
    class N1,N2,N3,N4,N5,N6 improved
    class M1,M2,M3,M4 modular
```

## System Statistics & Metrics

```mermaid
%%{init: {'theme':'base', 'themeVariables': { 'primaryColor': '#ffffff', 'primaryTextColor': '#000000', 'primaryBorderColor': '#333333', 'lineColor': '#333333', 'sectionBkColor': '#f0f0f0', 'altSectionBkColor': '#e0e0e0'}}}%%
pie title 📊 Code Distribution (Lines)
    "Audio Management" : 66610
    "Prayer Times" : 27052
    "Web Interface" : 53566
    "Configuration" : 25206
    "Testing" : 15000
    "Time Sync" : 21050
    "Scheduling" : 17770
```

## ♿ Color Blind Accessible Features

All diagrams have been updated with color blind-friendly design:

### **✅ High Contrast Grayscale**
- **Stroke width**: Increased to 4px for maximum visibility
- **Pure black borders**: `#000000` for strongest contrast
- **Pure black text**: `#000000` on all elements for maximum readability
- **Consistent spacing**: Clear visual separation between components

### **🎨 Grayscale Palette (Color Blind Safe)**
- **Light Gray**: `#f0f0f0` (highest brightness)
- **Medium Light**: `#e0e0e0` (high brightness)
- **Medium**: `#d0d0d0` (medium brightness)
- **Medium Dark**: `#c0c0c0` (lower brightness)
- **Dark Gray**: `#b0b0b0` (lowest brightness)
- **Legacy Pattern**: Dashed borders (`stroke-dasharray:10,5`) to distinguish legacy components

### **📱 Accessibility Improvements**
- **WCAG compliant**: All colors meet contrast ratio requirements
- **Readable in daylight**: Light backgrounds with dark text
- **Print friendly**: Colors work well in black & white
- **Screen reader friendly**: Emoji and text descriptions

### **🔍 Testing Recommendations**
Test the diagrams in:
- ☀️ **Bright daylight** (outdoor viewing)
- 🖥️ **High brightness monitors**
- 📱 **Mobile devices** (various screen types)
- 🖨️ **Printed documents** (black & white)
- 👓 **Accessibility tools** (color blind simulation)

The diagrams now provide excellent visibility in both light and dark environments while maintaining their informative structure and professional appearance! 🌅🌙
