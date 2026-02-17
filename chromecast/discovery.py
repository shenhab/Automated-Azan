"""
Chromecast Device Discovery Module

This module handles all device discovery operations with multiple strategies
and fallback mechanisms.
"""

import logging
import time
import threading
from typing import Dict, List, Optional, Any, Tuple
from datetime import datetime
from abc import ABC, abstractmethod
from uuid import UUID
import hashlib

import pychromecast
from pychromecast.discovery import CastBrowser, SimpleCastListener

# Import from parent directory
import sys
import os
sys.path.append(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from chromecast_config import ChromecastConfig, get_config
from chromecast_exceptions import (
    DiscoveryError, NoDevicesFoundError, DiscoveryTimeoutError,
    DiscoveryCooldownError
)
from chromecast_models import (
    CastDevice, DeviceInfo, DiscoveryResponse, DiscoveryMethod,
    DiscoveryStats
)


class DiscoveryStrategy(ABC):
    """Abstract base class for discovery strategies"""

    @abstractmethod
    def discover(self, timeout: int) -> Tuple[Dict[str, CastDevice], str]:
        """
        Discover Chromecast devices.

        Args:
            timeout: Discovery timeout in seconds

        Returns:
            Tuple of (devices dict, method used)
        """
        pass

    @abstractmethod
    def cleanup(self) -> None:
        """Clean up discovery resources"""
        pass


class CastBrowserStrategy(DiscoveryStrategy):
    """Discovery using CastBrowser with callbacks"""

    def __init__(self, config: ChromecastConfig):
        self.config = config
        self.browser: Optional[CastBrowser] = None
        self.listener: Optional[SimpleCastListener] = None
        self.devices: Dict[str, CastDevice] = {}
        self.discovery_complete = threading.Event()
        self.lock = threading.Lock()

    def discover(self, timeout: int) -> Tuple[Dict[str, CastDevice], str]:
        """Discover devices using CastBrowser"""
        logging.debug("Starting CastBrowser discovery...")

        # Clean up any previous browser
        self.cleanup()
        self.devices.clear()
        self.discovery_complete.clear()

        # Set up callbacks
        self.listener = SimpleCastListener(
            self._add_cast,
            self._remove_cast,
            self._update_cast
        )
        self.browser = CastBrowser(self.listener, None, None)
        self.browser.start_discovery()

        # Wait for discovery with timeout
        if self.discovery_complete.wait(timeout=timeout):
            logging.debug(f"CastBrowser found {len(self.devices)} devices")
            return dict(self.devices), DiscoveryMethod.CASTBROWSER

        # If no devices found via callbacks, try hybrid approach
        logging.debug("CastBrowser callbacks timed out, trying hybrid approach...")
        return self._hybrid_discovery()

    def _hybrid_discovery(self) -> Tuple[Dict[str, CastDevice], str]:
        """Hybrid discovery using get_chromecasts with browser extraction"""
        try:
            # Stop our non-working browser
            if self.browser:
                self.browser.stop_discovery()

            # Use get_chromecasts to get a working browser
            chromecasts, working_browser = pychromecast.get_chromecasts(
                timeout=self.config.DISCOVERY_TIMEOUT_SECONDS
            )

            devices_extracted = 0
            self.devices.clear()

            # Extract from browser storage
            if working_browser and hasattr(working_browser, 'devices') and working_browser.devices:
                logging.debug(f"Working browser has {len(working_browser.devices)} devices")

                for device_uuid, device_info in working_browser.devices.items():
                    cast_device = self._extract_device_from_service(
                        str(device_uuid), device_info
                    )
                    if cast_device:
                        self.devices[cast_device.uuid] = cast_device
                        devices_extracted += 1

            # Also extract from chromecasts list
            for cast in chromecasts:
                cast_device = self._extract_device_from_cast(cast)
                if cast_device and cast_device.uuid not in self.devices:
                    self.devices[cast_device.uuid] = cast_device
                    devices_extracted += 1

            # Cleanup working browser
            if working_browser:
                try:
                    working_browser.stop_discovery()
                except Exception as e:
                    logging.debug(f"Error stopping working browser: {e}")

            logging.debug(f"Hybrid approach extracted {devices_extracted} devices")
            return dict(self.devices), DiscoveryMethod.HYBRID

        except Exception as e:
            logging.error(f"Hybrid discovery failed: {e}")
            return {}, DiscoveryMethod.HYBRID

    def _add_cast(self, uuid: str, service: Any) -> None:
        """Callback when device is discovered"""
        try:
            with self.lock:
                # Handle different callback signatures
                if isinstance(service, str):
                    # Try to get actual service from browser
                    if self.browser and hasattr(self.browser, 'devices'):
                        if uuid in self.browser.devices:
                            service = self.browser.devices[uuid]
                        else:
                            return
                    else:
                        return

                cast_device = self._extract_device_from_service(uuid, service)
                if cast_device:
                    self.devices[uuid] = cast_device
                    logging.debug(f"Added device: {cast_device.name}")

                    # Signal completion if we have devices
                    if len(self.devices) >= 1:
                        self.discovery_complete.set()

        except Exception as e:
            logging.error(f"Error adding cast device {uuid}: {e}")

    def _update_cast(self, uuid: str, service: Any) -> None:
        """Callback when device is updated"""
        self._add_cast(uuid, service)

    def _remove_cast(self, uuid: str, service: Any) -> None:
        """Callback when device is removed"""
        with self.lock:
            if uuid in self.devices:
                device = self.devices.pop(uuid)
                logging.debug(f"Removed device: {device.name}")

    def _extract_device_from_service(self, uuid: str, service: Any) -> Optional[CastDevice]:
        """Extract CastDevice from service info"""
        try:
            if hasattr(service, 'friendly_name'):
                return CastDevice(
                    uuid=uuid,
                    name=service.friendly_name,
                    host=getattr(service, 'host', 'unknown'),
                    port=getattr(service, 'port', 8009),
                    model_name=getattr(service, 'model_name', 'Unknown'),
                    manufacturer=getattr(service, 'manufacturer', 'Unknown'),
                    service=service
                )
        except Exception as e:
            logging.debug(f"Error extracting device from service: {e}")
        return None

    def _extract_device_from_cast(self, cast: Any) -> Optional[CastDevice]:
        """Extract CastDevice from pychromecast object"""
        try:
            uuid = str(getattr(cast, 'uuid', cast.name))
            if hasattr(cast, 'uuid') and cast.uuid:
                uuid = str(cast.uuid)

            return CastDevice(
                uuid=uuid,
                name=cast.name,
                host=getattr(cast, 'host', 'unknown'),
                port=getattr(cast, 'port', 8009),
                model_name=getattr(cast, 'model_name', 'Unknown'),
                manufacturer=getattr(cast, 'device', {}).get('manufacturer', 'Unknown')
                           if hasattr(cast, 'device') else 'Unknown',
                cast_object=cast
            )
        except Exception as e:
            logging.debug(f"Error extracting device from cast: {e}")
        return None

    def cleanup(self) -> None:
        """Clean up browser resources"""
        if self.browser:
            try:
                self.browser.stop_discovery()
            except Exception as e:
                logging.debug(f"Error stopping browser: {e}")
            self.browser = None
        self.listener = None


class GetChromecastsStrategy(DiscoveryStrategy):
    """Fallback discovery using get_chromecasts()"""

    def __init__(self, config: ChromecastConfig):
        self.config = config
        self.devices: Dict[str, CastDevice] = {}

    def discover(self, timeout: int) -> Tuple[Dict[str, CastDevice], str]:
        """Discover devices using get_chromecasts"""
        logging.debug("Using get_chromecasts() discovery...")

        try:
            self.devices.clear()
            chromecasts, browser = pychromecast.get_chromecasts(timeout=timeout)

            for cast in chromecasts:
                try:
                    uuid = str(getattr(cast, 'uuid', cast.name))
                    if hasattr(cast, 'uuid') and cast.uuid:
                        uuid = str(cast.uuid)

                    device = CastDevice(
                        uuid=uuid,
                        name=cast.name,
                        host=getattr(cast, 'host', 'unknown'),
                        port=getattr(cast, 'port', 8009),
                        model_name=getattr(cast, 'model_name', 'Unknown'),
                        manufacturer=getattr(cast, 'device', {}).get('manufacturer', 'Unknown')
                                   if hasattr(cast, 'device') else 'Unknown',
                        cast_object=cast
                    )
                    self.devices[uuid] = device

                except Exception as e:
                    logging.warning(f"Error processing cast device {cast.name}: {e}")

            # Cleanup browser
            if browser:
                try:
                    browser.stop_discovery()
                except Exception as e:
                    if "loop must be running" not in str(e).lower():
                        logging.warning(f"Error during browser cleanup: {e}")

            logging.debug(f"get_chromecasts found {len(self.devices)} devices")
            return dict(self.devices), DiscoveryMethod.GET_CHROMECASTS

        except Exception as e:
            logging.error(f"get_chromecasts discovery failed: {e}")
            return {}, DiscoveryMethod.GET_CHROMECASTS

    def cleanup(self) -> None:
        """Clean up resources"""
        self.devices.clear()


class ChromecastDiscovery:
    """Main discovery manager with strategy pattern and caching"""

    def __init__(self, config: Optional[ChromecastConfig] = None):
        self.config = config or get_config()
        self.devices: Dict[str, CastDevice] = {}
        self.last_discovery_time: float = 0
        self.stats = DiscoveryStats()
        self.lock = threading.Lock()

        # Initialize strategies
        self.strategies = [
            CastBrowserStrategy(self.config),
            GetChromecastsStrategy(self.config)
        ]

    def discover_devices(self, force: bool = False) -> DiscoveryResponse:
        """
        Discover Chromecast devices with caching and cooldown.

        Args:
            force: Force rediscovery even within cooldown period

        Returns:
            DiscoveryResponse with discovered devices
        """
        with self.lock:
            current_time = time.time()

            # Check cooldown period
            if not force:
                cooldown_remaining = self.config.DISCOVERY_COOLDOWN_SECONDS - \
                                   (current_time - self.last_discovery_time)
                if cooldown_remaining > 0:
                    logging.info(f"Discovery cooldown active ({cooldown_remaining:.1f}s remaining)")
                    return {
                        'success': True,
                        'timestamp': datetime.now().isoformat(),
                        'error': None,
                        'method': None,
                        'devices_found': len(self.devices),
                        'devices': {uuid: device.to_dict() for uuid, device in self.devices.items()},
                        'skipped': True,
                        'reason': f"Cooldown period ({cooldown_remaining:.1f}s remaining)",
                        'discovery_result': None
                    }

            logging.info("Starting device discovery...")
            self.last_discovery_time = current_time

            # Try each strategy in order
            for strategy in self.strategies:
                try:
                    discovered_devices, method = strategy.discover(
                        self.config.DISCOVERY_TIMEOUT_SECONDS
                    )

                    if discovered_devices:
                        self.devices = discovered_devices
                        self._log_discovered_devices()
                        self.stats.add_discovery_result(True, len(discovered_devices), method)

                        return {
                            'success': True,
                            'timestamp': datetime.now().isoformat(),
                            'error': None,
                            'method': method,
                            'devices_found': len(discovered_devices),
                            'devices': {uuid: device.to_dict() for uuid, device in discovered_devices.items()},
                            'skipped': False,
                            'reason': None,
                            'discovery_result': {
                                'strategy': strategy.__class__.__name__,
                                'discovery_time': time.time() - current_time
                            }
                        }

                except Exception as e:
                    logging.error(f"Strategy {strategy.__class__.__name__} failed: {e}")
                    continue

            # All strategies failed
            self.stats.add_discovery_result(False, 0, "failed")
            raise NoDevicesFoundError("No devices found with any discovery strategy")

    def get_device(self, device_name: str) -> Optional[CastDevice]:
        """
        Get a specific device by name.

        Args:
            device_name: Name of the device to find

        Returns:
            CastDevice if found, None otherwise
        """
        for device in self.devices.values():
            if device.name.lower() == device_name.lower():
                return device
        return None

    def get_devices(self) -> List[CastDevice]:
        """Get list of all discovered devices"""
        return list(self.devices.values())

    def find_best_device(self) -> Optional[CastDevice]:
        """
        Find the best device for casting based on priority rules.

        Returns:
            Best CastDevice or None if no suitable device found
        """
        # First priority: Look for primary device
        primary_device = self.get_device(self.config.PRIMARY_DEVICE_NAME)
        if primary_device:
            logging.info(f"Found primary device: {primary_device.name}")
            return primary_device

        # Second priority: Look for fallback devices
        fallback_devices = []
        for device in self.devices.values():
            if device.model_name.strip() in self.config.FALLBACK_DEVICE_MODELS:
                fallback_devices.append(device)

        if fallback_devices:
            device = fallback_devices[0]
            logging.info(f"Using fallback device: {device.name} ({device.model_name})")
            return device

        logging.warning("No suitable device found")
        return None

    def _log_discovered_devices(self) -> None:
        """Log discovered devices"""
        for device in self.devices.values():
            logging.info(
                f" - {device.name} ({device.model_name}) at {device.host}:{device.port}"
            )

    def get_stats(self) -> Dict[str, Any]:
        """Get discovery statistics"""
        return {
            'total_discoveries': self.stats.total_discoveries,
            'successful_discoveries': self.stats.successful_discoveries,
            'failed_discoveries': self.stats.failed_discoveries,
            'last_discovery': self.stats.last_discovery.isoformat() if self.stats.last_discovery else None,
            'average_devices_found': self.stats.get_average_devices_found(),
            'methods_used': self.stats.discovery_methods_used
        }

    def cleanup(self) -> None:
        """Clean up all discovery resources"""
        for strategy in self.strategies:
            try:
                strategy.cleanup()
            except Exception as e:
                logging.warning(f"Error cleaning up strategy: {e}")
        self.devices.clear()