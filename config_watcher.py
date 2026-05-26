"""
Configuration File Watcher

Watches azan.toml for changes and hot-reloads settings into the running
scheduler without a restart.
"""

import hashlib
import logging
import os
import time
from datetime import datetime
from pathlib import Path
from typing import Any, Callable, Dict, Optional

import schedule
from watchdog.events import FileSystemEventHandler
from watchdog.observers import Observer

from settings import settings, _find_config_file

logger = logging.getLogger(__name__)


class _FileWatcher(FileSystemEventHandler):
    """Debounced filesystem event handler for a single file."""

    def __init__(self, config_path: Path, callback: Callable, debounce: float = 2.0):
        self.config_path = config_path.resolve()
        self.callback = callback
        self.debounce = debounce
        self._last_reload = 0.0
        self._last_hash = self._hash()

    def _hash(self) -> str:
        try:
            return hashlib.md5(self.config_path.read_bytes()).hexdigest()
        except Exception:
            return ""

    def on_modified(self, event):
        if not str(event.src_path).endswith(self.config_path.name):
            return
        new_hash = self._hash()
        if new_hash == self._last_hash:
            return
        now = time.time()
        if now - self._last_reload < self.debounce:
            return
        self._last_reload = now
        self._last_hash = new_hash
        logger.info("Config file changed — reloading")
        try:
            self.callback()
        except Exception as exc:
            logger.error("Error in config reload callback: %s", exc, exc_info=True)

    @property
    def last_reload_time(self) -> float:
        return self._last_reload

    @property
    def last_hash(self) -> str:
        return self._last_hash


class ConfigWatcher:
    """
    Watches azan.toml and propagates changes to the live scheduler.
    """

    def __init__(self, scheduler):
        self.scheduler = scheduler
        self.observer: Optional[Observer] = None
        self._file_watcher: Optional[_FileWatcher] = None

    def start(self) -> Dict[str, Any]:
        if self.observer and self.observer.is_alive():
            return {"success": False, "message": "Already watching", "timestamp": datetime.now().isoformat()}

        watch_path = _find_config_file() or Path("azan.toml")

        try:
            self._file_watcher = _FileWatcher(watch_path, self._on_change)
            self.observer = Observer()
            watch_dir = str(watch_path.resolve().parent)
            os.makedirs(watch_dir, exist_ok=True)
            self.observer.schedule(self._file_watcher, watch_dir, recursive=False)
            self.observer.start()
            logger.info("Watching config file: %s", watch_path)
            return {
                "success": True,
                "message": "Config watcher started",
                "watching": str(watch_path),
                "timestamp": datetime.now().isoformat(),
            }
        except Exception as exc:
            logger.error("Failed to start config watcher: %s", exc)
            return {"success": False, "error": str(exc), "timestamp": datetime.now().isoformat()}

    def stop(self) -> Dict[str, Any]:
        try:
            if self.observer and self.observer.is_alive():
                self.observer.stop()
                self.observer.join(timeout=5)
                return {"success": True, "message": "Config watcher stopped", "timestamp": datetime.now().isoformat()}
            return {"success": False, "message": "Not running", "timestamp": datetime.now().isoformat()}
        except Exception as exc:
            return {"success": False, "error": str(exc), "timestamp": datetime.now().isoformat()}

    def _on_change(self):
        """Reload settings and apply any changes to the live scheduler."""
        old_location = settings.prayer.location
        old_speaker = settings.speaker.group_name
        old_pre_fajr = settings.prayer.pre_fajr_enabled
        old_friday_kahf = settings.prayer.friday_kahf_enabled

        settings.reload()

        if settings.prayer.location != old_location:
            self._apply_location_change(old_location, settings.prayer.location)

        if settings.speaker.group_name != old_speaker:
            self._apply_speaker_change(old_speaker, settings.speaker.group_name)

        if settings.prayer.pre_fajr_enabled != old_pre_fajr:
            self._apply_pre_fajr_change(settings.prayer.pre_fajr_enabled)

        if settings.prayer.friday_kahf_enabled != old_friday_kahf:
            self._apply_friday_kahf_change(settings.prayer.friday_kahf_enabled)

    def _apply_location_change(self, old: str, new: str):
        logger.info("Location changed: %s → %s", old, new)
        schedule.clear()
        self.scheduler.location = new
        if hasattr(self.scheduler.fetcher, "_cache"):
            self.scheduler.fetcher._cache.clear()
        result = self.scheduler.load_prayer_times()
        if result.get("success"):
            schedule_result = self.scheduler.schedule_prayers()
            if schedule_result.get("success"):
                logger.info("Location change applied — %d prayers rescheduled", schedule_result.get("scheduled_count", 0))
            else:
                logger.error("Reschedule after location change failed: %s", schedule_result.get("error"))
        else:
            logger.error("Prayer times load failed for new location %s: %s", new, result.get("error"))

    def _apply_speaker_change(self, old: str, new: str):
        logger.info("Speaker changed: %s → %s", old, new)
        if hasattr(self.scheduler, "google_device"):
            self.scheduler.google_device = new
        cm = getattr(self.scheduler, "chromecast_manager", None)
        if cm:
            if hasattr(cm, "target_device"):
                cm.target_device = None
            if hasattr(cm, "last_discovery_time"):
                cm.last_discovery_time = 0

    def _apply_pre_fajr_change(self, enabled: bool):
        logger.info("Pre-Fajr Quran %s", "enabled" if enabled else "disabled")
        if hasattr(self.scheduler, "toggle_pre_fajr_quran"):
            result = self.scheduler.toggle_pre_fajr_quran(enabled)
            if not result.get("success"):
                logger.error("Failed to toggle pre-Fajr: %s", result.get("error"))
        else:
            schedule.clear()
            self.scheduler.schedule_prayers()

    def _apply_friday_kahf_change(self, enabled: bool):
        logger.info("Friday Surah Al-Kahf %s", "enabled" if enabled else "disabled")
        if hasattr(self.scheduler, "toggle_friday_kahf"):
            result = self.scheduler.toggle_friday_kahf(enabled)
            if not result.get("success"):
                logger.error("Failed to toggle Friday Al-Kahf: %s", result.get("error"))

    def get_status(self) -> Dict[str, Any]:
        is_running = bool(self.observer and self.observer.is_alive())
        return {
            "success": True,
            "running": is_running,
            "config_file": str(_find_config_file() or "azan.toml"),
            "last_reload": (
                datetime.fromtimestamp(self._file_watcher.last_reload_time).isoformat()
                if self._file_watcher and self._file_watcher.last_reload_time > 0
                else None
            ),
            "current_hash": self._file_watcher.last_hash if self._file_watcher else None,
            "timestamp": datetime.now().isoformat(),
        }
