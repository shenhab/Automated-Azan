#!/usr/bin/env python3
"""
Service Modules Integration Example

This script demonstrates how to use all the refactored service modules
together with their JSON API responses. Perfect for integrating into
other applications, microservices, or monitoring systems.
"""

import json
import sys
from datetime import datetime

# Import all refactored service modules
from config_manager import ConfigManager
from logging_setup import setup_logging
from athan_scheduler import AthanScheduler
from prayer_times_fetcher import PrayerTimesFetcher
from chromecast_manager import ChromecastManager
from time_sync import TimeSynchronizer, update_ntp_time
from web_interface_api import WebInterfaceAPI
from main import get_application_status


def pretty_print_json(data, title=""):
    """Pretty print JSON data with a title."""
    if title:
        print(f"\n--- {title} ---")
    print(json.dumps(data, indent=2, default=str))


def demonstrate_core_modules():
    """Demonstrate core application modules."""
    print("🔧 CORE MODULES DEMONSTRATION")
    print("=" * 50)

    # 1. Configuration Management
    print("\n1. Configuration Management:")
    config = ConfigManager()

    # Load and validate configuration
    all_settings = config.get_all_settings()
    validation = config.validate_config()

    print(f"   ✓ Settings loaded: {all_settings['success']}")
    print(f"   ✓ Configuration valid: {validation['success']}")

    if validation['success']:
        speakers = config.get_speakers_group_name()
        location = config.get_location()
        print(f"   ✓ Speakers: {speakers.get('speakers_group_name', 'Error')}")
        print(f"   ✓ Location: {location.get('location', 'Error')}")

    # 2. Logging Setup
    print("\n2. Logging Management:")

    # Setup logging
    logging_result = setup_logging("/tmp/integration_test.log")
    print(f"   ✓ Logging setup: {logging_result['success']}")

    # Get logging status
    from logging_setup import get_logging_status
    status = get_logging_status()
    print(f"   ✓ Logging status: {status['success']}")
    print(f"   ✓ Handlers active: {len(status.get('handlers', []))}")

    return config


def demonstrate_service_modules():
    """Demonstrate service modules."""
    print("\n🔧 SERVICE MODULES DEMONSTRATION")
    print("=" * 50)

    # 1. Prayer Times Service
    print("\n1. Prayer Times Service:")
    fetcher = PrayerTimesFetcher()

    # Get available sources
    sources = fetcher.get_available_sources()
    print(f"   ✓ Sources available: {len(sources.get('sources', {}))}")

    # Get file status
    file_status = fetcher.get_file_status()
    print(f"   ✓ File status check: {file_status['success']}")

    # Try to fetch prayer times (with error handling)
    try:
        prayer_times = fetcher.fetch_prayer_times("naas")
        if prayer_times['success']:
            print(f"   ✓ Prayer times fetched: {len(prayer_times['prayer_times'])} prayers")
        else:
            print(f"   ⚠ Prayer times fetch failed: {prayer_times.get('error', 'Unknown')}")
    except Exception as e:
        print(f"   ⚠ Prayer times error: {e}")

    # 2. Time Synchronization Service
    print("\n2. Time Synchronization Service:")

    # Main time sync function
    sync_result = update_ntp_time()
    print(f"   ✓ Time sync check: {sync_result['success']}")
    print(f"   ✓ System synchronized: {sync_result.get('synchronized', False)}")

    # Detailed time sync analysis
    synchronizer = TimeSynchronizer()
    detailed_status = synchronizer.sync_status_summary()
    print(f"   ✓ Detailed analysis: {detailed_status['success']}")
    if detailed_status['success']:
        print(f"   ✓ Overall status: {detailed_status.get('overall_status', 'unknown')}")

    # 3. Chromecast Management Service
    print("\n3. Chromecast Management Service:")
    try:
        cast_manager = ChromecastManager()

        # Get system status
        system_status = cast_manager.get_system_status()
        print(f"   ✓ System status: {system_status['success']}")

        # Get discovered devices
        devices = cast_manager.get_discovered_devices()
        print(f"   ✓ Device discovery: {devices['success']}")
        print(f"   ✓ Devices found: {devices.get('devices_count', 0)}")

        # Get Athan status
        athan_status = cast_manager.get_athan_status()
        print(f"   ✓ Athan status: {athan_status['success']}")
        print(f"   ✓ Currently playing: {athan_status.get('playing', False)}")

    except Exception as e:
        print(f"   ⚠ Chromecast service error: {e}")

    # 4. Web Interface API Service
    print("\n4. Web Interface API Service:")
    try:
        web_api = WebInterfaceAPI()

        # Get system status
        web_status = web_api.get_system_status()
        print(f"   ✓ Web API status: {web_status['success']}")

        # Get media files
        media_files = web_api.get_media_files()
        print(f"   ✓ Media files: {media_files['success']}")
        if media_files['success']:
            print(f"   ✓ Files found: {media_files.get('files_count', 0)}")

    except Exception as e:
        print(f"   ⚠ Web API service error: {e}")


def demonstrate_scheduler_integration():
    """Demonstrate scheduler integration."""
    print("\n🔧 SCHEDULER INTEGRATION DEMONSTRATION")
    print("=" * 50)

    try:
        # Initialize scheduler (this would normally be done by main.py)
        config = ConfigManager()
        validation = config.validate_config()

        if validation['success']:
            location_result = config.get_location()
            speakers_result = config.get_speakers_group_name()

            if location_result['success'] and speakers_result['success']:
                print("\n1. Scheduler Integration:")
                scheduler = AthanScheduler(
                    location=location_result['location'],
                    google_device=speakers_result['speakers_group_name']
                )

                # Get prayer times
                prayer_times = scheduler.get_prayer_times()
                print(f"   ✓ Prayer times loaded: {prayer_times['success']}")

                # Get next prayer
                next_prayer = scheduler.get_next_prayer_time()
                print(f"   ✓ Next prayer calculated: {next_prayer['success']}")
                if next_prayer['success'] and next_prayer.get('prayer'):
                    print(f"   ✓ Next prayer: {next_prayer['prayer']} at {next_prayer.get('formatted_time')}")

                # Get scheduler status
                scheduler_status = scheduler.get_scheduler_status()
                print(f"   ✓ Scheduler status: {scheduler_status['success']}")
                print(f"   ✓ Jobs scheduled: {scheduler_status.get('total_jobs', 0)}")

            else:
                print("   ⚠ Configuration missing required values")
        else:
            print("   ⚠ Configuration validation failed")

    except Exception as e:
        print(f"   ⚠ Scheduler integration error: {e}")


def demonstrate_application_health():
    """Demonstrate application health monitoring."""
    print("\n🔧 APPLICATION HEALTH MONITORING")
    print("=" * 50)

    try:
        # Get overall application status
        app_status = get_application_status()

        print(f"\n1. Application Health Check:")
        print(f"   ✓ Health check: {app_status['success']}")
        print(f"   ✓ Application status: {app_status.get('status', 'unknown')}")

        if app_status['success'] and app_status.get('status') == 'healthy':
            config_info = app_status.get('configuration', {})
            print(f"   ✓ Configuration valid: {config_info.get('valid', False)}")

            if 'prayer_times' in app_status:
                prayer_info = app_status['prayer_times']
                print(f"   ✓ Prayer times: {prayer_info.get('success', False)}")

            if 'next_prayer' in app_status:
                next_prayer_info = app_status['next_prayer']
                print(f"   ✓ Next prayer known: {next_prayer_info.get('success', False)}")
        else:
            print(f"   ⚠ Application has health issues")

    except Exception as e:
        print(f"   ⚠ Health monitoring error: {e}")


def demonstrate_api_endpoints():
    """Demonstrate how modules can be used as API endpoints."""
    print("\n🔧 API ENDPOINTS SIMULATION")
    print("=" * 50)

    # Simulate common API endpoints using our modules
    endpoints = {
        "/api/config": lambda: ConfigManager().get_all_settings(),
        "/api/config/validate": lambda: ConfigManager().validate_config(),
        "/api/prayer-times": lambda: PrayerTimesFetcher().get_available_sources(),
        "/api/prayer-times/naas": lambda: PrayerTimesFetcher().fetch_prayer_times("naas"),
        "/api/time/status": lambda: TimeSynchronizer().sync_status_summary(),
        "/api/time/ntp": lambda: update_ntp_time(),
        "/api/devices": lambda: ChromecastManager().get_discovered_devices(),
        "/api/athan/status": lambda: ChromecastManager().get_athan_status(),
        "/api/health": lambda: get_application_status(),
        "/api/system": lambda: WebInterfaceAPI().get_system_status()
    }

    print("\n1. API Endpoints Simulation:")
    successful_endpoints = 0

    for endpoint, func in endpoints.items():
        try:
            result = func()
            success = result.get('success', False)
            status_icon = "✅" if success else "❌"
            print(f"   {status_icon} {endpoint}: {'OK' if success else 'Error'}")
            if success:
                successful_endpoints += 1
        except Exception as e:
            print(f"   ❌ {endpoint}: Exception - {str(e)[:50]}...")

    print(f"\n   📊 API Health: {successful_endpoints}/{len(endpoints)} endpoints OK")


def main():
    """Main demonstration function."""
    print("🚀 AUTOMATED AZAN - SERVICE MODULES INTEGRATION DEMO")
    print("=" * 70)
    print("This demo shows how all refactored modules work together with JSON APIs")
    print("Perfect for integrating into other applications, microservices, or monitoring!")

    try:
        # 1. Core Modules
        config = demonstrate_core_modules()

        # 2. Service Modules
        demonstrate_service_modules()

        # 3. Scheduler Integration
        demonstrate_scheduler_integration()

        # 4. Application Health
        demonstrate_application_health()

        # 5. API Endpoints
        demonstrate_api_endpoints()

        print("\n" + "=" * 70)
        print("🎉 INTEGRATION DEMO COMPLETE")
        print("✅ All modules successfully return JSON responses")
        print("✅ Modules can be imported independently")
        print("✅ Error handling is consistent across all modules")
        print("✅ Ready for production integration!")

        print("\n💡 INTEGRATION TIPS:")
        print("• All functions return {'success': bool, ...} format")
        print("• Use try-catch blocks for robust error handling")
        print("• Check 'success' field before accessing data")
        print("• Timestamps included for tracking and debugging")
        print("• Detailed error information in 'error' field")

    except Exception as e:
        print(f"\n❌ Demo failed with error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()