import time
import json
import schedule
import logging
from datetime import datetime, timedelta
from dateutil import tz
from prayer_times_fetcher import PrayerTimesFetcher
from chromecast_manager import ChromecastManager
from time_sync import update_ntp_time


class AthanScheduler:
    """
    A class to fetch prayer times, schedule Athan playback, and handle execution failures gracefully.
    All methods return JSON responses for API compatibility.
    """

    def __init__(self, location="icci", google_device="athan"):
        """
        Initializes the Athan scheduler with:
        - `location`: 'naas' or 'icci' to select prayer times source.
        - `google_device`: Name of the Google Home speaker or speaker group.
        """
        self.location = location
        self.google_device = google_device
        self.fetcher = PrayerTimesFetcher()
        self.prayer_times = {}
        self.tz = tz.gettz('Europe/Dublin')
        self.chromecast_manager = ChromecastManager()

        # Load prayer times on initialization
        logging.info("[DEBUG] Initializing scheduler - loading prayer times and scheduling jobs")
        init_result = self.load_prayer_times()
        logging.info(f"[DEBUG] Initialization result: {init_result}")

        # Verify jobs were scheduled
        status = self.get_scheduler_status()
        logging.info(f"[DEBUG] Jobs scheduled during init: {status.get('total_jobs', 0)}")
        if not init_result.get('success', False):
            logging.warning("Failed to load prayer times during initialization")

    def get_next_prayer_time(self):
        """
        Get the next prayer time.

        Returns:
            dict: JSON response with next prayer information
        """
        try:
            now = datetime.now(self.tz)
            for prayer, time_tuple in self.prayer_times.items():
                try:
                    hour, minute = map(int, time_tuple.split(":"))
                    prayer_time = datetime(now.year, now.month, now.day, hour, minute, tzinfo=self.tz)
                    if prayer_time > now:
                        return {
                            "success": True,
                            "prayer": prayer,
                            "time": prayer_time.isoformat(),
                            "formatted_time": prayer_time.strftime("%H:%M"),
                            "seconds_until": (prayer_time - now).total_seconds(),
                            "current_time": now.isoformat()
                        }
                except ValueError as e:
                    logging.error("Error parsing time for prayer %s: %s", prayer, e)

            return {
                "success": True,
                "prayer": None,
                "time": None,
                "message": "No remaining prayers for today",
                "current_time": now.isoformat()
            }
        except Exception as e:
            logging.error(f"Error getting next prayer time: {e}")
            return {
                "success": False,
                "error": str(e),
                "current_time": datetime.now(self.tz).isoformat()
            }

    def load_prayer_times(self):
        """
        Fetches and stores today's prayer times from the selected API.

        Returns:
            dict: JSON response with prayer times data
        """
        try:
            logging.info("Fetching prayer times for location: %s", self.location)
            fetch_result = self.fetcher.fetch_prayer_times(self.location)

            if fetch_result.get('success', False):
                self.prayer_times = fetch_result.get('prayer_times', {})
                return {
                    "success": True,
                    "location": self.location,
                    "prayer_times": self.prayer_times,
                    "fetch_result": fetch_result,
                    "message": "Prayer times successfully fetched",
                    "timestamp": datetime.now(self.tz).isoformat()
                }
            else:
                logging.error("Failed to fetch prayer times: %s", fetch_result.get('error', 'Unknown error'))
                return {
                    "success": False,
                    "location": self.location,
                    "error": fetch_result.get('error', 'Unknown error'),
                    "fetch_result": fetch_result,
                    "timestamp": datetime.now(self.tz).isoformat()
                }
        except Exception as e:
            logging.error("Failed to fetch prayer times: %s", e)
            return {
                "success": False,
                "location": self.location,
                "error": str(e),
                "timestamp": datetime.now(self.tz).isoformat()
            }

    def schedule_prayers(self):
        """
        Schedules Athan for all upcoming prayer times today.

        Returns:
            dict: JSON response with scheduling results
        """
        try:
            schedule.clear()
            now = datetime.now(self.tz)
            scheduled_prayers = []
            skipped_prayers = []

            logging.info("Clearing previous schedules and scheduling today's prayers. Current time: %s", now.strftime("%Y-%m-%d %H:%M:%S"))
            logging.info(f"[DEBUG] Available prayer times: {self.prayer_times}")

            chromecast_manager = ChromecastManager()

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
                            scheduled_prayers.append({
                                "prayer": prayer,
                                "time": formatted_time,
                                "type": "regular_athan"
                            })
                        else:
                            schedule.every().day.at(formatted_time).do(chromecast_manager.start_adahn_alfajr)
                            scheduled_prayers.append({
                                "prayer": prayer,
                                "time": formatted_time,
                                "type": "fajr_athan"
                            })

                            # Schedule pre-Fajr Quran
                            wake_up_time = prayer_time - timedelta(minutes=45)
                            wake_up_str = wake_up_time.strftime("%H:%M")
                            schedule.every().day.at(wake_up_str).do(chromecast_manager.start_quran_radio)
                            scheduled_prayers.append({
                                "prayer": "Pre-Fajr Quran",
                                "time": wake_up_str,
                                "type": "quran_radio"
                            })

                        logging.debug("%s scheduled successfully at %s", prayer, formatted_time)
                    else:
                        skipped_prayers.append({
                            "prayer": prayer,
                            "time": time_tuple,
                            "reason": "Past time"
                        })
                        logging.warning("Skipping %s at %s as it's in the past.", prayer, time_tuple)

                except ValueError as e:
                    logging.error("Error parsing time for prayer %s: %s", prayer, e)
                    skipped_prayers.append({
                        "prayer": prayer,
                        "time": time_tuple,
                        "reason": f"Parse error: {str(e)}"
                    })

            # Final status check
            jobs_after = schedule.jobs
            logging.info(f"[DEBUG] Scheduling complete - {len(scheduled_prayers)} prayers scheduled")
            logging.info(f"[DEBUG] Total jobs in schedule: {len(jobs_after)}")

            return {
                "success": True,
                "scheduled_count": len(scheduled_prayers),
                "scheduled_prayers": scheduled_prayers,
                "skipped_prayers": skipped_prayers,
                "current_time": now.isoformat(),
                "message": f"Successfully scheduled {len(scheduled_prayers)} prayers"
            }

        except Exception as e:
            logging.error("Error during prayer scheduling: %s", e)
            return {
                "success": False,
                "error": str(e),
                "current_time": datetime.now(self.tz).isoformat()
            }

    def update_ntp_time(self):
        """
        Check and verify time synchronization using improved method

        Returns:
            dict: JSON response with time sync status
        """
        try:
            result = update_ntp_time()  # Use the new implementation from time_sync.py
            return {
                "success": True,
                "sync_result": result,
                "timestamp": datetime.now(self.tz).isoformat(),
                "message": "NTP time synchronization completed"
            }
        except Exception as e:
            logging.error(f"Error during NTP time sync: {e}")
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now(self.tz).isoformat()
            }

    def execute_prayer_athan(self, prayer_name):
        """
        Execute Athan for a specific prayer.

        Args:
            prayer_name (str): Name of the prayer

        Returns:
            dict: JSON response with execution result
        """
        try:
            if prayer_name.lower() == "fajr":
                success = self.chromecast_manager.start_adahn_alfajr()
            else:
                success = self.chromecast_manager.start_adahn()

            return {
                "success": success,
                "prayer": prayer_name,
                "timestamp": datetime.now(self.tz).isoformat(),
                "message": f"Athan {'played successfully' if success else 'failed to play'} for {prayer_name}"
            }
        except Exception as e:
            logging.error(f"Error executing Athan for {prayer_name}: {e}")
            return {
                "success": False,
                "prayer": prayer_name,
                "error": str(e),
                "timestamp": datetime.now(self.tz).isoformat()
            }

    def get_prayer_times(self):
        """
        Get current prayer times.

        Returns:
            dict: JSON response with prayer times
        """
        return {
            "success": True,
            "location": self.location,
            "prayer_times": self.prayer_times,
            "timezone": str(self.tz),
            "timestamp": datetime.now(self.tz).isoformat()
        }

    def get_scheduler_status(self):
        """
        Get current scheduler status.

        Returns:
            dict: JSON response with scheduler status
        """
        try:
            next_run = schedule.next_run()
            jobs = schedule.jobs

            # Debug logging
            logging.info(f"[DEBUG] Scheduler status check - found {len(jobs)} jobs")
            logging.info(f"[DEBUG] Next run: {next_run}")
            for i, job in enumerate(jobs):
                logging.info(f"[DEBUG] Job {i}: {job.job_func.__name__ if hasattr(job.job_func, '__name__') else str(job.job_func)} at {job.next_run}")

            return {
                "success": True,
                "next_run": next_run.isoformat() if next_run else None,
                "total_jobs": len(jobs),
                "jobs": [
                    {
                        "next_run": job.next_run.isoformat() if job.next_run else None,
                        "job_func": job.job_func.__name__ if hasattr(job.job_func, '__name__') else str(job.job_func)
                    } for job in jobs
                ],
                "current_time": datetime.now(self.tz).isoformat()
            }
        except Exception as e:
            logging.error(f"Error getting scheduler status: {e}")
            return {
                "success": False,
                "error": str(e),
                "current_time": datetime.now(self.tz).isoformat()
            }

    def refresh_schedule(self):
        """
        Refresh prayer times and schedule for a new day.

        Returns:
            dict: JSON response with refresh result
        """
        try:
            logging.info("Refreshing prayer times for a new day.")

            # Load new prayer times
            load_result = self.load_prayer_times()
            if not load_result.get('success', False):
                return {
                    "success": False,
                    "stage": "load_prayer_times",
                    "error": load_result.get('error', 'Unknown error'),
                    "timestamp": datetime.now(self.tz).isoformat()
                }

            # Schedule new prayers
            schedule_result = self.schedule_prayers()
            if not schedule_result.get('success', False):
                return {
                    "success": False,
                    "stage": "schedule_prayers",
                    "error": schedule_result.get('error', 'Unknown error'),
                    "timestamp": datetime.now(self.tz).isoformat()
                }

            return {
                "success": True,
                "prayer_times": self.prayer_times,
                "scheduled_count": schedule_result.get('scheduled_count', 0),
                "timestamp": datetime.now(self.tz).isoformat(),
                "message": "Schedule refreshed successfully"
            }

        except Exception as e:
            logging.error(f"Error refreshing schedule: {e}")
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now(self.tz).isoformat()
            }

    def run_scheduler(self):
        """
        Main scheduler loop. This is the only method that doesn't return JSON
        as it's meant to run indefinitely.
        """
        logging.info("Starting strict-loop Athan scheduler.")
        last_update_date = None

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
                    next_prayer_result = self.get_next_prayer_time()

                    if not next_prayer_result.get('prayer'):
                        logging.info("No remaining prayers for today. Sleeping until 1:00 AM.")
                        self.sleep_until_next_1am()
                        last_update_date = None
                        break

                    prayer = next_prayer_result['prayer']
                    sleep_duration = next_prayer_result['seconds_until']

                    logging.info(f"Next prayer: {prayer} at {next_prayer_result['formatted_time']}. Sleeping for {sleep_duration:.2f} seconds.")

                    if sleep_duration > 0:
                        # Sleep in chunks to allow for graceful shutdown
                        chunk_size = min(sleep_duration, 300)
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
                        execution_result = self.execute_prayer_athan(prayer)
                        if execution_result['success']:
                            logging.info(f"Successfully played Athan for {prayer}")
                        else:
                            logging.error(f"Failed to play Athan for {prayer}")

            except Exception as e:
                logging.error("Scheduler encountered an error: %s", e, exc_info=True)
                time.sleep(60)

    def sleep_until_next_1am(self):
        """
        Sleep until 1:00 AM the next day.

        Returns:
            dict: JSON response with sleep information (for logging/monitoring)
        """
        try:
            now = datetime.now(self.tz)
            next_1am = now.replace(hour=1, minute=0, second=0, microsecond=0)

            if now >= next_1am:
                next_1am += timedelta(days=1)

            sleep_duration = (next_1am - now).total_seconds()
            logging.info("Sleeping until 1:00 AM (%s). Sleep duration: %.2f seconds",
                        next_1am.strftime("%Y-%m-%d %H:%M:%S"), sleep_duration)

            # Sleep in chunks to avoid very long sleep periods
            while sleep_duration > 0:
                sleep_time = min(sleep_duration, 1800)  # 30-minute chunks
                time.sleep(sleep_time)
                sleep_duration -= sleep_time

                # Check if we're close to 1 AM
                now = datetime.now(self.tz)
                if now >= next_1am:
                    break

            logging.info("Woke up at 1:00 AM. Will refresh prayer times on next iteration.")

            return {
                "success": True,
                "wake_time": datetime.now(self.tz).isoformat(),
                "message": "Woke up at 1:00 AM"
            }

        except Exception as e:
            logging.error(f"Error during sleep cycle: {e}")
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now(self.tz).isoformat()
            }