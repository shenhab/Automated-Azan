"""
Configuration File Watcher Module

This module provides real-time configuration file monitoring and automatic
reload capabilities for the Automated Azan application.
"""

import asyncio
import os
import time
import logging
import hashlib
from pathlib import Path
from watchdog.observers import Observer
from watchdog.events import FileSystemEventHandler
from typing import Optional, Dict, Any, List, Callable
from datetime import datetime
import schedule

logger = logging.getLogger(__name__)


class ConfigFileWatcher(FileSystemEventHandler):
    """
    Watches configuration file for changes and triggers reload.
    """

    def __init__(self, config_path: str, callback: Callable, debounce_seconds: float = 1.0):
        """
        Initialize the config watcher.

        Args:
            config_path: Path to the configuration file
            callback: Function to call when config changes
            debounce_seconds: Minimum time between reloads to prevent rapid firing
        """
        self.config_path = Path(config_path).resolve()
        self.callback = callback
        self.debounce_seconds = debounce_seconds
        self.last_reload_time = 0
        self.last_hash = self._get_file_hash()

    def _get_file_hash(self) -> str:
        """Calculate MD5 hash of the config file."""
        try:
            with open(self.config_path, 'rb') as f:
                return hashlib.md5(f.read()).hexdigest()
        except Exception as e:
            logger.error(f"Failed to hash config file: {e}")
            return ""

    def on_modified(self, event):
        """Handle file modification events."""
        if not event.src_path.endswith(self.config_path.name):
            return

        current_hash = self._get_file_hash()
        if current_hash == self.last_hash:
            logger.debug("File modified but content unchanged, skipping reload")
            return

        # Debounce rapid changes
        current_time = time.time()
        if current_time - self.last_reload_time < self.debounce_seconds:
            logger.debug("Debouncing config reload (too soon after last reload)")
            return

        self.last_reload_time = current_time
        self.last_hash = current_hash

        logger.info(f"Config file changed, triggering reload")

        # Try async callback first, fall back to sync
        try:
            loop = asyncio.get_event_loop()
            if loop.is_running():
                asyncio.create_task(self.callback())
            else:
                # Run synchronously if no event loop
                if asyncio.iscoroutinefunction(self.callback):
                    asyncio.run(self.callback())
                else:
                    self.callback()
        except RuntimeError:
            # No event loop, use sync callback
            if asyncio.iscoroutinefunction(self.callback):
                # Create new event loop for async callback
                asyncio.run(self.callback())
            else:
                self.callback()


class ConfigWatcher:
    """
    Main configuration watcher that handles config changes and updates components.
    """

    def __init__(self, config_manager, scheduler):
        """
        Initialize the config watcher.

        Args:
            config_manager: ConfigManager instance
            scheduler: AthanScheduler instance
        """
        self.config_manager = config_manager
        self.scheduler = scheduler
        self.observer = None
        self.previous_config = {}
        self.file_watcher = None

    def start(self):
        """
        Start watching config file.

        Returns:
            dict: Status of the watch operation
        """
        try:
            if self.observer and self.observer.is_alive():
                return {
                    "success": False,
                    "message": "Already watching",
                    "timestamp": datetime.now().isoformat()
                }

            # Get initial config snapshot
            self.previous_config = self.config_manager.get_config_snapshot()

            # Determine which config file to watch (prioritize writable locations)
            config_paths_to_watch = [
                '/app/config/adahn.config',  # Docker volume location (writable)
                'config/adahn.config',       # Local config directory
                self.config_manager.config_file  # Original config file
            ]

            watch_file = None
            for config_path in config_paths_to_watch:
                if os.path.exists(config_path):
                    watch_file = config_path
                    logger.info(f"Found existing config file to watch: {config_path}")
                    break

            # If no config exists, watch the primary writable location where saves will happen
            if not watch_file:
                watch_file = '/app/config/adahn.config'
                logger.info(f"No existing config found, will watch primary save location: {watch_file}")

            # Create file watcher
            self.file_watcher = ConfigFileWatcher(
                watch_file,
                self.handle_config_change,
                debounce_seconds=2.0
            )

            # Setup observer
            self.observer = Observer()
            watch_dir = str(Path(watch_file).parent.resolve())

            # Ensure watch directory exists
            os.makedirs(watch_dir, exist_ok=True)

            self.observer.schedule(self.file_watcher, watch_dir, recursive=False)
            self.observer.start()

            logger.info(f"Started watching config: {watch_file}")
            return {
                "success": True,
                "message": "Config watcher started",
                "watching": watch_file,
                "timestamp": datetime.now().isoformat()
            }

        except Exception as e:
            logger.error(f"Failed to start config watcher: {e}")
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def stop(self):
        """
        Stop watching config file.

        Returns:
            dict: Status of stop operation
        """
        try:
            if self.observer and self.observer.is_alive():
                self.observer.stop()
                self.observer.join(timeout=5)
                logger.info("Config watcher stopped")
                return {
                    "success": True,
                    "message": "Config watcher stopped",
                    "timestamp": datetime.now().isoformat()
                }
            else:
                return {
                    "success": False,
                    "message": "Not watching",
                    "timestamp": datetime.now().isoformat()
                }
        except Exception as e:
            logger.error(f"Error stopping config watcher: {e}")
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    async def handle_config_change(self):
        """Handle config file changes asynchronously."""
        try:
            logger.info("Handling configuration change...")

            # Reload config
            result = self.config_manager.reload_config()
            if not result.get('success'):
                logger.error(f"Failed to reload config: {result.get('error')}")
                return

            # Get new config
            new_config = self.config_manager.get_config_snapshot()

            # Detect changes
            changes = self.config_manager.detect_changes(self.previous_config, new_config)

            if not changes:
                logger.info("No actual configuration changes detected")
                return

            logger.info(f"Configuration changes detected: {changes}")

            # Handle changes
            for section, items in changes.items():
                if section == 'Settings':
                    for key, (old_val, new_val) in items.items():
                        await self._handle_setting_change(key, old_val, new_val)

            # Update stored config
            self.previous_config = new_config

            logger.info("Configuration changes applied successfully")

        except Exception as e:
            logger.error(f"Error handling config change: {e}", exc_info=True)

    async def _handle_setting_change(self, key: str, old_val: str, new_val: str):
        """
        Handle specific setting changes.

        Args:
            key: Setting key that changed
            old_val: Previous value
            new_val: New value
        """
        logger.info(f"Config change: {key}: '{old_val}' → '{new_val}'")

        handlers = {
            'location': self._handle_location_change,
            'speakers-group-name': self._handle_speaker_change,
            'pre_fajr_enabled': self._handle_pre_fajr_change,
            'prayer_source': self._handle_prayer_source_change,
        }

        handler = handlers.get(key)
        if handler:
            try:
                await handler(old_val, new_val)
                logger.info(f"Successfully handled change for {key}")
            except Exception as e:
                logger.error(f"Failed to handle change for {key}: {e}", exc_info=True)
        else:
            logger.debug(f"No handler for config key: {key}")

    async def _handle_location_change(self, old_val: str, new_val: str):
        """Handle location changes."""
        try:
            logger.info(f"Handling location change from {old_val} to {new_val}")

            # Clear all scheduled jobs
            schedule.clear()
            logger.info("Cleared all scheduled jobs")

            # Update location
            self.scheduler.location = new_val

            # Clear prayer times cache
            if hasattr(self.scheduler.fetcher, '_cache'):
                self.scheduler.fetcher._cache.clear()
                logger.info("Cleared prayer times cache")

            # Reload prayer times
            result = self.scheduler.load_prayer_times()

            if result.get('success'):
                # Reschedule prayers with new times
                schedule_result = self.scheduler.schedule_prayers()
                if schedule_result.get('success'):
                    logger.info(f"Location changed to {new_val}, {schedule_result.get('scheduled_count', 0)} prayers rescheduled")
                else:
                    logger.error(f"Failed to reschedule prayers: {schedule_result.get('error')}")
            else:
                logger.error(f"Failed to load prayer times for new location: {result.get('error')}")

        except Exception as e:
            logger.error(f"Error handling location change: {e}", exc_info=True)

    async def _handle_speaker_change(self, old_val: str, new_val: str):
        """Handle speaker group changes."""
        try:
            logger.info(f"Handling speaker change from {old_val} to {new_val}")

            # Disconnect from current speakers
            if hasattr(self.scheduler.chromecast_manager, 'disconnect_all'):
                self.scheduler.chromecast_manager.disconnect_all()
                logger.info(f"Disconnected from {old_val}")

            # Update device name
            self.scheduler.google_device = new_val
            self.scheduler.chromecast_manager.device_name = new_val

            # Clear device cache if exists
            if hasattr(self.scheduler.chromecast_manager, 'chromecasts'):
                self.scheduler.chromecast_manager.chromecasts = []
            if hasattr(self.scheduler.chromecast_manager, 'selected_device'):
                self.scheduler.chromecast_manager.selected_device = None

            # Attempt to connect to new speaker group
            result = self.scheduler.chromecast_manager.connect_to_device(new_val)

            if result.get('success'):
                logger.info(f"Successfully connected to new speaker group: {new_val}")
            else:
                logger.warning(f"Failed to connect to new speaker group {new_val}: {result.get('error')}")
                logger.info("Will retry connection on next prayer time")

        except Exception as e:
            logger.error(f"Error handling speaker change: {e}", exc_info=True)

    async def _handle_pre_fajr_change(self, old_val: str, new_val: str):
        """Handle pre-Fajr setting changes."""
        try:
            # Parse boolean values
            is_enabled = new_val.lower() in ['true', '1', 'yes', 'on'] if new_val else False
            was_enabled = old_val.lower() in ['true', '1', 'yes', 'on'] if old_val else False

            logger.info(f"Handling pre-Fajr change: was_enabled={was_enabled}, is_enabled={is_enabled}")

            if is_enabled != was_enabled:
                if hasattr(self.scheduler, 'toggle_pre_fajr_quran'):
                    result = self.scheduler.toggle_pre_fajr_quran(is_enabled)
                    if result.get('success'):
                        logger.info(f"Pre-Fajr Quran {'enabled' if is_enabled else 'disabled'}")
                    else:
                        logger.error(f"Failed to toggle pre-Fajr: {result.get('error')}")
                else:
                    # Fallback: reschedule all prayers
                    schedule.clear()
                    schedule_result = self.scheduler.schedule_prayers()
                    if schedule_result.get('success'):
                        logger.info(f"Pre-Fajr Quran {'enabled' if is_enabled else 'disabled'} (via reschedule)")
                    else:
                        logger.error(f"Failed to reschedule prayers: {schedule_result.get('error')}")

        except Exception as e:
            logger.error(f"Error handling pre-Fajr change: {e}", exc_info=True)

    async def _handle_prayer_source_change(self, old_val: str, new_val: str):
        """Handle prayer source changes."""
        try:
            logger.info(f"Handling prayer source change from {old_val} to {new_val}")

            # Update source preference if configurable
            if hasattr(self.scheduler.fetcher, 'config') and hasattr(self.scheduler.fetcher.config, 'sources'):
                self.scheduler.fetcher.config.sources[self.scheduler.location] = new_val
                logger.info(f"Updated prayer source preference to {new_val}")

            # Clear cache
            if hasattr(self.scheduler.fetcher, '_cache'):
                self.scheduler.fetcher._cache.clear()
                logger.info("Cleared prayer times cache")

            # Reload prayer times from new source
            result = self.scheduler.load_prayer_times()

            if result.get('success'):
                # Check if times changed
                old_times = self.scheduler.prayer_times.copy()
                new_times = result.get('prayer_times', {})

                if old_times != new_times:
                    # Reschedule if times are different
                    schedule.clear()
                    schedule_result = self.scheduler.schedule_prayers()
                    if schedule_result.get('success'):
                        logger.info(f"Prayer source changed to {new_val}, prayers rescheduled")
                    else:
                        logger.error(f"Failed to reschedule prayers: {schedule_result.get('error')}")
                else:
                    logger.info(f"Prayer source changed to {new_val}, times unchanged")
            else:
                logger.error(f"Failed to load prayer times from new source: {result.get('error')}")

        except Exception as e:
            logger.error(f"Error handling prayer source change: {e}", exc_info=True)

    def get_status(self) -> Dict[str, Any]:
        """
        Get the current status of the config watcher.

        Returns:
            dict: Watcher status information
        """
        try:
            is_running = self.observer and self.observer.is_alive()

            status = {
                "success": True,
                "running": is_running,
                "config_file": self.config_manager.config_file if self.config_manager else None,
                "last_reload": datetime.fromtimestamp(
                    self.file_watcher.last_reload_time
                ).isoformat() if self.file_watcher and self.file_watcher.last_reload_time > 0 else None,
                "current_hash": self.file_watcher.last_hash if self.file_watcher else None,
                "timestamp": datetime.now().isoformat()
            }

            if self.previous_config:
                status["monitoring_sections"] = list(self.previous_config.keys())

            return status

        except Exception as e:
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }