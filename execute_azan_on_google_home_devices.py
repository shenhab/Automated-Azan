import time
import json
import schedule
import logging
import pychromecast
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

    def play_athan(self, prayer):
        """
        Plays the Athan on the specified Google Home device.

        :param prayer: The prayer name (e.g., 'Fajr', 'Dhuhr', etc.).
        """
        athan_urls = {
            "Fajr": "https://www.gurutux.com/media/adhan_al_fajr.mp3",
            "default": "https://www.gurutux.com/media/Athan.mp3",
            "elmesa7araty": "https://www.gurutux.com/media/elmese7araty.mp3"
        }

        volume_levels = {
            "Fajr": 0.5,
            "elmesa7araty": 1.0,
            "default": 1.0
        }

        azan_url = athan_urls.get(prayer, athan_urls["default"])
        volume = volume_levels.get(prayer, volume_levels["default"])

        logging.info("Attempting to play Athan for %s", prayer)

        try:
            chromecast_devices, _ = pychromecast.get_listed_chromecasts(
                friendly_names=[self.google_device], timeout=5
            )

            if not chromecast_devices:
                logging.error("No Chromecast device found with name: %s", self.google_device)
                return

            casting_device = chromecast_devices[0]
            casting_device.wait()

            cast_media_controller = casting_device.media_controller
            casting_device.set_volume(volume)
            cast_media_controller.play_media(azan_url, 'audio/mp3')

            logging.info("Athan is playing for %s", prayer)
            time.sleep(300)  # Allow Athan to play for 5 minutes

        except Exception as e:
            logging.error("Failed to play Athan for %s: %s", prayer, e)

    def schedule_prayers(self):
        """
        Schedules Athan for all upcoming prayer times today.
        """
        schedule.clear()
        now = datetime.now(self.tz)
        logging.info("Scheduling today's prayers.")
        chromecast_manager = ChromecastManager()

        for prayer, time_tuple in self.prayer_times.items():
            
            hour, minute = map(int, time_tuple.split(":"))
            prayer_time = datetime(now.year, now.month, now.day, hour, minute, tzinfo=self.tz)

            if prayer_time > now:  # Schedule only future prayers
                formatted_time = prayer_time.strftime("%H:%M")
                logging.info("Scheduling %s at %s", prayer, formatted_time)
                if prayer != "Fajr":
                    schedule.every().day.at(formatted_time).do(chromecast_manager.start_adahn)

                # For Fajr, also schedule the wake-up call
                if prayer == "Fajr":
                    schedule.every().day.at(formatted_time).do(chromecast_manager.start_adahn_alfajr)
                    wake_up_time = prayer_time - timedelta(minutes=45)
                    wake_up_str = wake_up_time.strftime("%H:%M")
                    schedule.every().day.at(wake_up_str).do(chromecast_manager.start_quran_radio)
                    logging.info("Scheduled wake-up call at %s", wake_up_str)

        logging.info("All prayers scheduled.")

    def run_scheduler(self):
        """
        Continuously runs the scheduler, ensuring that all Athans play at the correct time.
        If the script crashes, it restarts and resumes from where it left off.
        """
        logging.info("Starting Athan scheduler loop.")
        self.schedule_prayers()

        while True:
            try:
                schedule.run_pending()
                time.sleep(30)  # Check every 30 seconds for pending tasks
            except Exception as e:
                logging.error("Scheduler encountered an error: %s", e)
                time.sleep(10)  # Wait before retrying

    def sleep_until_midnight(self):
        """
        Sleeps until midnight, then refreshes the prayer times and restarts the schedule.
        """
        now = datetime.now(self.tz)
        midnight = datetime(now.year, now.month, now.day, 23, 59, 59, tzinfo=self.tz)

        logging.info("Sleeping until midnight to refresh prayer times.")
        time.sleep((midnight - now).total_seconds() + 500)  # Sleep until midnight

        logging.info("Refreshing prayer times for a new day.")
        self.load_prayer_times()
        self.schedule_prayers()
        
    def find_casting_candidate(self):
        """
        Finds a suitable Chromecast device to cast to.
        
        Priority order:
        1. If a Chromecast device named 'adahn' is found, it is returned immediately.
        2. Otherwise, if any Google Nest Mini devices are found, the first one is returned.
        3. If no suitable devices are found, an empty list is returned.
        
        :return: A list containing one Chromecast device or an empty list if none are found.
        """
        candidate_list = []  # Stores Google Nest Mini devices as fallback

        logging.info("Discovering Chromecast devices...")
        chromecasts, browser = pychromecast.get_chromecasts()  # Discover all Chromecast devices

        if not chromecasts:
            logging.warning("No Chromecast devices found.")
            return []

        logging.info(f"Found {len(chromecasts)} Chromecast devices.")

        for candidate in chromecasts:
            logging.debug(f"Checking device: {candidate.device.friendly_name} ({candidate.model_name})")

            # Check if the device name matches 'adahn' (case-insensitive)
            if candidate.device.friendly_name.lower() == 'adahn':
                logging.info(f"Found target Chromecast: {candidate.device.friendly_name}")
                return [candidate]  # Return the device immediately

            # If it's a Google Nest Mini, add it to the candidate list
            elif candidate.model_name == "Google Nest Mini":
                logging.info(f"Adding Google Nest Mini to candidate list: {candidate.device.friendly_name}")
                candidate_list.append(candidate)

        # If no 'adahn' was found but we have Google Nest Mini devices, return the first one
        if candidate_list:
            logging.info(f"Using fallback Chromecast: {candidate_list[0].device.friendly_name}")
            return [candidate_list[0]]

        # No suitable devices found
        logging.warning("No suitable Chromecast candidate found.")
        return []


if __name__ == "__main__":
    while True:
        scheduler = AthanScheduler(location="naas", google_device="Adahn")
        scheduler.run_scheduler()
        scheduler.sleep_until_midnight()
