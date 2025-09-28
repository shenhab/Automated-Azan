#!/usr/bin/env python3
"""
Automated Azan - Main Entry Point

This module serves as the main entry point for the Automated Azan application.
It initializes the configuration, logging, and starts the Athan scheduler.
All modules now return JSON responses for better integration and error handling.
"""

import logging
import threading
import sys
import os
import json

from config_manager import ConfigManager
from logging_setup import setup_logging
from athan_scheduler import AthanScheduler
from web_interface import start_web_interface


def print_json_status(result, operation_name):
    """
    Print status information from JSON responses with improved formatting.

    Args:
        result (dict): JSON response from a module function
        operation_name (str): Name of the operation for logging
    """
    if result.get('success', False):
        logging.info(f"{operation_name}: {result.get('message', 'Success')}")
        print(f"✓ {operation_name}: Success")
    else:
        error_msg = result.get('error', 'Unknown error')
        logging.error(f"{operation_name}: {error_msg}")
        print(f"✗ {operation_name}: {error_msg}")


def main():
    """
    Main entry point for the Automated Azan application.
    """
    print("🕌 Starting Automated Azan application...")

    # Initialize logging first
    logging_result = setup_logging()
    print_json_status(logging_result, "Logging setup")

    if not logging_result.get('success', False):
        print("Failed to setup logging. Exiting.")
        sys.exit(1)

    logging.info("Starting Automated Azan application...")

    try:
        # Initialize configuration manager
        config_manager = ConfigManager()

        # Validate configuration
        validation_result = config_manager.validate_config()
        print_json_status(validation_result, "Configuration validation")

        if not validation_result.get('success', False):
            logging.error("Configuration validation failed")
            sys.exit(1)

        # Get configuration values using JSON responses
        speakers_result = config_manager.get_speakers_group_name()
        location_result = config_manager.get_location()

        if not speakers_result.get('success', False):
            logging.error(f"Failed to get speakers group name: {speakers_result.get('error')}")
            sys.exit(1)

        if not location_result.get('success', False):
            logging.error(f"Failed to get location: {location_result.get('error')}")
            sys.exit(1)

        group_name = speakers_result['speakers_group_name']
        location = location_result['location']

        logging.info(f"Loaded configuration - speakers-group-name: {group_name}, location: {location}")
        print(f"✓ Configuration loaded - speakers: {group_name}, location: {location}")

        # Initialize Athan scheduler
        print("🔄 Initializing Athan scheduler...")
        scheduler = AthanScheduler(location=location, google_device=group_name)

        # Get initial prayer times status
        prayer_times_result = scheduler.get_prayer_times()
        print_json_status(prayer_times_result, "Prayer times loading")

        if prayer_times_result.get('success', False):
            prayer_times = prayer_times_result['prayer_times']
            logging.info(f"Prayer times for {location}: {prayer_times}")
            print(f"📅 Prayer times for {location}:")
            for prayer, time in prayer_times.items():
                print(f"   {prayer}: {time}")

        logging.info("AthanScheduler initialized successfully.")
        print("✓ AthanScheduler initialized successfully")

        # Start web interface in background thread with shared chromecast manager and scheduler
        print("🌐 Starting web interface...")
        web_thread = threading.Thread(
            target=start_web_interface,
            args=(scheduler.chromecast_manager, scheduler),
            daemon=True
        )
        web_thread.start()
        logging.info("Web interface started in background thread")
        print("✓ Web interface started on http://localhost:5000")

        # Get next prayer information before starting scheduler
        next_prayer_result = scheduler.get_next_prayer_time()
        if next_prayer_result.get('success', False) and next_prayer_result.get('prayer'):
            next_prayer = next_prayer_result['prayer']
            next_time = next_prayer_result['formatted_time']
            logging.info(f"Next prayer: {next_prayer} at {next_time}")
            print(f"🔔 Next prayer: {next_prayer} at {next_time}")
        else:
            logging.info("No remaining prayers for today")
            print("ℹ️  No remaining prayers for today")

        print("🚀 Starting main scheduler...")
        print("📊 Monitor status at: http://localhost:5000")
        print("🔍 Device management: http://localhost:5000/chromecasts")
        print("🧪 Audio testing: http://localhost:5000/test")

        # Start the main scheduler (this blocks)
        scheduler.run_scheduler()
        logging.info("AthanScheduler started successfully.")

    except Exception as e:
        logging.error(f"An error occurred while starting the application: {e}", exc_info=True)
        print(f"✗ Application startup failed: {e}")
        sys.exit(1)


def get_application_status():
    """
    Get the current application status as JSON.
    This function can be called by other applications importing this module.

    Returns:
        dict: JSON response with application status
    """
    try:
        # Initialize components to check status
        config_manager = ConfigManager()

        # Get configuration status
        config_info = config_manager.get_config_info()
        validation_result = config_manager.validate_config()

        # Check if we can create scheduler (basic health check)
        try:
            if validation_result.get('success', False):
                speakers_result = config_manager.get_speakers_group_name()
                location_result = config_manager.get_location()

                if speakers_result.get('success') and location_result.get('success'):
                    # Try to create scheduler instance
                    scheduler = AthanScheduler(
                        location=location_result['location'],
                        google_device=speakers_result['speakers_group_name']
                    )
                    prayer_times = scheduler.get_prayer_times()
                    next_prayer = scheduler.get_next_prayer_time()
                    scheduler_status = scheduler.get_scheduler_status()

                    return {
                        "success": True,
                        "application": "Automated Azan",
                        "status": "healthy",
                        "configuration": {
                            "valid": True,
                            "speakers_group": speakers_result['speakers_group_name'],
                            "location": location_result['location']
                        },
                        "prayer_times": prayer_times,
                        "next_prayer": next_prayer,
                        "scheduler": scheduler_status,
                        "config_file_info": config_info,
                        "timestamp": prayer_times.get('timestamp')
                    }
                else:
                    return {
                        "success": False,
                        "application": "Automated Azan",
                        "status": "configuration_error",
                        "error": "Failed to load required configuration",
                        "speakers_result": speakers_result,
                        "location_result": location_result
                    }
            else:
                return {
                    "success": False,
                    "application": "Automated Azan",
                    "status": "configuration_invalid",
                    "validation_result": validation_result,
                    "config_info": config_info
                }

        except Exception as scheduler_error:
            return {
                "success": False,
                "application": "Automated Azan",
                "status": "scheduler_error",
                "error": str(scheduler_error),
                "configuration": {
                    "valid": validation_result.get('success', False)
                },
                "config_info": config_info
            }

    except Exception as e:
        return {
            "success": False,
            "application": "Automated Azan",
            "status": "critical_error",
            "error": str(e)
        }


if __name__ == "__main__":
    main()