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
from twilio.rest import Client
from dotenv import load_dotenv

load_dotenv()

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
        self.chromecast_manager = ChromecastManager()
        
        # Load environment variables
        self.ACCOUNT_SID = os.getenv("TWILIO_ACCOUNT_SID")
        self.AUTH_TOKEN = os.getenv("TWILIO_AUTH_TOKEN")
        self.CONTENT_SID = os.getenv("TWILIO_CONTENT_SID")
        self.TWILIO_WHATSAPP_NUMBER = os.getenv("TWILIO_WHATSAPP_NUMBER")
        self.RECIPIENT_NUMBER = os.getenv("RECIPIENT_NUMBER")

        # Debugging: Log loaded values
        logging.debug("Loaded Twilio Environment Variables:")
        logging.debug(f"  TWILIO_ACCOUNT_SID: {self.ACCOUNT_SID}")
        logging.debug(f"  TWILIO_AUTH_TOKEN: {'Set' if self.AUTH_TOKEN else 'Not Set'}")  # Mask token
        logging.debug(f"  TWILIO_CONTENT_SID: {self.CONTENT_SID}")
        logging.debug(f"  TWILIO_WHATSAPP_NUMBER: {self.TWILIO_WHATSAPP_NUMBER}")
        logging.debug(f"  RECIPIENT_NUMBER: {self.RECIPIENT_NUMBER}")

    def send_whatsapp_notification(self, prayer_name, scheduled_time):
        """
        Sends a WhatsApp notification for prayer time with the current system time and scheduled time.

        :param prayer_name: The name of the prayer (e.g., Fajr, Dhuhr).
        :param scheduled_time: The scheduled time of the prayer (formatted as HH:MM).
        """
        try:
            current_time = datetime.now(self.tz).strftime('%H:%M:%S')  # Get the current system time

            # Initialize Twilio client
            client = Client(self.ACCOUNT_SID, self.AUTH_TOKEN)

            # Send WhatsApp message
            message = client.messages.create(
                from_=self.TWILIO_WHATSAPP_NUMBER,
                content_sid=self.CONTENT_SID,
                content_variables=f'{{"1":"{prayer_name}","2":"{scheduled_time}","3":"{current_time}"}}',
                to=self.RECIPIENT_NUMBER
            )

            logging.info(f"✅ WhatsApp notification sent successfully. Message SID: {message.sid}")
            return message.sid
        except Exception as e:
            logging.error(f"❌ Error sending WhatsApp notification: {e}", exc_info=True)


            
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
        while True:
            try:
                self.update_ntp_time()
                self.load_prayer_times()

                while True:
                    prayer, next_prayer_time = self.get_next_prayer_time()
                    if not prayer:
                        logging.info("No remaining prayers for today. Sleeping until 1:00 AM.")
                        self.sleep_until_next_1am()
                        break

                    sleep_duration = (next_prayer_time - datetime.now(self.tz)).total_seconds()
                    logging.info(f"Next prayer: {prayer} at {next_prayer_time.strftime('%H:%M')}. Sleeping for {sleep_duration:.2f} seconds.")

                    if sleep_duration > 0:
                        time.sleep(sleep_duration)

                    self.send_whatsapp_notification(prayer, next_prayer_time.strftime('%H:%M'))
                    # Execute the Athan playback at precise time
                    if prayer == "Fajr":
                        self.chromecast_manager.start_adahn_alfajr()
                    else:
                        self.chromecast_manager.start_adahn()

            except Exception as e:
                logging.error("Scheduler encountered an error: %s", e, exc_info=True)
                time.sleep(10)  # Small delay before retrying


    def sleep_until_next_1am(self):
        now = datetime.now(self.tz)
        next_1am = now.replace(hour=1, minute=0, second=0, microsecond=0)

        if now >= next_1am:
            next_1am += timedelta(days=1)

        sleep_duration = (next_1am - now).total_seconds()
        logging.info("Sleeping until 1:00 AM (%s).", next_1am.strftime("%Y-%m-%d %H:%M:%S"))
        time.sleep(sleep_duration)

        logging.info("Woke up at 1:00 AM. Refreshing prayer times.")
        self.update_ntp_time()
        self.load_prayer_times()

        
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
