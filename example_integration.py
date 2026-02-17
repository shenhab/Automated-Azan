#!/usr/bin/env python3
"""
Example Integration Script

This script demonstrates how other applications can import and use
the Automated Azan modules with their JSON API responses.
"""

import json
import sys
from datetime import datetime

# Import the modular components
from config_manager import ConfigManager
from logging_setup import setup_logging, get_logging_status
from athan_scheduler import AthanScheduler
from main import get_application_status


def pretty_print_json(data, title=""):
    """Pretty print JSON data with a title."""
    if title:
        print(f"\n--- {title} ---")
    print(json.dumps(data, indent=2, default=str))


def example_configuration_management():
    """Example of using ConfigManager in another application."""
    print("🔧 Configuration Management Example")

    config = ConfigManager()

    # Get all settings
    all_settings = config.get_all_settings()
    pretty_print_json(all_settings, "All Configuration Settings")

    # Get specific settings
    speakers = config.get_speakers_group_name()
    location = config.get_location()
    prayer_source = config.get_prayer_source()

    print(f"\n✓ Speakers: {speakers.get('speakers_group_name') if speakers['success'] else 'Error'}")
    print(f"✓ Location: {location.get('location') if location['success'] else 'Error'}")
    print(f"✓ Prayer Source: {prayer_source.get('prayer_source')}")

    # Validate configuration
    validation = config.validate_config()
    print(f"✓ Configuration Valid: {validation['success']}")

    return config


def example_logging_management():
    """Example of using logging_setup in another application."""
    print("\n📝 Logging Management Example")

    # Setup logging
    setup_result = setup_logging("/tmp/external_app.log")
    pretty_print_json(setup_result, "Logging Setup Result")

    # Get logging status
    status = get_logging_status()
    pretty_print_json(status, "Current Logging Status")

    return setup_result['success']


def example_prayer_times_integration():
    """Example of using AthanScheduler for prayer times in another application."""
    print("\n🕌 Prayer Times Integration Example")

    # Initialize scheduler
    scheduler = AthanScheduler(location="naas", google_device="athan")

    # Get prayer times
    prayer_times = scheduler.get_prayer_times()
    pretty_print_json(prayer_times, "Prayer Times")

    # Get next prayer
    next_prayer = scheduler.get_next_prayer_time()
    pretty_print_json(next_prayer, "Next Prayer")

    # Get scheduler status
    scheduler_status = scheduler.get_scheduler_status()
    pretty_print_json(scheduler_status, "Scheduler Status")

    # Schedule prayers (for demonstration)
    schedule_result = scheduler.schedule_prayers()
    print(f"\n✓ Scheduled {schedule_result.get('scheduled_count', 0)} prayers")

    return scheduler


def example_application_health_check():
    """Example of checking application health from another app."""
    print("\n🏥 Application Health Check Example")

    status = get_application_status()
    pretty_print_json(status, "Application Status")

    # Simple health check logic
    if status['success'] and status.get('status') == 'healthy':
        print("✅ Application is healthy and ready")
        return True
    else:
        print("❌ Application has issues")
        return False


def example_api_like_usage():
    """Example of API-like usage that other applications might implement."""
    print("\n🔌 API-like Usage Example")

    # Simulate API endpoints
    api_responses = {}

    # GET /config
    config = ConfigManager()
    api_responses['config'] = config.get_all_settings()

    # GET /config/validate
    api_responses['config_validation'] = config.validate_config()

    # GET /prayer-times
    scheduler = AthanScheduler(location="naas")
    api_responses['prayer_times'] = scheduler.get_prayer_times()

    # GET /next-prayer
    api_responses['next_prayer'] = scheduler.get_next_prayer_time()

    # GET /scheduler/status
    api_responses['scheduler_status'] = scheduler.get_scheduler_status()

    # GET /health
    api_responses['health'] = get_application_status()

    print("API Endpoints Simulation:")
    for endpoint, response in api_responses.items():
        success = response.get('success', False)
        status_icon = "✅" if success else "❌"
        print(f"  {status_icon} /{endpoint.replace('_', '-')}: {success}")

    return api_responses


def main():
    """Main demonstration function."""
    print("🚀 Automated Azan - Module Integration Examples")
    print("=" * 60)

    try:
        # 1. Configuration Management
        config = example_configuration_management()

        # 2. Logging Management
        logging_ok = example_logging_management()

        # 3. Prayer Times Integration
        scheduler = example_prayer_times_integration()

        # 4. Application Health Check
        health_ok = example_application_health_check()

        # 5. API-like Usage
        api_responses = example_api_like_usage()

        print("\n" + "=" * 60)
        print("📊 Integration Summary:")
        print(f"✓ Configuration: Working")
        print(f"✓ Logging: {'Working' if logging_ok else 'Issues'}")
        print(f"✓ Prayer Times: Working")
        print(f"✓ Health Check: {'Healthy' if health_ok else 'Issues'}")
        print(f"✓ API Simulation: {len([r for r in api_responses.values() if r.get('success')])} / {len(api_responses)} endpoints OK")

        print("\n💡 Integration Tips:")
        print("- All functions return JSON with 'success' boolean")
        print("- Timestamps are included for tracking")
        print("- Detailed error information available in 'error' field")
        print("- Use try-catch blocks for robust error handling")
        print("- Check 'success' field before accessing data")

    except Exception as e:
        print(f"\n❌ Integration example failed: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()