"""
Simplified Chromecast Manager

This is the main manager class that orchestrates all Chromecast operations
using the modular components.
"""

import logging
import socket
from typing import Optional, Dict, Any, List
from datetime import datetime

# Import from parent directory
import sys
import os
sys.path.append(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from chromecast_config import ChromecastConfig, get_config
from chromecast_exceptions import (
    NoDevicesFoundError, NoSuitableDeviceError,
    DeviceNotFoundError, ChromecastError
)
from chromecast_models import (
    CastDevice, PrayerType,
    SystemStatusResponse, DeviceListResponse,
    PlaybackResponse, AthanPlaybackResponse, DeviceStatusResponse,
    CleanupResponse, CleanupAction
)
from chromecast.discovery import ChromecastDiscovery
from chromecast.connection import DeviceConnectionPool
from chromecast.playback import MediaController, AthanController


class ChromecastManager:
    """
    Simplified Chromecast Manager using modular components.

    This manager coordinates:
    - Device discovery
    - Connection management
    - Media playback
    - Athan scheduling
    - System monitoring
    """

    def __init__(self, config: Optional[ChromecastConfig] = None):
        """Initialize ChromecastManager with all components"""
        self.config = config or get_config()

        # Initialize components
        self.discovery = ChromecastDiscovery(self.config)
        self.connection_pool = DeviceConnectionPool(self.config)
        self.media_controller = MediaController(self.connection_pool, self.config)
        self.athan_controller = AthanController(self.media_controller, self.config)

        # Current target device
        self.target_device: Optional[CastDevice] = None

        # Initial discovery
        self._initial_discovery()

    def _initial_discovery(self) -> None:
        """Perform initial device discovery"""
        try:
            result = self.discovery.discover_devices()
            if result['success'] and result['devices_found'] > 0:
                logging.info(f"Initial discovery found {result['devices_found']} devices")
                self._select_target_device()
            else:
                logging.warning("No devices found during initial discovery")
        except Exception as e:
            logging.error(f"Initial discovery failed: {e}")

    def _select_target_device(self) -> Optional[CastDevice]:
        """Select the best target device"""
        device = self.discovery.find_best_device()
        if device:
            self.target_device = device
            logging.info(f"Selected target device: {device.name}")
        return device

    # === Device Discovery Methods ===

    def discover_devices(self, force: bool = False) -> Dict[str, Any]:
        """
        Discover Chromecast devices.

        Args:
            force: Force rediscovery even within cooldown

        Returns:
            Discovery response
        """
        try:
            result = self.discovery.discover_devices(force)

            # Update target device if needed
            if result['success'] and result['devices_found'] > 0:
                if not self.target_device:
                    self._select_target_device()

            return result

        except ChromecastError as e:
            return e.to_dict()
        except Exception as e:
            logging.error(f"Discovery error: {e}")
            return {
                'success': False,
                'error': str(e),
                'timestamp': datetime.now().isoformat()
            }

    def get_discovered_devices(self) -> DeviceListResponse:
        """Get list of discovered devices"""
        devices = self.discovery.get_devices()
        return {
            'success': True,
            'timestamp': datetime.now().isoformat(),
            'error': None,
            'devices_count': len(devices),
            'devices': [device.to_dict() for device in devices],
            'last_discovery_time': datetime.fromtimestamp(
                self.discovery.last_discovery_time
            ).isoformat() if self.discovery.last_discovery_time else None
        }

    # === Media Playback Methods ===

    def play_url_on_cast(
        self,
        url: str,
        device_name: Optional[str] = None
    ) -> PlaybackResponse:
        """
        Play media URL on Chromecast.

        Args:
            url: Media URL to play
            device_name: Optional device name, uses target if not specified

        Returns:
            Playback response
        """
        try:
            # Get target device
            if device_name:
                device = self.discovery.get_device(device_name)
                if not device:
                    raise DeviceNotFoundError(device_name)
            else:
                device = self.target_device
                if not device:
                    device = self._select_target_device()
                    if not device:
                        raise NoSuitableDeviceError(len(self.discovery.devices))

            # Play media
            return self.media_controller.play_media(device, url)

        except ChromecastError as e:
            response = e.to_dict()
            response['url'] = url
            return response
        except Exception as e:
            logging.error(f"Playback error: {e}")
            return {
                'success': False,
                'error': str(e),
                'url': url,
                'timestamp': datetime.now().isoformat(),
                'device': None,
                'device_source': None,
                'attempts': 0,
                'total_time': None,
                'connection_result': None,
                'load_result': None,
                'playback_attempts': None
            }

    # === Athan Methods ===

    def start_adahn(self) -> AthanPlaybackResponse:
        """Start regular Athan"""
        return self._play_athan(PrayerType.REGULAR)

    def start_adahn_alfajr(self) -> AthanPlaybackResponse:
        """Start Fajr Athan"""
        return self._play_athan(PrayerType.FAJR)

    def _play_athan(self, prayer_type: PrayerType) -> AthanPlaybackResponse:
        """Play Athan with specified type"""
        try:
            # Ensure we have a target device
            if not self.target_device:
                self._select_target_device()
                if not self.target_device:
                    raise NoSuitableDeviceError(len(self.discovery.devices))

            return self.athan_controller.play_athan(self.target_device, prayer_type)

        except ChromecastError as e:
            response = e.to_dict()
            response['prayer_type'] = prayer_type
            return response
        except Exception as e:
            logging.error(f"Athan playback error: {e}")
            return {
                'success': False,
                'error': str(e),
                'prayer_type': prayer_type,
                'timestamp': datetime.now().isoformat(),
                'media_url': None,
                'start_time': None,
                'playback_result': None,
                'skipped': False,
                'reason': None,
                'current_status': None,
                'message': None
            }

    def stop_athan(self) -> Dict[str, Any]:
        """Stop Athan playback"""
        if not self.target_device:
            return {
                'success': True,
                'was_playing': False,
                'message': "No target device",
                'timestamp': datetime.now().isoformat()
            }

        return self.athan_controller.stop_athan(self.target_device)

    def get_athan_status(self) -> Dict[str, Any]:
        """Get Athan playback status"""
        return self.athan_controller.get_status()

    def start_quran_radio(self) -> PlaybackResponse:
        """Start Quran radio stream"""
        return self.play_url_on_cast(self.config.QURAN_RADIO_URL)

    # === Device Status Methods ===

    def get_device_status(self) -> DeviceStatusResponse:
        """Get current target device status"""
        if not self.target_device:
            return {
                'success': True,
                'timestamp': datetime.now().isoformat(),
                'error': None,
                'status': 'no_device',
                'device': None,
                'availability_check': None,
                'message': "No target device selected"
            }

        availability = self.connection_pool.check_availability(self.target_device)

        return {
            'success': True,
            'timestamp': datetime.now().isoformat(),
            'error': None,
            'status': 'available' if availability['available'] else 'unavailable',
            'device': {
                'name': self.target_device.name,
                'model': self.target_device.model_name,
                'host': self.target_device.host,
                'port': self.target_device.port
            },
            'availability_check': availability,
            'message': None
        }

    # === System Methods ===

    def get_system_status(self) -> SystemStatusResponse:
        """Get comprehensive system status"""
        try:
            return {
                'success': True,
                'timestamp': datetime.now().isoformat(),
                'error': None,
                'system_status': {
                    'devices': self.get_discovered_devices(),
                    'target_device': self.get_device_status(),
                    'athan_playback': self.get_athan_status(),
                    'discovery': {
                        'stats': self.discovery.get_stats(),
                        'last_discovery': datetime.fromtimestamp(
                            self.discovery.last_discovery_time
                        ).isoformat() if self.discovery.last_discovery_time else None
                    },
                    'connection_pool': self.connection_pool.get_pool_status(),
                    'config': self.config.to_dict()
                }
            }
        except Exception as e:
            logging.error(f"Error getting system status: {e}")
            return {
                'success': False,
                'timestamp': datetime.now().isoformat(),
                'error': str(e),
                'system_status': {}
            }

    def cleanup(self) -> CleanupResponse:
        """Clean up all resources"""
        cleanup_actions: List[CleanupAction] = []

        try:
            # Stop Athan if playing
            try:
                athan_result = self.stop_athan()
                cleanup_actions.append({
                    'action': 'stop_athan',
                    'success': athan_result.get('success', False),
                    'error': athan_result.get('error'),
                    'devices_cleared': None,
                    'device_name': None,
                    'was_playing': athan_result.get('was_playing', False)
                })
            except Exception as e:
                cleanup_actions.append({
                    'action': 'stop_athan',
                    'success': False,
                    'error': str(e),
                    'devices_cleared': None,
                    'device_name': None,
                    'was_playing': None
                })

            # Clean up connection pool
            try:
                self.connection_pool.cleanup()
                cleanup_actions.append({
                    'action': 'cleanup_connections',
                    'success': True,
                    'error': None,
                    'devices_cleared': None,
                    'device_name': None,
                    'was_playing': None
                })
            except Exception as e:
                cleanup_actions.append({
                    'action': 'cleanup_connections',
                    'success': False,
                    'error': str(e),
                    'devices_cleared': None,
                    'device_name': None,
                    'was_playing': None
                })

            # Clean up discovery
            try:
                devices_count = len(self.discovery.devices)
                self.discovery.cleanup()
                cleanup_actions.append({
                    'action': 'cleanup_discovery',
                    'success': True,
                    'error': None,
                    'devices_cleared': devices_count,
                    'device_name': None,
                    'was_playing': None
                })
            except Exception as e:
                cleanup_actions.append({
                    'action': 'cleanup_discovery',
                    'success': False,
                    'error': str(e),
                    'devices_cleared': None,
                    'device_name': None,
                    'was_playing': None
                })

            # Clear target device
            target_name = self.target_device.name if self.target_device else None
            self.target_device = None
            cleanup_actions.append({
                'action': 'clear_target_device',
                'success': True,
                'error': None,
                'devices_cleared': None,
                'device_name': target_name,
                'was_playing': None
            })

            return {
                'success': True,
                'timestamp': datetime.now().isoformat(),
                'error': None,
                'cleanup_actions': cleanup_actions,
                'message': "Cleanup completed successfully"
            }

        except Exception as e:
            logging.error(f"Cleanup error: {e}")
            return {
                'success': False,
                'timestamp': datetime.now().isoformat(),
                'error': str(e),
                'cleanup_actions': cleanup_actions,
                'message': "Cleanup failed"
            }

    def __del__(self):
        """Destructor to ensure cleanup"""
        try:
            self.cleanup()
        except:
            pass

    # === Utility Methods ===

    def _get_media_url(self, filename: str) -> str:
        """Get media URL for local file"""
        try:
            # Get local IP
            s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            s.settimeout(0)
            s.connect(('10.254.254.254', 1))
            local_ip = s.getsockname()[0]
            s.close()
        except Exception:
            local_ip = '127.0.0.1'

        return f"http://{local_ip}:{self.config.WEB_INTERFACE_PORT}/media/{filename}"


# For backward compatibility
if __name__ == "__main__":
    import json

    print("=== Improved ChromecastManager Demo ===\n")

    manager = ChromecastManager()

    # Get system status
    status = manager.get_system_status()
    print("System Status:")
    print(json.dumps(status, indent=2))

    print("\n=== Demo Complete ===")