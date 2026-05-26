#!/usr/bin/env python3
"""
Web Interface API Helper Module

This module provides JSON API functions for the web interface,
making web functionality available for import into other applications.
All methods return JSON responses for API compatibility.
"""

import os
import json
import logging
from datetime import datetime
from prayer_times_fetcher import PrayerTimesFetcher
from chromecast_manager import ChromecastManager
from settings import settings


class WebInterfaceAPI:
    """
    A class to provide JSON API access to web interface functionality.
    All methods return JSON responses for API compatibility.
    """

    def __init__(self):
        """Initialize the Web Interface API."""
        self.current_config = {}
        self.prayer_times = {}
        self.prayer_times_last_updated = None
        self.discovered_devices = []
        self.cast_manager = None

    def load_config(self):
        """Load current configuration from the settings singleton."""
        try:
            self.current_config = settings.as_web_dict()
            return {
                "success": True,
                "config": self.current_config,
                "message": "Configuration loaded successfully",
                "timestamp": datetime.now().isoformat(),
            }
        except Exception as e:
            logging.error(f"Error loading configuration: {e}")
            return {"success": False, "error": str(e), "timestamp": datetime.now().isoformat()}

    def save_config(self, config_data: dict):
        """
        Save configuration from a dict. Accepts either the new flat web dict
        or a section-keyed dict for backwards compatibility.
        """
        try:
            # Support flat web-dict format {"speakers_group_name": ..., "location": ..., ...}
            updates = {}
            if "speakers_group_name" in config_data:
                updates["speaker"] = {"group_name": config_data["speakers_group_name"]}
            if "location" in config_data:
                updates.setdefault("prayer", {})["location"] = config_data["location"]
            if "pre_fajr_enabled" in config_data:
                updates.setdefault("prayer", {})["pre_fajr_enabled"] = config_data["pre_fajr_enabled"]

            if updates:
                settings.update(**updates)

            path = settings.save()
            self.current_config = settings.as_web_dict()
            return {
                "success": True,
                "config_file": str(path),
                "config": self.current_config,
                "message": f"Configuration saved to {path}",
                "timestamp": datetime.now().isoformat(),
            }
        except Exception as e:
            logging.error(f"Error saving configuration: {e}")
            return {"success": False, "error": str(e), "timestamp": datetime.now().isoformat()}

    def discover_chromecasts(self):
        """
        Discover available Chromecast devices.

        Returns:
            dict: JSON response with discovered devices
        """
        try:
            if not self.cast_manager:
                self.cast_manager = ChromecastManager()

            discovery_result = self.cast_manager.discover_devices(force_rediscovery=True)

            if discovery_result.get('success', False):
                devices_info = self.cast_manager.get_discovered_devices()
                self.discovered_devices = devices_info.get('devices', [])

                return {
                    "success": True,
                    "devices_found": len(self.discovered_devices),
                    "devices": self.discovered_devices,
                    "discovery_method": discovery_result.get('method', 'unknown'),
                    "discovery_result": discovery_result,
                    "message": f"Found {len(self.discovered_devices)} Chromecast devices",
                    "timestamp": datetime.now().isoformat()
                }
            else:
                return {
                    "success": False,
                    "error": "Device discovery failed",
                    "discovery_result": discovery_result,
                    "timestamp": datetime.now().isoformat()
                }

        except Exception as e:
            logging.error(f"Error discovering Chromecasts: {e}")
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def get_prayer_times(self, location="icci", force_refresh=False):
        """
        Get prayer times for the specified location.

        Args:
            location (str): Location to get prayer times for
            force_refresh (bool): Force refresh of prayer times

        Returns:
            dict: JSON response with prayer times
        """
        try:
            # Check if we need to refresh prayer times
            now = datetime.now()
            needs_refresh = (
                force_refresh or
                not self.prayer_times or
                not self.prayer_times_last_updated or
                (now - self.prayer_times_last_updated).days >= 1
            )

            if needs_refresh:
                fetcher = PrayerTimesFetcher()
                fetch_result = fetcher.fetch_prayer_times(location, force_download=force_refresh)

                if fetch_result.get('success', False):
                    self.prayer_times = fetch_result['prayer_times']
                    self.prayer_times_last_updated = now

                    return {
                        "success": True,
                        "prayer_times": self.prayer_times,
                        "location": location,
                        "last_updated": self.prayer_times_last_updated.isoformat(),
                        "fetched_fresh": True,
                        "fetch_result": fetch_result,
                        "message": f"Prayer times fetched successfully for {location}",
                        "timestamp": datetime.now().isoformat()
                    }
                else:
                    return {
                        "success": False,
                        "error": "Failed to fetch prayer times",
                        "location": location,
                        "fetch_result": fetch_result,
                        "timestamp": datetime.now().isoformat()
                    }
            else:
                return {
                    "success": True,
                    "prayer_times": self.prayer_times,
                    "location": location,
                    "last_updated": self.prayer_times_last_updated.isoformat() if self.prayer_times_last_updated else None,
                    "fetched_fresh": False,
                    "message": "Using cached prayer times",
                    "timestamp": datetime.now().isoformat()
                }

        except Exception as e:
            logging.error(f"Error getting prayer times: {e}")
            return {
                "success": False,
                "error": str(e),
                "location": location,
                "timestamp": datetime.now().isoformat()
            }

    def get_next_prayer_info(self):
        """
        Get information about the next prayer.

        Returns:
            dict: JSON response with next prayer information
        """
        try:
            if not self.prayer_times or 'error' in self.prayer_times:
                return {
                    "success": False,
                    "error": "Prayer times not available",
                    "next_prayer": None,
                    "timestamp": datetime.now().isoformat()
                }

            now = datetime.now()
            current_time = now.strftime("%H:%M")

            # Prayer order for comparison
            prayer_order = ['Fajr', 'Dhuhr', 'Asr', 'Maghrib', 'Isha']

            for prayer in prayer_order:
                if prayer in self.prayer_times:
                    prayer_time = self.prayer_times[prayer]
                    if current_time < prayer_time:
                        return {
                            "success": True,
                            "next_prayer": {
                                "name": prayer,
                                "time": prayer_time,
                                "is_today": True
                            },
                            "current_time": current_time,
                            "all_prayers": self.prayer_times,
                            "message": f"Next prayer is {prayer} at {prayer_time}",
                            "timestamp": datetime.now().isoformat()
                        }

            # If no prayer found for today, next is Fajr tomorrow
            fajr_time = self.prayer_times.get('Fajr', 'Unknown')
            return {
                "success": True,
                "next_prayer": {
                    "name": "Fajr",
                    "time": fajr_time,
                    "is_today": False,
                    "display_time": f"{fajr_time} (Tomorrow)"
                },
                "current_time": current_time,
                "all_prayers": self.prayer_times,
                "message": f"Next prayer is Fajr tomorrow at {fajr_time}",
                "timestamp": datetime.now().isoformat()
            }

        except Exception as e:
            logging.error(f"Error calculating next prayer: {e}")
            return {
                "success": False,
                "error": str(e),
                "next_prayer": None,
                "timestamp": datetime.now().isoformat()
            }

    def test_chromecast_device(self, device_uuid):
        """
        Test a specific Chromecast device.

        Args:
            device_uuid (str): UUID of the device to test

        Returns:
            dict: JSON response with test results
        """
        try:
            if not self.cast_manager:
                self.cast_manager = ChromecastManager()

            # Find the device
            devices_result = self.cast_manager.get_discovered_devices()
            if not devices_result.get('success', False):
                return {
                    "success": False,
                    "error": "Failed to get device list",
                    "timestamp": datetime.now().isoformat()
                }

            target_device = None
            for device in devices_result['devices']:
                if device['uuid'] == device_uuid:
                    target_device = device
                    break

            if not target_device:
                return {
                    "success": False,
                    "error": f"Device with UUID {device_uuid} not found",
                    "available_devices": [d['uuid'] for d in devices_result['devices']],
                    "timestamp": datetime.now().isoformat()
                }

            # Test the device with a sample audio file
            test_url = "http://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4"

            playback_result = self.cast_manager.play_url_on_cast(test_url, max_retries=1, preserve_target=True)

            return {
                "success": playback_result.get('success', False),
                "device": target_device,
                "test_url": test_url,
                "playback_result": playback_result,
                "message": f"Test {'successful' if playback_result.get('success') else 'failed'} for device {target_device['name']}",
                "timestamp": datetime.now().isoformat()
            }

        except Exception as e:
            logging.error(f"Error testing Chromecast device: {e}")
            return {
                "success": False,
                "error": str(e),
                "device_uuid": device_uuid,
                "timestamp": datetime.now().isoformat()
            }

    def get_system_status(self):
        """
        Get comprehensive system status.

        Returns:
            dict: JSON response with system status
        """
        try:
            # Load current config
            config_result = self.load_config() if not self.current_config else {
                "success": True,
                "config": self.current_config,
                "message": "Using cached configuration"
            }

            # Get prayer times
            prayer_result = self.get_prayer_times()

            # Get next prayer info
            next_prayer_result = self.get_next_prayer_info()

            # Get Chromecast status
            if not self.cast_manager:
                self.cast_manager = ChromecastManager()

            device_status = self.cast_manager.get_system_status()
            athan_status = self.cast_manager.get_athan_status()

            return {
                "success": True,
                "system_status": {
                    "configuration": config_result,
                    "prayer_times": prayer_result,
                    "next_prayer": next_prayer_result,
                    "chromecast_devices": device_status,
                    "athan_playback": athan_status
                },
                "summary": {
                    "config_loaded": config_result.get('success', False),
                    "prayer_times_available": prayer_result.get('success', False),
                    "next_prayer_known": next_prayer_result.get('success', False),
                    "devices_available": device_status.get('success', False) and len(device_status.get('system_status', {}).get('devices', {}).get('devices', [])) > 0,
                    "athan_playing": athan_status.get('success', False) and athan_status.get('playing', False)
                },
                "timestamp": datetime.now().isoformat()
            }

        except Exception as e:
            logging.error(f"Error getting system status: {e}")
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def trigger_athan_test(self, prayer_type="regular"):
        """
        Trigger a test Athan playback.

        Args:
            prayer_type (str): Type of prayer ("regular" or "fajr")

        Returns:
            dict: JSON response with trigger results
        """
        try:
            if not self.cast_manager:
                self.cast_manager = ChromecastManager()

            if prayer_type.lower() == "fajr":
                result = self.cast_manager.start_adahn_alfajr()
            else:
                result = self.cast_manager.start_adahn()

            return {
                "success": result.get('success', False),
                "prayer_type": prayer_type,
                "athan_result": result,
                "message": f"Athan test {'started successfully' if result.get('success') else 'failed'}",
                "timestamp": datetime.now().isoformat()
            }

        except Exception as e:
            logging.error(f"Error triggering Athan test: {e}")
            return {
                "success": False,
                "error": str(e),
                "prayer_type": prayer_type,
                "timestamp": datetime.now().isoformat()
            }

    def stop_current_playback(self):
        """
        Stop any current playback.

        Returns:
            dict: JSON response with stop results
        """
        try:
            if not self.cast_manager:
                self.cast_manager = ChromecastManager()

            stop_result = self.cast_manager.stop_athan()

            return {
                "success": stop_result.get('success', False),
                "stop_result": stop_result,
                "message": "Playback stop command sent",
                "timestamp": datetime.now().isoformat()
            }

        except Exception as e:
            logging.error(f"Error stopping playback: {e}")
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def get_media_files(self):
        """
        Get list of available media files.

        Returns:
            dict: JSON response with media files
        """
        try:
            media_dir = "Media"
            if not os.path.exists(media_dir):
                return {
                    "success": False,
                    "error": f"Media directory '{media_dir}' not found",
                    "timestamp": datetime.now().isoformat()
                }

            media_files = []
            for filename in os.listdir(media_dir):
                if filename.lower().endswith(('.mp3', '.wav', '.m4a', '.aac')):
                    file_path = os.path.join(media_dir, filename)
                    file_stat = os.stat(file_path)

                    media_files.append({
                        "filename": filename,
                        "size_bytes": file_stat.st_size,
                        "size_mb": round(file_stat.st_size / (1024 * 1024), 2),
                        "modified_time": datetime.fromtimestamp(file_stat.st_mtime).isoformat(),
                        "file_path": file_path
                    })

            return {
                "success": True,
                "media_directory": media_dir,
                "files_count": len(media_files),
                "media_files": media_files,
                "message": f"Found {len(media_files)} media files",
                "timestamp": datetime.now().isoformat()
            }

        except Exception as e:
            logging.error(f"Error getting media files: {e}")
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }


# Example usage and testing
if __name__ == "__main__":
    print("=== Web Interface API JSON Demo ===\n")

    api = WebInterfaceAPI()

    # Test configuration loading
    print("1. Loading configuration:")
    config_result = api.load_config()
    print(json.dumps(config_result, indent=2))

    # Test prayer times
    print("\n2. Getting prayer times:")
    prayer_result = api.get_prayer_times()
    print(json.dumps(prayer_result, indent=2))

    # Test next prayer info
    print("\n3. Getting next prayer info:")
    next_prayer_result = api.get_next_prayer_info()
    print(json.dumps(next_prayer_result, indent=2))

    # Test system status
    print("\n4. Getting system status:")
    status_result = api.get_system_status()
    print(json.dumps(status_result, indent=2))

    print("\n=== Web Interface API JSON Demo Complete ===")