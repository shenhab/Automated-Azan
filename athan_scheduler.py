import time
import json
import os
import random
import schedule
import logging
import threading
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
        self.chromecast_manager = ChromecastManager(google_device_name=self.google_device)

        # Load prayer times on initialization
        logging.info("[DEBUG] Initializing scheduler - loading prayer times and scheduling jobs")
        init_result = self.load_prayer_times()
        logging.info(f"[DEBUG] Prayer times load result: {init_result}")

        # Schedule prayers if prayer times were loaded successfully
        if init_result.get('success', False):
            logging.info("[DEBUG] Prayer times loaded successfully, now scheduling prayers")
            schedule_result = self.schedule_prayers()
            logging.info(f"[DEBUG] Prayer scheduling result: {schedule_result}")
        else:
            logging.warning("Failed to load prayer times during initialization")

        # Verify jobs were scheduled
        status = self.get_scheduler_status()
        logging.info(f"[DEBUG] Jobs scheduled during init: {status.get('total_jobs', 0)}")

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

            from settings import settings
            pre_fajr_enabled = settings.prayer.pre_fajr_enabled
            friday_kahf_enabled = settings.prayer.friday_kahf_enabled

            # Use the existing chromecast_manager instance instead of creating a new one
            chromecast_manager = self.chromecast_manager

            for prayer, time_tuple in self.prayer_times.items():
                logging.debug("Processing prayer: %s, scheduled time: %s", prayer, time_tuple)

                try:
                    hour, minute = map(int, time_tuple.split(":"))
                    prayer_time = datetime(now.year, now.month, now.day, hour, minute, tzinfo=self.tz)

                    if prayer_time > now:  # Schedule only future prayers
                        formatted_time = prayer_time.strftime("%H:%M")
                        logging.info("Scheduling %s at %s", prayer, formatted_time)

                        if prayer == "Fajr":
                            schedule.every().day.at(formatted_time).do(chromecast_manager.start_adahn_alfajr).tag(prayer.lower())
                            scheduled_prayers.append({
                                "prayer": prayer,
                                "time": formatted_time,
                                "type": "fajr_athan"
                            })
                            if pre_fajr_enabled:
                                pre_fajr_result = self.schedule_pre_fajr_quran_for_time(prayer_time)
                                if pre_fajr_result.get('success'):
                                    scheduled_prayers.append({
                                        "prayer": "Pre-Fajr Quran",
                                        "time": pre_fajr_result.get('scheduled_time'),
                                        "type": "quran_recitation"
                                    })
                                else:
                                    logging.warning(f"Failed to schedule pre-Fajr Quran: {pre_fajr_result.get('error')}")

                        elif prayer == "Dhuhr":
                            schedule.every().day.at(formatted_time).do(chromecast_manager.start_adahn).tag(prayer.lower())
                            scheduled_prayers.append({
                                "prayer": prayer,
                                "time": formatted_time,
                                "type": "regular_athan"
                            })
                            if friday_kahf_enabled and now.weekday() == 4:  # 4 = Friday
                                kahf_result = self.schedule_friday_kahf_for_time(prayer_time)
                                if kahf_result.get('success'):
                                    scheduled_prayers.append({
                                        "prayer": "Friday Surah Al-Kahf",
                                        "time": kahf_result.get('scheduled_time'),
                                        "type": "friday_kahf"
                                    })
                                else:
                                    logging.warning(f"Failed to schedule Friday Surah Al-Kahf: {kahf_result.get('error')}")

                        else:
                            schedule.every().day.at(formatted_time).do(chromecast_manager.start_adahn).tag(prayer.lower())
                            scheduled_prayers.append({
                                "prayer": prayer,
                                "time": formatted_time,
                                "type": "regular_athan"
                            })

                        logging.debug("%s scheduled successfully at %s", prayer, formatted_time)
                    else:
                        skipped_prayers.append({
                            "prayer": prayer,
                            "time": time_tuple,
                            "reason": "Past time"
                        })
                        logging.info("Skipping %s at %s as it's in the past.", prayer, time_tuple)

                except ValueError as e:
                    logging.error("Error parsing time for prayer %s: %s", prayer, e)
                    skipped_prayers.append({
                        "prayer": prayer,
                        "time": time_tuple,
                        "reason": f"Parse error: {str(e)}"
                    })

            self.skipped_prayers = skipped_prayers  # persist for status view
            logging.info("Scheduling complete — %d upcoming, %d already past",
                         len(scheduled_prayers), len(skipped_prayers))

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
        Get current scheduler status including done and upcoming jobs for today.

        Returns:
            dict: JSON response with scheduler status
        """
        try:
            from datetime import timedelta
            now = datetime.now(self.tz)
            today = now.date()

            _label_map = {
                "fajr":         ("Fajr Athan",           "fajr_athan"),
                "dhuhr":        ("Dhuhr Athan",           "regular_athan"),
                "asr":          ("Asr Athan",             "regular_athan"),
                "maghrib":      ("Maghrib Athan",         "regular_athan"),
                "isha":         ("Isha Athan",            "regular_athan"),
                "pre_fajr":     ("Pre-Fajr Quran",        "quran_recitation"),
                "friday_kahf":  ("Friday Surah Al-Kahf",  "friday_kahf"),
            }

            def _job_label(job):
                for tag in getattr(job, 'tags', set()):
                    if tag in _label_map:
                        return _label_map[tag]
                return getattr(job.job_func, '__name__', str(job.job_func)), "unknown"

            upcoming = []
            done_today = []

            for job in schedule.jobs:
                label, type_ = _job_label(job)
                tags = list(getattr(job, 'tags', set()))

                if job.next_run and job.next_run.date() > today:
                    # Job fired today — next_run is tomorrow; recover the time it played
                    played_at = job.next_run - timedelta(days=1)
                    done_today.append({
                        "label": label,
                        "type": type_,
                        "tags": tags,
                        "status": "done",
                        "scheduled_time": played_at.strftime("%H:%M"),
                        "next_run": None,
                    })
                else:
                    upcoming.append({
                        "label": label,
                        "type": type_,
                        "tags": tags,
                        "status": "upcoming",
                        "scheduled_time": job.next_run.strftime("%H:%M") if job.next_run else None,
                        "next_run": job.next_run.isoformat() if job.next_run else None,
                    })

            # Prayers that were already past when schedule_prayers() last ran
            for s in getattr(self, 'skipped_prayers', []):
                prayer_key = s.get('prayer', '').lower()
                label, type_ = _label_map.get(prayer_key, (s.get('prayer', 'Unknown'), 'unknown'))
                done_today.append({
                    "label": label,
                    "type": type_,
                    "tags": [prayer_key],
                    "status": "done",
                    "scheduled_time": s.get('time'),
                    "next_run": None,
                })

            done_today.sort(key=lambda j: j['scheduled_time'] or '')
            upcoming.sort(key=lambda j: j['next_run'] or '')
            all_jobs = done_today + upcoming

            next_run = schedule.next_run()
            return {
                "success": True,
                "running": True,
                "next_run": next_run.isoformat() if next_run else None,
                "total_jobs": len(upcoming),
                "jobs": all_jobs,
                "current_time": now.isoformat(),
            }
        except Exception as e:
            logging.error("Error getting scheduler status: %s", e)
            return {
                "success": False,
                "error": str(e),
                "current_time": datetime.now(self.tz).isoformat(),
            }

    def check_dst_change(self):
        """
        Check if DST has changed since the last timetable download.
        This is called daily at 1:00 AM to detect DST transitions.

        Returns:
            dict: JSON response with DST check result
        """
        try:
            dst_changed = self.fetcher._has_dst_changed(self.location)
            current_offset = self.fetcher._get_dst_offset()

            result = {
                "success": True,
                "dst_changed": dst_changed,
                "current_offset": current_offset,
                "location": self.location,
                "timestamp": datetime.now(self.tz).isoformat()
            }

            if dst_changed:
                # Get the old offset from metadata if available
                try:
                    metadata_file = self.fetcher._get_dst_metadata_file(self.location)
                    if os.path.exists(metadata_file):
                        import json
                        with open(metadata_file, 'r') as f:
                            metadata = json.load(f)
                            result['old_offset'] = metadata.get('dst_offset', 'unknown')
                            result['new_offset'] = current_offset
                            result['message'] = f"DST changed from {result['old_offset']}s to {current_offset}s"
                except Exception as e:
                    logging.debug(f"Could not read old DST metadata: {e}")
                    result['message'] = "DST change detected"
            else:
                result['message'] = "No DST change detected"

            return result

        except Exception as e:
            logging.error(f"Error checking DST change: {e}")
            return {
                "success": False,
                "dst_changed": False,
                "error": str(e),
                "timestamp": datetime.now(self.tz).isoformat()
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

    def schedule_pre_fajr_quran(self):
        """
        Schedule Quran recitation 30 minutes before Fajr.

        Returns:
            dict: JSON response with scheduling result
        """
        if 'Fajr' not in self.prayer_times:
            logging.warning("Cannot schedule pre-Fajr: Fajr time not found")
            return {"success": False, "error": "Fajr time not found"}

        try:
            fajr_time_str = self.prayer_times['Fajr']
            hour, minute = map(int, fajr_time_str.split(':'))
            now = datetime.now(self.tz)
            fajr_dt = datetime(now.year, now.month, now.day, hour, minute, tzinfo=self.tz)

            return self.schedule_pre_fajr_quran_for_time(fajr_dt)
        except Exception as e:
            logging.error(f"Failed to schedule pre-Fajr Quran: {e}")
            return {"success": False, "error": str(e)}

    def schedule_pre_fajr_quran_for_time(self, fajr_time):
        """
        Schedule Quran recitation before Fajr using the configured pre_fajr_minutes offset.

        Args:
            fajr_time (datetime): The Fajr prayer time

        Returns:
            dict: JSON response with scheduling result
        """
        try:
            from settings import settings
            offset = settings.prayer.pre_fajr_minutes
            pre_fajr_dt = fajr_time - timedelta(minutes=offset)
            pre_fajr_time = pre_fajr_dt.strftime("%H:%M")

            schedule.every().day.at(pre_fajr_time).do(
                self.play_pre_fajr_quran
            ).tag('pre_fajr')

            logging.info(f"Scheduled pre-Fajr Quran at {pre_fajr_time} ({offset} min before Fajr at {fajr_time.strftime('%H:%M')})")
            return {
                "success": True,
                "scheduled_time": pre_fajr_time,
                "fajr_time": fajr_time.strftime('%H:%M'),
                "offset_minutes": offset,
            }
        except Exception as e:
            logging.error(f"Failed to schedule pre-Fajr Quran: {e}")
            return {"success": False, "error": str(e)}

    def play_pre_fajr_quran(self):
        """
        Start a background playlist of shuffled Quran recitations from
        server13.mp3quran.net/husr/ that plays until pre_fajr_minutes have elapsed.
        """
        from settings import settings

        _BASE = "https://server13.mp3quran.net/husr"
        _SUBDIRS = [
            "",
            "Almusshaf-Al-Mojawwad",
            "Rewayat-Aldori-A-n-Abi-Amr",
            "Rewayat-Qalon-A-n-Nafi",
            "Rewayat-Warsh-A-n-Nafi",
        ]
        all_urls = [
            f"{_BASE}/{subdir + '/' if subdir else ''}{n:03d}.mp3"
            for subdir in _SUBDIRS
            for n in range(1, 115)
        ]
        random.shuffle(all_urls)

        duration_seconds = settings.prayer.pre_fajr_minutes * 60

        # Stop any previous playlist still running
        if hasattr(self, '_pre_fajr_stop_event') and self._pre_fajr_stop_event:
            self._pre_fajr_stop_event.set()

        stop_event = threading.Event()
        self._pre_fajr_stop_event = stop_event
        pre_fajr_speaker = settings.speaker.resolve("pre_fajr")

        t = threading.Thread(
            target=self._run_quran_playlist,
            args=(all_urls, duration_seconds, stop_event, pre_fajr_speaker),
            daemon=True,
            name="pre-fajr-quran",
        )
        t.start()

        logging.info(
            f"Pre-Fajr Quran playlist started: {len(all_urls)} tracks shuffled, "
            f"{settings.prayer.pre_fajr_minutes}-minute window"
        )
        return {
            "success": True,
            "tracks_available": len(all_urls),
            "duration_minutes": settings.prayer.pre_fajr_minutes,
            "message": f"Pre-Fajr Quran playlist started ({settings.prayer.pre_fajr_minutes} min window)",
            "timestamp": datetime.now().isoformat(),
        }

    def _run_quran_playlist(self, urls, duration_seconds, stop_event, speaker_override=None):
        """Background thread: play URLs sequentially until the window closes or stop is requested."""
        end_time = time.time() + duration_seconds

        for url in urls:
            if stop_event.is_set() or time.time() >= end_time:
                break

            logging.info(f"Pre-Fajr Quran: playing {url}")
            result = self.chromecast_manager.play_url_on_cast(
                url,
                speaker_override=speaker_override,
                media_title="تلاوة ما قبل الفجر",
                media_artist="القرآن الكريم",
                stream_type="BUFFERED",
            )

            if not result.get('success'):
                logging.warning(f"Pre-Fajr Quran: failed to play {url}, skipping")
                continue

            self._wait_for_track_end(end_time, stop_event)

        logging.info("Pre-Fajr Quran playlist finished")

    def _wait_for_track_end(self, end_time, stop_event, poll_interval=5):
        """Poll Chromecast status until the current track ends, stop fires, or end_time passes."""
        # Wait for playback to start (up to ~20s)
        for _ in range(4):
            if stop_event.is_set() or time.time() >= end_time:
                return
            try:
                mc = self.chromecast_manager.target_device.media_controller
                mc.update_status()
                if mc.status.player_state in ("PLAYING", "BUFFERING"):
                    break
            except Exception:
                pass
            time.sleep(poll_interval)

        # Wait for IDLE (track ended)
        while time.time() < end_time and not stop_event.is_set():
            try:
                mc = self.chromecast_manager.target_device.media_controller
                mc.update_status()
                if mc.status.player_state in ("IDLE", "UNKNOWN", None, ""):
                    return
            except Exception:
                pass
            time.sleep(poll_interval)

    def cancel_pre_fajr_quran(self):
        """
        Cancel scheduled pre-Fajr Quran and stop any running playlist thread.

        Returns:
            dict: JSON response with cancellation result
        """
        try:
            # Stop the background playlist thread if running
            if hasattr(self, '_pre_fajr_stop_event') and self._pre_fajr_stop_event:
                self._pre_fajr_stop_event.set()
                self._pre_fajr_stop_event = None

            pre_fajr_jobs = [job for job in schedule.jobs if 'pre_fajr' in getattr(job, 'tags', set())]
            schedule.clear('pre_fajr')

            logging.info(f"Cancelled {len(pre_fajr_jobs)} pre-Fajr Quran schedule(s)")
            return {
                "success": True,
                "message": f"Pre-Fajr Quran cancelled ({len(pre_fajr_jobs)} job(s) removed)",
                "jobs_removed": len(pre_fajr_jobs),
            }
        except Exception as e:
            logging.error(f"Error cancelling pre-Fajr Quran: {e}")
            return {"success": False, "error": str(e)}

    def toggle_pre_fajr_quran(self, enable):
        """
        Enable or disable pre-Fajr Quran.

        Args:
            enable (bool): True to enable, False to disable

        Returns:
            dict: JSON response with toggle result
        """
        try:
            if enable:
                # Cancel any existing pre-Fajr jobs first
                self.cancel_pre_fajr_quran()
                # Schedule new pre-Fajr
                result = self.schedule_pre_fajr_quran()
                if result.get('success'):
                    return {
                        "success": True,
                        "enabled": True,
                        "message": f"Pre-Fajr Quran enabled at {result.get('scheduled_time')}"
                    }
                else:
                    return result
            else:
                # Disable pre-Fajr
                result = self.cancel_pre_fajr_quran()
                if result.get('success'):
                    return {
                        "success": True,
                        "enabled": False,
                        "message": "Pre-Fajr Quran disabled"
                    }
                else:
                    return result
        except Exception as e:
            logging.error(f"Error toggling pre-Fajr Quran: {e}")
            return {"success": False, "error": str(e)}

    # ------------------------------------------------------------------ #
    #  Friday Surah Al-Kahf                                                #
    # ------------------------------------------------------------------ #

    def schedule_friday_kahf_for_time(self, dhuhr_time):
        """Schedule Surah Al-Kahf every Friday, 2 hours before the given Dhuhr time."""
        try:
            kahf_dt = dhuhr_time - timedelta(hours=2)
            kahf_time = kahf_dt.strftime("%H:%M")

            schedule.every().friday.at(kahf_time).do(
                self.play_friday_kahf
            ).tag('friday_kahf')

            logging.info(
                f"Scheduled Friday Surah Al-Kahf at {kahf_time} "
                f"(2 hours before Dhuhr at {dhuhr_time.strftime('%H:%M')})"
            )
            return {
                "success": True,
                "scheduled_time": kahf_time,
                "dhuhr_time": dhuhr_time.strftime('%H:%M'),
            }
        except Exception as e:
            logging.error(f"Failed to schedule Friday Surah Al-Kahf: {e}")
            return {"success": False, "error": str(e)}

    def play_friday_kahf(self):
        """Play Surah Al-Kahf (018.mp3) from server13.mp3quran.net/husr/."""
        url = "https://server13.mp3quran.net/husr/018.mp3"
        logging.info(f"Friday: playing Surah Al-Kahf from {url}")
        result = self.chromecast_manager.play_url_on_cast(
            url,
            speaker_override=settings.speaker.resolve("friday_kahf"),
            media_title="سورة الكهف",
            media_artist="تلاوة جمعة مباركة",
            stream_type="BUFFERED",
        )
        if result.get('success'):
            logging.info("Friday Surah Al-Kahf started successfully")
        else:
            logging.error(f"Failed to play Surah Al-Kahf: {result.get('error')}")
        return result

    def cancel_friday_kahf(self):
        """Remove all scheduled Friday Al-Kahf jobs."""
        try:
            jobs = [j for j in schedule.jobs if 'friday_kahf' in getattr(j, 'tags', set())]
            schedule.clear('friday_kahf')
            logging.info(f"Cancelled {len(jobs)} Friday Al-Kahf schedule(s)")
            return {
                "success": True,
                "message": f"Friday Al-Kahf cancelled ({len(jobs)} job(s) removed)",
                "jobs_removed": len(jobs),
            }
        except Exception as e:
            logging.error(f"Error cancelling Friday Al-Kahf: {e}")
            return {"success": False, "error": str(e)}

    def toggle_friday_kahf(self, enable):
        """Enable or disable Friday Surah Al-Kahf playback."""
        try:
            self.cancel_friday_kahf()
            if enable:
                result = self.schedule_friday_kahf_for_time(self._dhuhr_as_datetime())
                if result.get('success'):
                    return {"success": True, "enabled": True,
                            "message": f"Friday Al-Kahf enabled at {result.get('scheduled_time')}"}
                return result
            return {"success": True, "enabled": False, "message": "Friday Al-Kahf disabled"}
        except Exception as e:
            logging.error(f"Error toggling Friday Al-Kahf: {e}")
            return {"success": False, "error": str(e)}

    def _dhuhr_as_datetime(self):
        """Return today's Dhuhr time as a timezone-aware datetime (or raise if not loaded)."""
        dhuhr_str = self.prayer_times.get('Dhuhr')
        if not dhuhr_str:
            raise ValueError("Dhuhr prayer time not loaded")
        hour, minute = map(int, dhuhr_str.split(':'))
        now = datetime.now(self.tz)
        return datetime(now.year, now.month, now.day, hour, minute, tzinfo=self.tz)

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

                    # Check for DST changes at 1:00 AM
                    dst_check_result = self.check_dst_change()
                    if dst_check_result.get('dst_changed', False):
                        logging.warning(f"⚠️  DST CHANGE DETECTED: {dst_check_result.get('message', 'DST offset changed')}")
                        logging.warning(f"    Old offset: {dst_check_result.get('old_offset', 'unknown')}s -> New offset: {dst_check_result.get('new_offset', 'unknown')}s")
                    else:
                        logging.info(f"✅ DST check: No change detected (offset: {dst_check_result.get('current_offset', 'unknown')}s)")

                    self.update_ntp_time()
                    refresh_result = self.refresh_schedule()
                    if refresh_result.get('success', False):
                        last_update_date = current_date
                    else:
                        logging.error(f"Failed to refresh schedule: {refresh_result.get('error', 'Unknown error')}")
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
                        # Sleep in 30-second chunks so schedule.run_pending() fires sub-jobs
                        while sleep_duration > 0:
                            sleep_time = min(30, sleep_duration)
                            time.sleep(sleep_time)
                            sleep_duration -= sleep_time
                            schedule.run_pending()

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
                next_1am = next_1am + timedelta(days=1)

            sleep_duration = (next_1am - now).total_seconds()
            logging.info("Sleeping until 1:00 AM (%s). Sleep duration: %.2f seconds",
                        next_1am.strftime("%Y-%m-%d %H:%M:%S"), sleep_duration)

            # Sleep in 30-second chunks so schedule.run_pending() can fire any queued jobs
            while sleep_duration > 0:
                sleep_time = min(30, sleep_duration)
                time.sleep(sleep_time)
                sleep_duration -= sleep_time
                schedule.run_pending()

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