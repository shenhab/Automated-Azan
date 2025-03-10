import time
import json
import schedule
import logging
import pychromecast
import configparser
from datetime import datetime, timedelta
from dateutil import tz
from prayer_times_fetcher import PrayerTimesFetcher  # Import the new class
from chromecast_manager import ChromecastManager

# Configure logging
logging.basicConfig(format='%(asctime)s [%(levelname)s]: %(message)s', 
                    filename="/var/log/azan_service.log", level=logging.INFO)

class AthanScheduler:
    """
    A class to fetch prayer times, schedule Athan playback, and handle execution failures gracefully.
    """
    
    def __init__(self, location="icci", google_device="Adahn"):
        """
        Initializes the Athan scheduler with:
        - `location`: 'naas' or 'icci' to select prayer times source.
        - `google_device`: Name of the Google Home speaker or speaker group.
        """
        self.location = location
        self.google_device = google_device
        self.fetcher = PrayerTimesFetcher()  # Instantiate the prayer times fetcher
        self.prayer_times = {}
        self.tz = tz.gettz('Europe/Dublin')
        self.load_prayer_times()  # Fetch prayer times immediately

    def load_prayer_times(self):
        """
        Fetches and stores today's prayer times from the selected API.
        """
        logging.info("Fetching prayer times for location: %s", self.location)
        try:
            self.prayer_times = self.fetcher.fetch_prayer_times(self.location)
            logging.info("Prayer times successfully fetched: %s", json.dumps(self.prayer_times, indent=4))
        except Exception as e:
            logging.error("Failed to fetch prayer times: %s", e)

    def schedule_prayers(self):
        """
        Schedules Athan for all upcoming prayer times today.
        """
        schedule.clear()
        now = datetime.now(self.tz)
        logging.info("Clearing previous schedules and scheduling today's prayers. Current time: %s", now.strftime("%Y-%m-%d %H:%M:%S"))

        chromecast_manager = ChromecastManager()
        scheduled_count = 0

        for prayer, time_tuple in self.prayer_times.items():
            logging.debug("Processing prayer: %s, scheduled time: %s", prayer, time_tuple)

            try:
                hour, minute = map(int, time_tuple.split(":"))
                prayer_time = datetime(now.year, now.month, now.day, hour, minute, tzinfo=self.tz)

                if prayer_time > now:  # Schedule only future prayers
                    formatted_time = prayer_time.strftime("%H:%M")
                    logging.info("Scheduling %s at %s", prayer, formatted_time)
                    
                    if prayer != "Fajr":
                        schedule.every().day.at(formatted_time).do(chromecast_manager.start_adahn)
                        logging.debug("%s scheduled successfully at %s", prayer, formatted_time)
                    else:
                        schedule.every().day.at(formatted_time).do(chromecast_manager.start_adahn_alfajr)
                        logging.debug("Fajr Athan scheduled at %s", formatted_time)

                        wake_up_time = prayer_time - timedelta(minutes=45)
                        wake_up_str = wake_up_time.strftime("%H:%M")
                        schedule.every().day.at(wake_up_str).do(chromecast_manager.start_quran_radio)
                        logging.info("Scheduled wake-up call for Fajr at %s", wake_up_str)

                    scheduled_count += 1
                else:
                    logging.warning("Skipping %s at %s as it's in the past.", prayer, time_tuple)
            
            except ValueError as e:
                logging.error("Error parsing time for prayer %s: %s", prayer, e)
            except Exception as e:
                logging.critical("Unexpected error while scheduling %s: %s", prayer, e, exc_info=True)

        logging.info("Total prayers scheduled: %d", scheduled_count)
        logging.info("Scheduling process completed.")


    def run_scheduler(self):
        """
        Continuously runs the scheduler, ensuring that all Athans play at the correct time.
        If the script crashes, it restarts and resumes from where it left off.
        """
        logging.info("Starting Athan scheduler loop.")
        self.schedule_prayers()  # Schedule initial prayers

        next_run = schedule.next_run()
        if next_run:
            logging.info("First scheduled task is at: %s", next_run.strftime("%Y-%m-%d %H:%M:%S"))

        while True:
            try:
                now = datetime.now(self.tz)
                logging.info("Checking pending tasks at %s", now.strftime("%Y-%m-%d %H:%M:%S"))

                schedule.run_pending()  # Execute scheduled jobs

                # Log next job time if available
                next_run = schedule.next_run()
                if next_run:
                    logging.info("Next scheduled task at: %s", next_run.strftime("%Y-%m-%d %H:%M:%S"))

                # Use schedule.idle_seconds() to sleep efficiently
                sleep_time = schedule.idle_seconds()
                if sleep_time is None or sleep_time < 0:
                    sleep_time = 30  # Default to 30 seconds if nothing is scheduled

                logging.info(f"Sleeping for {sleep_time:.2f} seconds.")
                time.sleep(sleep_time)

            except Exception as e:
                logging.error("Scheduler encountered an error: %s", e, exc_info=True)
                time.sleep(10)  # Wait before retrying


    def sleep_until_midnight(self):
        """
        Sleeps until just after midnight, then refreshes the prayer times and restarts the schedule.
        """
        now = datetime.now(self.tz)
        midnight = datetime(now.year, now.month, now.day, 23, 59, 59, tzinfo=self.tz)

        # If it's already midnight, target the next day's midnight
        if now.hour == 23 and now.minute >= 59:
            midnight += timedelta(days=1)

        # Calculate the exact sleep duration
        sleep_duration = (midnight - now).total_seconds() + 1  # Ensures we wake up right after midnight

        logging.info("Sleeping for %d seconds until midnight (%s).", int(sleep_duration), midnight.strftime("%Y-%m-%d %H:%M:%S"))

        # Sleep until midnight
        time.sleep(sleep_duration)

        # Confirm wake-up
        logging.info("Woke up after midnight. Refreshing prayer times for the new day.")

        # Refresh prayer times and reschedule
        self.load_prayer_times()
        self.schedule_prayers()

        # Confirm refresh completed
        logging.info("Prayer times refreshed and schedule restarted successfully.")
        
    def refresh_schedule(self):
        logging.info("Refreshing prayer times for a new day.")
        self.load_prayer_times()
        self.schedule_prayers()
    

if __name__ == "__main__":
    logging.info("Starting Adahn configuration loading...")

    config = configparser.ConfigParser()
    config.read("adahn.config")

    try:
        group_name = config["Settings"]["speakers-group-name"]
        location = config["Settings"]["location"]
        logging.info(f"Loaded configuration - speakers-group-name: {group_name}, location: {location}")
    except KeyError as e:
        logging.error(f"Missing required configuration key: {e}")
        exit(1)  # Stop execution if a key is missing

    try:
        scheduler = AthanScheduler(location=location, google_device=group_name)
        logging.info("AthanScheduler initialized successfully.")
        
        scheduler.run_scheduler()
        logging.info("AthanScheduler started successfully.")
    except Exception as e:
        logging.error(f"An error occurred while starting the scheduler: {e}", exc_info=True)
        exit(1)  # Stop execution on failure
