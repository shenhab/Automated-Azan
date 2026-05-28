import time
import json
import os
import random
import logging
import threading
from datetime import datetime, timedelta
from dateutil import tz
from apscheduler.schedulers.background import BackgroundScheduler
from apscheduler.triggers.date import DateTrigger
from apscheduler.triggers.cron import CronTrigger
from apscheduler.jobstores.base import JobLookupError
from prayer_times_fetcher import PrayerTimesFetcher
from chromecast_manager import ChromecastManager
from time_sync import update_ntp_time


_PRAYER_JOB_IDS = {'fajr', 'dhuhr', 'asr', 'maghrib', 'isha', 'pre_fajr', 'friday_kahf'}

_LABEL_MAP = {
    "fajr":         ("Fajr Athan",          "fajr_athan"),
    "dhuhr":        ("Dhuhr Athan",          "regular_athan"),
    "asr":          ("Asr Athan",            "regular_athan"),
    "maghrib":      ("Maghrib Athan",        "regular_athan"),
    "isha":         ("Isha Athan",           "regular_athan"),
    "pre_fajr":     ("Pre-Fajr Quran",       "quran_recitation"),
    "friday_kahf":  ("Friday Surah Al-Kahf", "friday_kahf"),
}


class AthanScheduler:
    """
    Schedules Athan playback and Quran recitations using APScheduler.
    All methods return JSON-compatible dicts for API compatibility.
    """

    def __init__(self, location="icci", google_device="athan"):
        self.location = location
        self.google_device = google_device
        self.fetcher = PrayerTimesFetcher()
        self.prayer_times = {}
        self.tz = tz.gettz('Europe/Dublin')
        self.chromecast_manager = ChromecastManager(google_device_name=self.google_device)
        self.skipped_prayers = []
        self._today_jobs = []  # snapshot of scheduled jobs for the current day

        self._scheduler = BackgroundScheduler(timezone=self.tz)

        logging.info("[DEBUG] Initializing scheduler - loading prayer times and scheduling jobs")
        init_result = self.load_prayer_times()
        logging.info(f"[DEBUG] Prayer times load result: {init_result}")

        if init_result.get('success', False):
            logging.info("[DEBUG] Prayer times loaded successfully, now scheduling prayers")
            schedule_result = self.schedule_prayers()
            logging.info(f"[DEBUG] Prayer scheduling result: {schedule_result}")
        else:
            logging.warning("Failed to load prayer times during initialization")

        status = self.get_scheduler_status()
        logging.info(f"[DEBUG] Jobs scheduled during init: {status.get('total_jobs', 0)}")

    # ------------------------------------------------------------------ #
    #  Prayer time helpers                                                  #
    # ------------------------------------------------------------------ #

    def get_next_prayer_time(self):
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

    def get_prayer_times(self):
        return {
            "success": True,
            "location": self.location,
            "prayer_times": self.prayer_times,
            "timezone": str(self.tz),
            "timestamp": datetime.now(self.tz).isoformat()
        }

    # ------------------------------------------------------------------ #
    #  Scheduling                                                           #
    # ------------------------------------------------------------------ #

    def schedule_prayers(self):
        """Remove today's prayer jobs and reschedule from self.prayer_times."""
        try:
            # Remove only prayer-type jobs; keep 'daily_refresh'
            for job_id in _PRAYER_JOB_IDS:
                try:
                    self._scheduler.remove_job(job_id)
                except JobLookupError:
                    pass

            now = datetime.now(self.tz)
            scheduled_prayers = []
            skipped_prayers = []
            today_jobs = []

            logging.info("Scheduling today's prayers. Current time: %s", now.strftime("%Y-%m-%d %H:%M:%S"))
            logging.info(f"[DEBUG] Available prayer times: {self.prayer_times}")

            from settings import settings
            pre_fajr_enabled = settings.prayer.pre_fajr_enabled
            friday_kahf_enabled = settings.prayer.friday_kahf_enabled
            cm = self.chromecast_manager

            for prayer, time_tuple in self.prayer_times.items():
                logging.debug("Processing prayer: %s, scheduled time: %s", prayer, time_tuple)
                try:
                    hour, minute = map(int, time_tuple.split(":"))
                    prayer_time = datetime(now.year, now.month, now.day, hour, minute, tzinfo=self.tz)

                    if prayer_time > now:
                        formatted_time = prayer_time.strftime("%H:%M")
                        logging.info("Scheduling %s at %s", prayer, formatted_time)

                        if prayer == "Fajr":
                            self._scheduler.add_job(
                                cm.start_adahn_alfajr,
                                trigger=DateTrigger(run_date=prayer_time),
                                id='fajr',
                                replace_existing=True,
                                misfire_grace_time=120,
                            )
                            scheduled_prayers.append({"prayer": prayer, "time": formatted_time, "type": "fajr_athan"})
                            today_jobs.append({
                                "label": "Fajr Athan", "type": "fajr_athan",
                                "tags": ["fajr"], "scheduled_time": formatted_time,
                                "run_date": prayer_time,
                            })

                            if pre_fajr_enabled:
                                pre_fajr_result = self.schedule_pre_fajr_quran_for_time(prayer_time)
                                if pre_fajr_result.get('success'):
                                    scheduled_prayers.append({
                                        "prayer": "Pre-Fajr Quran",
                                        "time": pre_fajr_result['scheduled_time'],
                                        "type": "quran_recitation",
                                    })
                                    today_jobs.append({
                                        "label": "Pre-Fajr Quran", "type": "quran_recitation",
                                        "tags": ["pre_fajr"],
                                        "scheduled_time": pre_fajr_result['scheduled_time'],
                                        "run_date": pre_fajr_result['run_date'],
                                    })
                                else:
                                    logging.warning(f"Failed to schedule pre-Fajr Quran: {pre_fajr_result.get('error')}")

                        elif prayer == "Dhuhr":
                            self._scheduler.add_job(
                                cm.start_adahn,
                                trigger=DateTrigger(run_date=prayer_time),
                                id='dhuhr',
                                replace_existing=True,
                                misfire_grace_time=120,
                            )
                            scheduled_prayers.append({"prayer": prayer, "time": formatted_time, "type": "regular_athan"})
                            today_jobs.append({
                                "label": "Dhuhr Athan", "type": "regular_athan",
                                "tags": ["dhuhr"], "scheduled_time": formatted_time,
                                "run_date": prayer_time,
                            })

                            if friday_kahf_enabled and now.weekday() == 4:
                                kahf_result = self.schedule_friday_kahf_for_time(prayer_time)
                                if kahf_result.get('success'):
                                    scheduled_prayers.append({
                                        "prayer": "Friday Surah Al-Kahf",
                                        "time": kahf_result['scheduled_time'],
                                        "type": "friday_kahf",
                                    })
                                    today_jobs.append({
                                        "label": "Friday Surah Al-Kahf", "type": "friday_kahf",
                                        "tags": ["friday_kahf"],
                                        "scheduled_time": kahf_result['scheduled_time'],
                                        "run_date": kahf_result['run_date'],
                                    })
                                else:
                                    logging.warning(f"Failed to schedule Friday Surah Al-Kahf: {kahf_result.get('error')}")

                        else:
                            prayer_id = prayer.lower()
                            self._scheduler.add_job(
                                cm.start_adahn,
                                trigger=DateTrigger(run_date=prayer_time),
                                id=prayer_id,
                                replace_existing=True,
                                misfire_grace_time=120,
                            )
                            label = _LABEL_MAP.get(prayer_id, (f"{prayer} Athan", "regular_athan"))[0]
                            type_ = _LABEL_MAP.get(prayer_id, (f"{prayer} Athan", "regular_athan"))[1]
                            scheduled_prayers.append({"prayer": prayer, "time": formatted_time, "type": type_})
                            today_jobs.append({
                                "label": label, "type": type_,
                                "tags": [prayer_id], "scheduled_time": formatted_time,
                                "run_date": prayer_time,
                            })

                        logging.debug("%s scheduled successfully at %s", prayer, formatted_time)
                    else:
                        skipped_prayers.append({"prayer": prayer, "time": time_tuple, "reason": "Past time"})
                        logging.info("Skipping %s at %s as it's in the past.", prayer, time_tuple)

                except ValueError as e:
                    logging.error("Error parsing time for prayer %s: %s", prayer, e)
                    skipped_prayers.append({"prayer": prayer, "time": time_tuple, "reason": f"Parse error: {str(e)}"})

            self.skipped_prayers = skipped_prayers
            self._today_jobs = today_jobs
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
            return {"success": False, "error": str(e), "current_time": datetime.now(self.tz).isoformat()}

    def get_scheduler_status(self):
        try:
            now = datetime.now(self.tz)

            # Jobs still pending in APScheduler (DateTrigger jobs disappear after firing)
            active_ids = {job.id for job in self._scheduler.get_jobs()}

            upcoming = []
            done_today = []

            for job_info in self._today_jobs:
                tags = job_info.get('tags', [])
                job_id = tags[0] if tags else ''
                is_done = job_id not in active_ids
                run_date = job_info.get('run_date')

                entry = {
                    "label": job_info['label'],
                    "type": job_info['type'],
                    "tags": tags,
                    "scheduled_time": job_info['scheduled_time'],
                    "status": "done" if is_done else "upcoming",
                    "next_run": None if is_done else (run_date.isoformat() if isinstance(run_date, datetime) else None),
                }
                (done_today if is_done else upcoming).append(entry)

            # Add prayers that were already past when schedule_prayers() last ran
            for s in getattr(self, 'skipped_prayers', []):
                prayer_key = s.get('prayer', '').lower()
                label, type_ = _LABEL_MAP.get(prayer_key, (s.get('prayer', 'Unknown'), 'unknown'))
                done_today.append({
                    "label": label, "type": type_,
                    "tags": [prayer_key], "status": "done",
                    "scheduled_time": s.get('time'), "next_run": None,
                })

            done_today.sort(key=lambda j: j['scheduled_time'] or '')
            upcoming.sort(key=lambda j: j['next_run'] or '')
            all_jobs = done_today + upcoming

            prayer_jobs = [j for j in self._scheduler.get_jobs() if j.id != 'daily_refresh']
            next_run = min(
                (j.next_run_time for j in prayer_jobs if j.next_run_time),
                default=None
            )

            return {
                "success": True,
                "running": self._scheduler.running,
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

    # ------------------------------------------------------------------ #
    #  Main loop                                                            #
    # ------------------------------------------------------------------ #

    def run_scheduler(self):
        """Start APScheduler and block forever. All prayer jobs run in background threads."""
        logging.info("Starting APScheduler-based Athan scheduler.")

        self._scheduler.start()
        logging.info("APScheduler started. Daily refresh scheduled at 01:00.")

        self._scheduler.add_job(
            self._daily_refresh,
            trigger=CronTrigger(hour=1, minute=0, timezone=self.tz),
            id='daily_refresh',
            replace_existing=True,
        )

        while True:
            try:
                time.sleep(60)
            except Exception as e:
                logging.error("Scheduler main loop error: %s", e, exc_info=True)
                time.sleep(60)

    def _daily_refresh(self):
        """Runs at 01:00 each day to reload prayer times and reschedule jobs."""
        logging.info("Daily refresh triggered at 01:00.")
        dst_result = self.check_dst_change()
        if dst_result.get('dst_changed'):
            logging.warning("DST CHANGE DETECTED: %s", dst_result.get('message', ''))
        self.update_ntp_time()
        self.refresh_schedule()

    # ------------------------------------------------------------------ #
    #  NTP / DST / refresh                                                  #
    # ------------------------------------------------------------------ #

    def update_ntp_time(self):
        try:
            result = update_ntp_time()
            return {
                "success": True,
                "sync_result": result,
                "timestamp": datetime.now(self.tz).isoformat(),
                "message": "NTP time synchronization completed"
            }
        except Exception as e:
            logging.error(f"Error during NTP time sync: {e}")
            return {"success": False, "error": str(e), "timestamp": datetime.now(self.tz).isoformat()}

    def execute_prayer_athan(self, prayer_name):
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

    def check_dst_change(self):
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
                try:
                    metadata_file = self.fetcher._get_dst_metadata_file(self.location)
                    if os.path.exists(metadata_file):
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
        try:
            logging.info("Refreshing prayer times for a new day.")

            load_result = self.load_prayer_times()
            if not load_result.get('success', False):
                return {
                    "success": False,
                    "stage": "load_prayer_times",
                    "error": load_result.get('error', 'Unknown error'),
                    "timestamp": datetime.now(self.tz).isoformat()
                }

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
            return {"success": False, "error": str(e), "timestamp": datetime.now(self.tz).isoformat()}

    # ------------------------------------------------------------------ #
    #  Pre-Fajr Quran                                                       #
    # ------------------------------------------------------------------ #

    def schedule_pre_fajr_quran(self):
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
        try:
            from settings import settings
            offset = settings.prayer.pre_fajr_minutes
            pre_fajr_dt = fajr_time - timedelta(minutes=offset)
            now = datetime.now(self.tz)

            if pre_fajr_dt <= now:
                return {"success": False, "error": "Pre-Fajr time is already past"}

            self._scheduler.add_job(
                self.play_pre_fajr_quran,
                trigger=DateTrigger(run_date=pre_fajr_dt),
                id='pre_fajr',
                replace_existing=True,
                misfire_grace_time=120,
            )

            pre_fajr_time = pre_fajr_dt.strftime("%H:%M")
            logging.info(
                f"Scheduled pre-Fajr Quran at {pre_fajr_time} "
                f"({offset} min before Fajr at {fajr_time.strftime('%H:%M')})"
            )
            return {
                "success": True,
                "scheduled_time": pre_fajr_time,
                "fajr_time": fajr_time.strftime('%H:%M'),
                "offset_minutes": offset,
                "run_date": pre_fajr_dt,
            }
        except Exception as e:
            logging.error(f"Failed to schedule pre-Fajr Quran: {e}")
            return {"success": False, "error": str(e)}

    def play_pre_fajr_quran(self):
        from settings import settings

        quran_url = "https://backup.qurango.net/radio/mahmoud_khalil_alhussary_warsh"
        duration_seconds = settings.prayer.pre_fajr_minutes * 60

        if hasattr(self, '_pre_fajr_stop_event') and self._pre_fajr_stop_event:
            self._pre_fajr_stop_event.set()

        stop_event = threading.Event()
        self._pre_fajr_stop_event = stop_event
        pre_fajr_speaker = settings.speaker.resolve("pre_fajr")

        t = threading.Thread(
            target=self._run_quran_stream,
            args=(quran_url, duration_seconds, stop_event, pre_fajr_speaker),
            daemon=True,
            name="pre-fajr-quran",
        )
        t.start()

        logging.info(
            f"Pre-Fajr Quran stream started: {settings.prayer.pre_fajr_minutes}-minute window"
        )
        return {
            "success": True,
            "duration_minutes": settings.prayer.pre_fajr_minutes,
            "message": f"Pre-Fajr Quran stream started ({settings.prayer.pre_fajr_minutes} min window)",
            "timestamp": datetime.now().isoformat(),
        }

    def _run_quran_stream(self, url, duration_seconds, stop_event, speaker_override=None):
        """Background thread: play the Quran stream until the window closes or stop is requested."""
        logging.info(f"Pre-Fajr Quran: starting stream {url}")
        result = self.chromecast_manager.play_url_on_cast(
            url,
            speaker_override=speaker_override,
            media_title="تلاوة ما قبل الفجر",
            media_artist="محمود خليل الحصري - رواية ورش",
            stream_type="LIVE",
        )
        if not result.get('success'):
            logging.warning(f"Pre-Fajr Quran: failed to start stream: {result.get('error')}")
            return

        # Wait for the pre-fajr window to elapse or stop to be requested
        end_time = time.time() + duration_seconds
        while time.time() < end_time and not stop_event.is_set():
            time.sleep(10)

        logging.info("Pre-Fajr Quran stream window ended")

    # Keep legacy name for callers that reference _run_quran_playlist
    def _run_quran_playlist(self, urls, duration_seconds, stop_event, speaker_override=None):
        self._run_quran_stream(urls[0] if urls else "", duration_seconds, stop_event, speaker_override)

    def _wait_for_track_end(self, end_time, stop_event, poll_interval=5):
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
        try:
            if hasattr(self, '_pre_fajr_stop_event') and self._pre_fajr_stop_event:
                self._pre_fajr_stop_event.set()
                self._pre_fajr_stop_event = None

            removed = 0
            try:
                self._scheduler.remove_job('pre_fajr')
                removed = 1
            except JobLookupError:
                pass

            self._today_jobs = [j for j in self._today_jobs if 'pre_fajr' not in j.get('tags', [])]

            logging.info(f"Cancelled {removed} pre-Fajr Quran schedule(s)")
            return {
                "success": True,
                "message": f"Pre-Fajr Quran cancelled ({removed} job(s) removed)",
                "jobs_removed": removed,
            }
        except Exception as e:
            logging.error(f"Error cancelling pre-Fajr Quran: {e}")
            return {"success": False, "error": str(e)}

    def toggle_pre_fajr_quran(self, enable):
        try:
            if enable:
                self.cancel_pre_fajr_quran()
                result = self.schedule_pre_fajr_quran()
                if result.get('success'):
                    # Keep _today_jobs in sync
                    from settings import settings
                    offset = settings.prayer.pre_fajr_minutes
                    if 'Fajr' in self.prayer_times:
                        fajr_time_str = self.prayer_times['Fajr']
                        hour, minute = map(int, fajr_time_str.split(':'))
                        now = datetime.now(self.tz)
                        fajr_dt = datetime(now.year, now.month, now.day, hour, minute, tzinfo=self.tz)
                        pre_fajr_dt = fajr_dt - timedelta(minutes=offset)
                        self._today_jobs.append({
                            "label": "Pre-Fajr Quran", "type": "quran_recitation",
                            "tags": ["pre_fajr"],
                            "scheduled_time": result.get('scheduled_time'),
                            "run_date": pre_fajr_dt,
                        })
                    return {
                        "success": True,
                        "enabled": True,
                        "message": f"Pre-Fajr Quran enabled at {result.get('scheduled_time')}"
                    }
                return result
            else:
                result = self.cancel_pre_fajr_quran()
                if result.get('success'):
                    return {"success": True, "enabled": False, "message": "Pre-Fajr Quran disabled"}
                return result
        except Exception as e:
            logging.error(f"Error toggling pre-Fajr Quran: {e}")
            return {"success": False, "error": str(e)}

    # ------------------------------------------------------------------ #
    #  Friday Surah Al-Kahf                                                 #
    # ------------------------------------------------------------------ #

    def schedule_friday_kahf_for_time(self, dhuhr_time):
        try:
            kahf_dt = dhuhr_time - timedelta(hours=2)
            now = datetime.now(self.tz)

            if kahf_dt <= now:
                return {"success": False, "error": "Friday Kahf time is already past"}

            self._scheduler.add_job(
                self.play_friday_kahf,
                trigger=DateTrigger(run_date=kahf_dt),
                id='friday_kahf',
                replace_existing=True,
                misfire_grace_time=120,
            )

            kahf_time = kahf_dt.strftime("%H:%M")
            logging.info(
                f"Scheduled Friday Surah Al-Kahf at {kahf_time} "
                f"(2 hours before Dhuhr at {dhuhr_time.strftime('%H:%M')})"
            )
            return {
                "success": True,
                "scheduled_time": kahf_time,
                "dhuhr_time": dhuhr_time.strftime('%H:%M'),
                "run_date": kahf_dt,
            }
        except Exception as e:
            logging.error(f"Failed to schedule Friday Surah Al-Kahf: {e}")
            return {"success": False, "error": str(e)}

    def play_friday_kahf(self):
        from settings import settings
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
        try:
            removed = 0
            try:
                self._scheduler.remove_job('friday_kahf')
                removed = 1
            except JobLookupError:
                pass

            self._today_jobs = [j for j in self._today_jobs if 'friday_kahf' not in j.get('tags', [])]

            logging.info(f"Cancelled {removed} Friday Al-Kahf schedule(s)")
            return {
                "success": True,
                "message": f"Friday Al-Kahf cancelled ({removed} job(s) removed)",
                "jobs_removed": removed,
            }
        except Exception as e:
            logging.error(f"Error cancelling Friday Al-Kahf: {e}")
            return {"success": False, "error": str(e)}

    def toggle_friday_kahf(self, enable):
        try:
            self.cancel_friday_kahf()
            if enable:
                result = self.schedule_friday_kahf_for_time(self._dhuhr_as_datetime())
                if result.get('success'):
                    self._today_jobs.append({
                        "label": "Friday Surah Al-Kahf", "type": "friday_kahf",
                        "tags": ["friday_kahf"],
                        "scheduled_time": result.get('scheduled_time'),
                        "run_date": result.get('run_date'),
                    })
                    return {"success": True, "enabled": True,
                            "message": f"Friday Al-Kahf enabled at {result.get('scheduled_time')}"}
                return result
            return {"success": True, "enabled": False, "message": "Friday Al-Kahf disabled"}
        except Exception as e:
            logging.error(f"Error toggling Friday Al-Kahf: {e}")
            return {"success": False, "error": str(e)}

    def _dhuhr_as_datetime(self):
        dhuhr_str = self.prayer_times.get('Dhuhr')
        if not dhuhr_str:
            raise ValueError("Dhuhr prayer time not loaded")
        hour, minute = map(int, dhuhr_str.split(':'))
        now = datetime.now(self.tz)
        return datetime(now.year, now.month, now.day, hour, minute, tzinfo=self.tz)

    # ------------------------------------------------------------------ #
    #  Compat stub                                                          #
    # ------------------------------------------------------------------ #

    def sleep_until_next_1am(self):
        """No-op: kept for API compatibility. Sleeping is handled by APScheduler."""
        return {"success": True, "message": "Handled by APScheduler daily refresh"}
