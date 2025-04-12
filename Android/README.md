# Automated Azan - Android Application

This Android application replicates the functionality of the Python-based Automated Azan system, providing prayer time notifications and Azan playback on Google Home devices.

## Features

- **Prayer Time Calculation**: Fetches prayer times from ICCI Dublin and Naas sources
- **Google Home Integration**: Plays Azan on Google Home devices at prayer times
- **Fajr-specific Features**: Special Fajr Azan sound and wake-up call 45 minutes before Fajr
- **WhatsApp Notifications**: Optional Twilio-based WhatsApp notifications for prayer times
- **Automatic Scheduling**: Daily scheduling of prayer times with proper handling of month transitions

## Architecture

The application follows modern Android architecture principles:

- **Kotlin-based**: Written entirely in Kotlin
- **MVVM Architecture**: Using ViewModel, Repository pattern
- **Background Processing**: WorkManager for reliable scheduling
- **Cast Framework**: Google Cast SDK for Google Home integration

## Project Structure

```
app/
├── src/
│   ├── main/
│   │   ├── java/com/automatedazan/
│   │   │   ├── broadcast/           # Broadcast receivers
│   │   │   ├── cast/                # Chromecast/Google Home integration
│   │   │   ├── data/
│   │   │   │   ├── api/             # Network services
│   │   │   │   ├── model/           # Data models
│   │   │   │   └── repository/      # Data repositories
│   │   │   ├── service/             # Background services
│   │   │   ├── ui/                  # UI components and activities
│   │   │   ├── worker/              # WorkManager workers for scheduling
│   │   │   └── AutomatedAzanApp.kt  # Application class
│   │   └── res/                     # Android resources
│   └── androidTest/                 # Instrumented tests
└── build.gradle                     # App-level build file
```

## Setup Requirements

1. Android device or emulator running Android 7.0 (API level 24) or higher
2. Google Home device accessible on the same WiFi network
3. For WhatsApp notifications:
   - Twilio account
   - WhatsApp Business API access via Twilio

## How It Works

1. **Prayer Time Fetching**: The app downloads prayer timetables from ICCI or NAAS sources and caches them locally.
2. **Prayer Scheduling**: Every day at 1:00 AM, the app schedules notifications for all upcoming prayer times.
3. **Azan Playback**: At prayer time, the app connects to the configured Google Home device and plays the appropriate Azan.
4. **WhatsApp Notifications**: If enabled, the app sends WhatsApp notifications at prayer times via Twilio.
5. **Special Fajr Handling**: For Fajr, the app can play a wake-up Quran recitation 45 minutes before the prayer time.

## Key Differences from Python Version

- **Mobile-First**: Runs entirely on an Android device instead of a server/desktop
- **User Interface**: Provides a graphical interface for configuration and viewing prayer times
- **Cast Integration**: Uses the official Google Cast SDK instead of pychromecast
- **Scheduling**: Uses Android's WorkManager system instead of Python's schedule library
- **Configuration**: Uses Android's SharedPreferences instead of a config file

## Setup Instructions

1. Install the app on your Android device
2. Open the app and configure:
   - Prayer time location (ICCI or Naas)
   - Google Home device name
   - (Optional) Twilio settings for WhatsApp notifications
3. The app will automatically schedule all prayer times

## Dependencies

- Google Play Services Cast API
- AndroidX Libraries
- Retrofit for network calls
- OkHttp for HTTP client
- WorkManager for background scheduling
- Twilio SDK for WhatsApp integration

## Building the Project

1. Clone the repository
2. Open the project in Android Studio
3. Build and run on your device

## Future Improvements

- Add support for more prayer time sources
- Implement a widget for displaying the next prayer time
- Add customization options for Azan sounds
- Improve Cast device discovery process
- Add multi-language support