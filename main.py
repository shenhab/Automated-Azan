import time
import json
import schedule
import logging
import pychromecast
import configparser
import subprocess
from datetime import datetime, timedelta
from dateutil import tz
from prayer_times_fetcher import PrayerTimesFetcher  # Import the new class
from chromecast_manager import ChromecastManager
import os
from dotenv import load_dotenv

load_dotenv()

# Configure logging
log_file = os.environ.get('LOG_FILE', '/var/log/azan_service.log')
logging.basicConfig(format='%(asctime)s [%(levelname)s]: %(message)s', 
                    filename=log_file, level=logging.INFO)

# Also log to console for Docker logs
console_handler = logging.StreamHandler()
console_handler.setFormatter(logging.Formatter('%(asctime)s [%(levelname)s]: %(message)s'))
logging.getLogger().addHandler(console_handler)

# Reduce pychromecast logging verbosity to reduce connection error spam
logging.getLogger('pychromecast').setLevel(logging.WARNING)
logging.getLogger('pychromecast.socket_client').setLevel(logging.ERROR)


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
        self.chromecast_manager = ChromecastManager()

    def get_next_prayer_time(self):
        now = datetime.now(self.tz)
        for prayer, time_tuple in self.prayer_times.items():
            try:
                hour, minute = map(int, time_tuple.split(":"))
                prayer_time = datetime(now.year, now.month, now.day, hour, minute, tzinfo=self.tz)
                if prayer_time > now:
                    return prayer, prayer_time
            except ValueError as e:
                logging.error("Error parsing time for prayer %s: %s", prayer, e)
        return None, None

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

    def daily_schedule(self):
        """
        Runs the daily scheduling tasks. Exits when there are no pending tasks.
        """
        logging.info("Starting daily Athan scheduler.")
        self.schedule_prayers()  # Schedule today's prayers

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

                if sleep_time is None:
                    logging.info("No pending tasks. Exiting daily schedule.")
                    self.sleep_until_next_1am()
                    break  # Exit the loop
                elif sleep_time < 0:
                    sleep_time = 0.1  # Small sleep to avoid excessive looping

                logging.info(f"Sleeping for {sleep_time:.2f} seconds.")
                time.sleep(sleep_time)

            except Exception as e:
                logging.error("Daily scheduler encountered an error: %s", e, exc_info=True)
                time.sleep(10)  # Wait before retrying


    def update_ntp_time(self):
        """
        Updates the NTP time on the host and verifies synchronization.
        """
        logging.info("Updating NTP time on the host...")

        try:
            # Check if running in Docker container
            if os.path.exists('/.dockerenv'):
                logging.info("Running in Docker container, skipping NTP service restart")
                return
            
            # Restart the NTP service
            subprocess.run(["sudo", "systemctl", "restart", "systemd-timesyncd"], check=True)
            time.sleep(2)  # Allow time to sync

            # Check NTP synchronization status
            result = subprocess.run(["timedatectl", "status"], capture_output=True, text=True, check=True)
            logging.info("NTP status:\n%s", result.stdout)

            if "synchronized: yes" in result.stdout.lower():
                logging.info("✅ NTP time is properly synchronized.")
            else:
                logging.warning("⚠️ NTP time is not properly synchronized. Please check the configuration.")

        except subprocess.CalledProcessError as e:
            logging.error("❌ Error occurred while updating NTP: %s", e)


    def run_scheduler(self):
        """
        Updates the NTP time on the host and verifies synchronization.
        """
        logging.info("Updating NTP time on the host...")

        try:
            # Restart the NTP service
            subprocess.run(["sudo", "systemctl", "restart", "systemd-timesyncd"], check=True)
            time.sleep(2)  # Allow time to sync

            # Check NTP synchronization status
            result = subprocess.run(["timedatectl", "status"], capture_output=True, text=True, check=True)
            logging.info("NTP status:\n%s", result.stdout)

            if "synchronized: yes" in result.stdout.lower():
                logging.info("✅ NTP time is properly synchronized.")
            else:
                logging.warning("⚠️ NTP time is not properly synchronized. Please check the configuration.")

        except subprocess.CalledProcessError as e:
            logging.error("❌ Error occurred while updating NTP: %s", e)


    def run_scheduler(self):
        logging.info("Starting strict-loop Athan scheduler.")
        last_update_date = None  # Track when we last updated prayer times
        
        while True:
            try:
                current_date = datetime.now(self.tz).date()
                
                # Only update prayer times once per day or on first run
                if last_update_date != current_date:
                    logging.info(f"Updating prayer times for new day: {current_date}")
                    self.update_ntp_time()
                    self.load_prayer_times()
                    last_update_date = current_date
                else:
                    logging.debug(f"Using cached prayer times for {current_date}")

                while True:
                    prayer, next_prayer_time = self.get_next_prayer_time()
                    if not prayer:
                        logging.info("No remaining prayers for today. Sleeping until 1:00 AM.")
                        self.sleep_until_next_1am()
                        last_update_date = None  # Force update on next iteration
                        break

                    sleep_duration = (next_prayer_time - datetime.now(self.tz)).total_seconds()
                    logging.info(f"Next prayer: {prayer} at {next_prayer_time.strftime('%H:%M')}. Sleeping for {sleep_duration:.2f} seconds.")

                    if sleep_duration > 0:
                        # Sleep in chunks to allow for graceful shutdown and reduce log spam
                        chunk_size = min(sleep_duration, 300)  # Sleep in 5-minute chunks max
                        while sleep_duration > 0:
                            sleep_time = min(chunk_size, sleep_duration)
                            time.sleep(sleep_time)
                            sleep_duration -= sleep_time
                            
                            # Check if we've moved to a new day during sleep
                            if datetime.now(self.tz).date() != current_date:
                                logging.info("Date changed during sleep, will update prayer times")
                                last_update_date = None
                                break

                    # Only execute if we're still on the same day
                    if datetime.now(self.tz).date() == current_date:
                        try:
                            # Execute the Athan playback at precise time
                            if prayer == "Fajr":
                                success = self.chromecast_manager.start_adahn_alfajr()
                            else:
                                success = self.chromecast_manager.start_adahn()
                                
                            if not success:
                                logging.error(f"Failed to play Athan for {prayer}")
                            else:
                                logging.info(f"Successfully played Athan for {prayer}")
                                
                        except Exception as e:
                            logging.error(f"Error executing Athan for {prayer}: {e}")

            except Exception as e:
                logging.error("Scheduler encountered an error: %s", e, exc_info=True)
                time.sleep(60)  # Wait 1 minute before retrying to avoid spam


    def sleep_until_next_1am(self):
        now = datetime.now(self.tz)
        next_1am = now.replace(hour=1, minute=0, second=0, microsecond=0)

        if now >= next_1am:
            next_1am += timedelta(days=1)

        sleep_duration = (next_1am - now).total_seconds()
        logging.info("Sleeping until 1:00 AM (%s). Sleep duration: %.2f seconds", next_1am.strftime("%Y-%m-%d %H:%M:%S"), sleep_duration)
        
        # Sleep in chunks to avoid very long sleep periods that are hard to interrupt
        while sleep_duration > 0:
            sleep_time = min(sleep_duration, 1800)  # Sleep in 30-minute chunks
            time.sleep(sleep_time)
            sleep_duration -= sleep_time
            
            # Check if we're close to 1 AM
            now = datetime.now(self.tz)
            if now >= next_1am:
                break

        logging.info("Woke up at 1:00 AM. Will refresh prayer times on next iteration.")

        
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
