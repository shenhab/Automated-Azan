#!/usr/bin/env python3
"""
Automated Azan — Main Entry Point

Initialises logging, loads settings, starts the Athan scheduler (blocking),
and launches the web interface in a background thread.
"""

import logging
import threading
import sys

from settings import settings
from logging_setup import setup_logging
from athan_scheduler import AthanScheduler
from web_interface import start_web_interface
from config_watcher import ConfigWatcher


def main():
    print("Starting Automated Azan application...")

    logging_result = setup_logging(settings.log.file_path)
    if not logging_result.get("success", False):
        print("Failed to setup logging. Exiting.")
        sys.exit(1)

    logging.info("Starting Automated Azan application...")

    try:
        group_name = settings.speaker.group_name
        location = settings.prayer.location

        logging.info("Configuration — speaker: %s, location: %s", group_name, location)
        print(f"Configuration loaded — speaker: {group_name}, location: {location}")

        print("Initialising Athan scheduler...")
        scheduler = AthanScheduler(location=location, google_device=group_name)

        prayer_times_result = scheduler.get_prayer_times()
        if prayer_times_result.get("success", False):
            prayer_times = prayer_times_result["prayer_times"]
            logging.info("Prayer times for %s: %s", location, prayer_times)
            print(f"Prayer times for {location}:")
            for prayer, time in prayer_times.items():
                print(f"   {prayer}: {time}")

        print("Setting up configuration hot-reload...")
        config_watcher = ConfigWatcher(scheduler)
        watcher_result = config_watcher.start()

        if watcher_result.get("success"):
            print("Config hot-reload enabled — changes apply automatically")
        else:
            print(f"Config hot-reload unavailable: {watcher_result.get('error')} — restart required for changes")

        print("Starting web interface...")
        web_thread = threading.Thread(
            target=start_web_interface,
            args=(scheduler.chromecast_manager, scheduler, config_watcher),
            daemon=True,
        )
        web_thread.start()
        print("Web interface started on http://localhost:5000")

        next_prayer_result = scheduler.get_next_prayer_time()
        if next_prayer_result.get("success", False) and next_prayer_result.get("prayer"):
            print(f"Next prayer: {next_prayer_result['prayer']} at {next_prayer_result['formatted_time']}")
        else:
            print("No remaining prayers for today")

        print("Starting main scheduler...")
        print("Monitor status at: http://localhost:5000")
        scheduler.run_scheduler()

    except Exception as e:
        logging.error("Application startup failed: %s", e, exc_info=True)
        print(f"Application startup failed: {e}")
        sys.exit(1)


def get_application_status() -> dict:
    """
    Return current application status as a dict.
    Can be called by other modules importing main.
    """
    from datetime import datetime

    try:
        scheduler = AthanScheduler(
            location=settings.prayer.location,
            google_device=settings.speaker.group_name,
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
                "speakers_group": settings.speaker.group_name,
                "location": settings.prayer.location,
            },
            "prayer_times": prayer_times,
            "next_prayer": next_prayer,
            "scheduler": scheduler_status,
            "timestamp": datetime.now().isoformat(),
        }
    except Exception as e:
        from datetime import datetime

        return {
            "success": False,
            "application": "Automated Azan",
            "status": "error",
            "error": str(e),
            "timestamp": datetime.now().isoformat(),
        }


if __name__ == "__main__":
    main()
