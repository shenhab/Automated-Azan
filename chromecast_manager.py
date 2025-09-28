import pychromecast
from pychromecast.discovery import CastBrowser, SimpleCastListener
import logging
import os
import time
from typing import Optional
import socket
import threading
from datetime import datetime


class ChromecastManager:
    """
    A class to manage Chromecast devices and find the best candidate for casting.
    All methods return JSON responses for API compatibility.

    Priority order:
    1. If a Chromecast device named 'Adahn' is found, it is used.
    2. Otherwise, if any Google Nest Mini devices are found, the first one is used.
    3. If no suitable devices are found, an error is logged.

    Supports playing media URLs on the selected Chromecast.
    """

    # Class-level cache shared across all instances
    _shared_chromecasts = {}
    _shared_last_discovery = 0
    _discovery_lock = threading.Lock()

    def __init__(self):
        """Initialize the ChromecastManager and discover devices."""
        # Use shared cache if available
        self.chromecasts = ChromecastManager._shared_chromecasts.copy()
        self.target_device = None  # Cache the target device
        self.last_discovery_time = ChromecastManager._shared_last_discovery
        self.discovery_cooldown = 60  # Increase to 60 seconds between rediscoveries
        self.browser = None
        self.listener = None
        self.discovery_complete = threading.Event()

        # Athan playback state management
        self.athan_playing = False
        self.athan_start_time = None
        self.playback_lock = threading.Lock()

        # Only discover if no cached devices or cache is old
        if not self.chromecasts or (time.time() - self.last_discovery_time) > self.discovery_cooldown:
            self.discover_devices()
        else:
            logging.info(f"Using cached devices: {len(self.chromecasts)} devices available")

    def _get_media_url(self, filename):
        """
        Get the appropriate media URL for the given filename.

        Args:
            filename (str): Media filename

        Returns:
            dict: JSON response with media URL
        """
        try:
            # Use HOST_IP environment variable if set (for specific Docker deployments)
            local_ip = os.environ.get('HOST_IP', None)

            if not local_ip:
                # Get the local IP address that the web interface is running on
                s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
                s.settimeout(0)
                try:
                    # Connect to Google DNS to get the real outbound IP
                    # This should work correctly in Docker with host network mode
                    s.connect(('8.8.8.8', 80))
                    local_ip = s.getsockname()[0]
                except Exception:
                    # Fallback to localhost
                    local_ip = '127.0.0.1'
                finally:
                    s.close()

            # Log the detected IP for debugging
            logging.debug(f"Detected IP for media URL: {local_ip}")

            media_url = f"http://{local_ip}:5000/media/{filename}"

            return {
                "success": True,
                "filename": filename,
                "media_url": media_url,
                "local_ip": local_ip,
                "timestamp": datetime.now().isoformat()
            }

        except Exception as e:
            return {
                "success": False,
                "filename": filename,
                "error": str(e),
                "fallback_url": f"http://127.0.0.1:5000/media/{filename}",
                "timestamp": datetime.now().isoformat()
            }

    def discover_devices(self, force_rediscovery=False):
        """
        Discovers all available Chromecast devices using CastBrowser with fallback.
        Uses shared cache to avoid redundant discoveries across instances.

        Args:
            force_rediscovery (bool): Force rediscovery even within cooldown

        Returns:
            dict: JSON response with discovery results
        """
        with ChromecastManager._discovery_lock:
            current_time = time.time()

            # Check shared cache first
            time_since_last = current_time - ChromecastManager._shared_last_discovery

            # Skip discovery if within cooldown period (unless forced)
            if not force_rediscovery and time_since_last < self.discovery_cooldown:
                # Use cached devices
                self.chromecasts = ChromecastManager._shared_chromecasts.copy()
                self.last_discovery_time = ChromecastManager._shared_last_discovery

                logging.debug(f"Using cached devices (last discovery {time_since_last:.1f}s ago)")
                return {
                    "success": True,
                    "skipped": True,
                    "reason": f"Using cache (age: {time_since_last:.1f}s, cooldown: {self.discovery_cooldown}s)",
                    "devices_cached": len(self.chromecasts),
                    "devices": {uuid: info for uuid, info in self.chromecasts.items()},
                    "timestamp": datetime.now().isoformat()
                }

        logging.info("Discovering Chromecast devices...")
        self.last_discovery_time = current_time
        ChromecastManager._shared_last_discovery = current_time

        # Try CastBrowser first (modern approach)
        castbrowser_result = self._discover_with_castbrowser()
        if castbrowser_result.get('success', False):
            # Update shared cache
            ChromecastManager._shared_chromecasts = self.chromecasts.copy()

            return {
                "success": True,
                "method": "castbrowser",
                "devices_found": len(self.chromecasts),
                "devices": {uuid: {k: v for k, v in info.items() if k != 'service' and k != 'cast_object'}
                          for uuid, info in self.chromecasts.items()},
                "discovery_result": castbrowser_result,
                "timestamp": datetime.now().isoformat()
            }

        # Fallback to get_chromecasts() if CastBrowser fails
        logging.info("CastBrowser failed, falling back to get_chromecasts()...")
        fallback_result = self._discover_with_get_chromecasts()

        # Update shared cache
        ChromecastManager._shared_chromecasts = self.chromecasts.copy()

        return {
            "success": len(self.chromecasts) > 0,
            "method": "fallback",
            "devices_found": len(self.chromecasts),
            "devices": {uuid: {k: v for k, v in info.items() if k != 'service' and k != 'cast_object'}
                      for uuid, info in self.chromecasts.items()},
            "castbrowser_result": castbrowser_result,
            "fallback_result": fallback_result,
            "timestamp": datetime.now().isoformat()
        }

    def _discover_with_castbrowser(self):
        """
        Try discovery with CastBrowser (modern approach).

        Returns:
            dict: JSON response with discovery status
        """
        try:
            logging.debug("Attempting discovery with CastBrowser...")

            # Stop previous browser if exists
            if self.browser:
                self.browser.stop_discovery()

            # Clear previous discoveries
            self.chromecasts.clear()
            self.discovery_complete.clear()

            # Method 1: Try traditional CastBrowser with callbacks first
            logging.debug("Trying CastBrowser with callbacks...")
            self.listener = SimpleCastListener(self._add_cast, self._remove_cast, self._update_cast)
            self.browser = CastBrowser(self.listener, None, None)
            self.browser.start_discovery()

            # Wait briefly for callbacks (3 seconds max)
            if self.discovery_complete.wait(timeout=3):
                logging.debug(f"CastBrowser callbacks worked! Found {len(self.chromecasts)} devices")
                if self.chromecasts:
                    device_list = []
                    for uuid, cast_info in self.chromecasts.items():
                        device_list.append({
                            "name": cast_info['name'],
                            "model": cast_info['model_name'],
                            "host": cast_info['host'],
                            "port": cast_info['port']
                        })
                        logging.info(f" - {cast_info['name']} ({cast_info['model_name']}) at {cast_info['host']}:{cast_info['port']}")

                    self.browser.stop_discovery()
                    return {
                        "success": True,
                        "method": "callbacks",
                        "devices_found": len(self.chromecasts),
                        "devices": device_list,
                        "timestamp": datetime.now().isoformat()
                    }

            # Method 2: If callbacks didn't work, use the working get_chromecasts() approach
            logging.debug("CastBrowser callbacks didn't work, using get_chromecasts() hybrid approach...")

            # Stop our non-working browser
            self.browser.stop_discovery()

            # Use get_chromecasts to get a properly working browser
            chromecasts, working_browser = pychromecast.get_chromecasts(timeout=8)
            logging.debug(f"get_chromecasts() found {len(chromecasts)} cast objects")

            devices_extracted = 0
            device_list = []

            if working_browser and hasattr(working_browser, 'devices') and working_browser.devices:
                logging.debug(f"Working browser has {len(working_browser.devices)} devices in storage")

                # Extract device information from the working browser
                for device_uuid, device_info in working_browser.devices.items():
                    device_uuid_str = str(device_uuid)

                    try:
                        if hasattr(device_info, 'friendly_name'):
                            cast_info = {
                                'uuid': device_uuid_str,
                                'name': device_info.friendly_name,
                                'host': getattr(device_info, 'host', 'unknown'),
                                'port': getattr(device_info, 'port', 8009),
                                'model_name': getattr(device_info, 'model_name', 'Unknown'),
                                'manufacturer': getattr(device_info, 'manufacturer', 'Unknown'),
                                'service': device_info
                            }
                            self.chromecasts[device_uuid_str] = cast_info
                            devices_extracted += 1
                            device_list.append({
                                "name": cast_info['name'],
                                "model": cast_info['model_name'],
                                "host": cast_info['host'],
                                "port": cast_info['port']
                            })

                    except Exception as e:
                        logging.debug(f"Error extracting device {device_uuid}: {e}")
                        continue

                # Also extract from chromecasts list as backup
                for cast in chromecasts:
                    try:
                        cast_uuid = getattr(cast, 'uuid', cast.name)
                        if hasattr(cast, 'uuid') and cast.uuid:
                            cast_uuid = str(cast.uuid)

                        # Skip if we already have this device
                        if cast_uuid in self.chromecasts:
                            continue

                        cast_info = {
                            'uuid': cast_uuid,
                            'name': cast.name,
                            'host': getattr(cast, 'host', 'unknown'),
                            'port': getattr(cast, 'port', 8009),
                            'model_name': getattr(cast, 'model_name', 'Unknown'),
                            'manufacturer': getattr(cast, 'device', {}).get('manufacturer', 'Unknown') if hasattr(cast, 'device') else 'Unknown',
                            'cast_object': cast  # Store the actual cast object for later use
                        }
                        self.chromecasts[cast_uuid] = cast_info
                        devices_extracted += 1
                        device_list.append({
                            "name": cast_info['name'],
                            "model": cast_info['model_name'],
                            "host": cast_info['host'],
                            "port": cast_info['port']
                        })

                    except Exception as e:
                        logging.debug(f"Error processing cast object {cast.name}: {e}")
                        continue

                # Cleanup the working browser
                working_browser.stop_discovery()

                # Set our browser to the working one (already stopped)
                self.browser = None

                logging.debug(f"Extracted {devices_extracted} devices from working browser")

            # Consider CastBrowser successful if we found any devices
            if self.chromecasts:
                for uuid, cast_info in self.chromecasts.items():
                    logging.info(f" - {cast_info['name']} ({cast_info['model_name']}) at {cast_info['host']}:{cast_info['port']}")

                return {
                    "success": True,
                    "method": "hybrid",
                    "devices_found": len(self.chromecasts),
                    "devices": device_list,
                    "devices_extracted": devices_extracted,
                    "timestamp": datetime.now().isoformat()
                }
            else:
                return {
                    "success": False,
                    "method": "hybrid",
                    "error": "No devices found in hybrid approach",
                    "timestamp": datetime.now().isoformat()
                }

        except Exception as e:
            logging.debug(f"CastBrowser hybrid discovery failed: {e}")
            self.chromecasts.clear()
            return {
                "success": False,
                "method": "castbrowser",
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def _discover_with_get_chromecasts(self):
        """
        Fallback discovery using deprecated get_chromecasts().

        Returns:
            dict: JSON response with discovery status
        """
        try:
            logging.debug("Using get_chromecasts() fallback method...")

            # Clear previous discoveries
            self.chromecasts.clear()

            # Use get_chromecasts for discovery
            chromecasts, browser = pychromecast.get_chromecasts()

            device_list = []
            processed_count = 0
            error_count = 0

            # Convert to our format
            for cast in chromecasts:
                try:
                    # Get UUID (fallback to name if not available)
                    uuid = getattr(cast, 'uuid', cast.name)
                    if hasattr(cast, 'uuid') and cast.uuid:
                        uuid = str(cast.uuid)

                    # Get host/port with fallback
                    host = getattr(cast, 'host', 'unknown')
                    port = getattr(cast, 'port', 8009)

                    cast_info = {
                        'uuid': uuid,
                        'name': cast.name,
                        'host': host,
                        'port': port,
                        'model_name': getattr(cast, 'model_name', 'Unknown'),
                        'manufacturer': getattr(cast, 'device', {}).get('manufacturer', 'Unknown') if hasattr(cast, 'device') else 'Unknown',
                        'cast_object': cast  # Store the actual cast object for later use
                    }
                    self.chromecasts[uuid] = cast_info
                    processed_count += 1

                    device_list.append({
                        "name": cast_info['name'],
                        "model": cast_info['model_name'],
                        "host": cast_info['host'],
                        "port": cast_info['port']
                    })

                except Exception as e:
                    logging.warning(f"Error processing cast device {cast.name}: {e}")
                    error_count += 1
                    continue

            # Cleanup the browser
            if browser:
                try:
                    browser.stop_discovery()
                except Exception as e:
                    # Ignore common threading/loop errors during cleanup
                    if "loop must be running" in str(e).lower():
                        logging.debug(f"Ignoring Zeroconf cleanup error: {e}")
                    else:
                        logging.warning(f"Error during browser cleanup: {e}")

            if self.chromecasts:
                for uuid, cast_info in self.chromecasts.items():
                    logging.info(f" - {cast_info['name']} ({cast_info['model_name']}) at {cast_info['host']}:{cast_info['port']}")

                return {
                    "success": True,
                    "method": "get_chromecasts",
                    "devices_found": len(self.chromecasts),
                    "devices": device_list,
                    "processed_count": processed_count,
                    "error_count": error_count,
                    "timestamp": datetime.now().isoformat()
                }
            else:
                return {
                    "success": False,
                    "method": "get_chromecasts",
                    "error": "No devices found with fallback method",
                    "processed_count": processed_count,
                    "error_count": error_count,
                    "timestamp": datetime.now().isoformat()
                }

        except Exception as e:
            logging.error(f"Error during fallback device discovery: {e}")
            self.chromecasts.clear()
            return {
                "success": False,
                "method": "get_chromecasts",
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def get_discovered_devices(self):
        """
        Get list of all discovered devices.

        Returns:
            dict: JSON response with device list
        """
        devices = []
        for uuid, cast_info in self.chromecasts.items():
            devices.append({
                "uuid": uuid,
                "name": cast_info['name'],
                "host": cast_info['host'],
                "port": cast_info['port'],
                "model_name": cast_info['model_name'],
                "manufacturer": cast_info['manufacturer'],
                "available": self._is_device_available(cast_info)
            })

        return {
            "success": True,
            "devices_count": len(devices),
            "devices": devices,
            "last_discovery_time": datetime.fromtimestamp(self.last_discovery_time).isoformat() if self.last_discovery_time else None,
            "timestamp": datetime.now().isoformat()
        }

    def _add_cast(self, uuid, service):
        """Callback when a new cast device is discovered."""
        try:
            # Handle different callback signatures in different pychromecast versions
            if isinstance(service, str):
                # In some versions, the second parameter is the service name string
                # Try to get the actual service info from the browser
                logging.debug(f"Received string service callback: {service}")
                if self.browser and hasattr(self.browser, 'devices'):
                    # Look up the device in browser's devices
                    if uuid in self.browser.devices:
                        device_info = self.browser.devices[uuid]
                        if hasattr(device_info, 'friendly_name'):
                            service = device_info
                        else:
                            logging.debug(f"Device info doesn't have expected attributes: {type(device_info)}")
                            return
                    else:
                        logging.debug(f"UUID {uuid} not found in browser devices")
                        return
                else:
                    logging.debug("Browser not available or doesn't have devices attribute")
                    return

            cast_info = {
                'uuid': uuid,
                'name': service.friendly_name,
                'host': service.host,
                'port': service.port,
                'model_name': service.model_name,
                'manufacturer': service.manufacturer,
                'service': service
            }
            self.chromecasts[uuid] = cast_info
            logging.debug(f"Added cast device: {service.friendly_name} ({service.model_name})")

            # Signal discovery completion if we found some devices
            if len(self.chromecasts) >= 1:
                self.discovery_complete.set()

        except Exception as e:
            logging.error(f"Error adding cast device {uuid}: {e}")

    def _update_cast(self, uuid, service):
        """Callback when a cast device is updated - treat as add."""
        self._add_cast(uuid, service)

    def _remove_cast(self, uuid, service):
        """Callback when a cast device is removed."""
        if uuid in self.chromecasts:
            cast_info = self.chromecasts.pop(uuid)
            logging.debug(f"Removed cast device: {cast_info['name']}")

            # Clear cached target device if it was removed
            if self.target_device and getattr(self.target_device, 'uuid', None) == uuid:
                self.target_device = None

    def _find_casting_candidate(self, retry_discovery=True):
        """
        Finds a suitable Chromecast device to cast to.

        Args:
            retry_discovery (bool): Whether to retry device discovery if no device is found

        Returns:
            dict: JSON response with casting candidate or error
        """
        # Try to use cached device first if it's still available
        if self.target_device and self._is_device_available_by_cast(self.target_device).get('available', False):
            logging.debug(f"Using cached device: {self.target_device.name}")
            return {
                "success": True,
                "device": {
                    "name": self.target_device.name,
                    "host": getattr(self.target_device, 'host', 'unknown'),
                    "model": getattr(self.target_device, 'model_name', 'unknown')
                },
                "source": "cached",
                "cast_device": self.target_device,
                "timestamp": datetime.now().isoformat()
            }

        candidate_list = []  # Stores Google Nest devices as a fallback

        for uuid, cast_info in self.chromecasts.items():
            try:
                logging.debug(f"Checking device: {cast_info['name']} ({cast_info['model_name']}) at {cast_info['host']}")

                # Check if the device name matches 'Adahn' (case-insensitive)
                if cast_info['name'].lower() == 'adahn':
                    logging.info(f"Found target Chromecast: {cast_info['name']} at {cast_info['host']}")

                    # Create Chromecast instance - handle both discovery methods
                    cast_device = self._create_cast_device(cast_info)
                    if cast_device:
                        self.target_device = cast_device  # Cache the device
                        return {
                            "success": True,
                            "device": {
                                "name": cast_info['name'],
                                "host": cast_info['host'],
                                "model": cast_info['model_name']
                            },
                            "source": "primary_target",
                            "cast_device": cast_device,
                            "timestamp": datetime.now().isoformat()
                        }

                # If it's a Google Nest device, add it to the candidate list
                elif cast_info['model_name'].strip() in ["Google Nest Mini", "Google Nest Hub", "Google Home", "Google Home Mini"]:
                    logging.info(f"Adding Google Nest device to candidate list: {cast_info['name']}")
                    candidate_list.append(cast_info)

            except Exception as e:
                logging.warning(f"Error checking device {cast_info['name']}: {e}")
                continue

        # If no 'Adahn' was found but we have Google Nest devices, return the first one
        if candidate_list:
            cast_info = candidate_list[0]
            logging.info(f"Using fallback Chromecast: {cast_info['name']}")
            cast_device = self._create_cast_device(cast_info)
            if cast_device:
                self.target_device = cast_device  # Cache the device
                return {
                    "success": True,
                    "device": {
                        "name": cast_info['name'],
                        "host": cast_info['host'],
                        "model": cast_info['model_name']
                    },
                    "source": "fallback_candidate",
                    "cast_device": cast_device,
                    "candidates_available": len(candidate_list),
                    "timestamp": datetime.now().isoformat()
                }

        # If no suitable devices found and retry is enabled, try rediscovering
        if retry_discovery:
            logging.warning("No suitable device found, attempting rediscovery...")
            discovery_result = self.discover_devices(force_rediscovery=True)

            if discovery_result.get('success', False) and discovery_result.get('devices_found', 0) > 0:
                return self._find_casting_candidate(retry_discovery=False)  # Avoid infinite recursion
            else:
                return {
                    "success": False,
                    "error": "No devices found after rediscovery",
                    "discovery_result": discovery_result,
                    "timestamp": datetime.now().isoformat()
                }

        # No suitable devices found
        logging.warning("No suitable Chromecast candidate found.")
        self.target_device = None
        return {
            "success": False,
            "error": "No suitable Chromecast candidate found",
            "devices_checked": len(self.chromecasts),
            "candidates_found": len(candidate_list),
            "timestamp": datetime.now().isoformat()
        }

    def _create_cast_device(self, cast_info):
        """Create a Chromecast device from cast_info, handling both discovery methods."""
        try:
            # If we have a cast_object from get_chromecasts(), use it directly
            if 'cast_object' in cast_info and cast_info['cast_object']:
                logging.info(f"Using cached cast object for {cast_info['name']}")
                return cast_info['cast_object']

            # If we have a service from CastBrowser, create from service
            elif 'service' in cast_info and cast_info['service']:
                logging.debug(f"Creating cast device from service for {cast_info['name']}")
                try:
                    # Create CastInfo from service and use Chromecast constructor
                    from pychromecast.models import CastInfo
                    from uuid import UUID

                    # Convert string UUID to UUID object if needed
                    uuid_obj = cast_info['uuid']
                    if isinstance(uuid_obj, str):
                        try:
                            uuid_obj = UUID(uuid_obj)
                        except Exception as uuid_error:
                            logging.debug(f"Invalid UUID format '{uuid_obj}': {uuid_error}")
                            # Generate a fallback UUID based on host/port
                            import hashlib
                            uuid_str = hashlib.md5(f"{cast_info['host']}:{cast_info['port']}".encode()).hexdigest()
                            uuid_obj = UUID(uuid_str[:8] + '-' + uuid_str[8:12] + '-' + uuid_str[12:16] + '-' + uuid_str[16:20] + '-' + uuid_str[20:32])

                    cast_info_obj = CastInfo(
                        services=frozenset([cast_info['service']]) if cast_info['service'] else frozenset(),
                        uuid=uuid_obj,
                        model_name=cast_info['model_name'],
                        friendly_name=cast_info['name'],
                        host=cast_info['host'],
                        port=cast_info['port'],
                        cast_type=cast_info.get('cast_type', 'cast'),  # Default to 'cast'
                        manufacturer=cast_info.get('manufacturer', 'Google Inc.')
                    )

                    logging.info(f"Creating Chromecast from CastInfo object for {cast_info['name']}")
                    return pychromecast.Chromecast(
                        cast_info_obj,
                        timeout=5,
                        zconf=self.browser.zc if self.browser and hasattr(self.browser, 'zc') else None
                    )

                except Exception as service_error:
                    logging.debug(f"Service creation failed: {service_error}")
                    # Fall through to host/port creation

            # Fallback: create from host/port using get_chromecast_from_host
            logging.debug(f"Creating cast device from host/port for {cast_info['name']}")
            try:
                from uuid import UUID

                # Convert string UUID to UUID object if needed
                uuid_obj = cast_info['uuid']
                if isinstance(uuid_obj, str):
                    try:
                        uuid_obj = UUID(uuid_obj)
                    except Exception:
                        # Generate a fallback UUID based on host/port
                        import hashlib
                        uuid_str = hashlib.md5(f"{cast_info['host']}:{cast_info['port']}".encode()).hexdigest()
                        uuid_obj = UUID(uuid_str[:8] + '-' + uuid_str[8:12] + '-' + uuid_str[12:16] + '-' + uuid_str[16:20] + '-' + uuid_str[20:32])

                # The function expects a tuple: (host, port, uuid, friendly_name, model_name)
                host_tuple = (
                    cast_info['host'],
                    int(cast_info['port']),  # Ensure port is integer
                    uuid_obj,  # Use UUID object, not string
                    cast_info['name'],
                    cast_info['model_name']
                )
                logging.info(f"Trying get_chromecast_from_host with tuple for {cast_info['name']}")
                return pychromecast.get_chromecast_from_host(
                    host_tuple,
                    timeout=5
                )
            except Exception as host_error:
                logging.debug(f"Host creation with tuple failed: {host_error}")
                # Last resort - return None and skip this device
                return None

        except Exception as e:
            logging.error(f"Error creating cast device for {cast_info['name']}: {e}")
            return None

    def _is_device_available(self, cast_info):
        """
        Check if a device info dict is still available and responsive.

        Args:
            cast_info (dict): Device information

        Returns:
            dict: JSON response with availability status
        """
        try:
            # Quick network connectivity check
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            sock.settimeout(3)  # 3 second timeout
            result = sock.connect_ex((cast_info['host'], cast_info['port']))
            sock.close()

            available = result == 0
            return {
                "success": True,
                "available": available,
                "device_name": cast_info['name'],
                "host": cast_info['host'],
                "port": cast_info['port'],
                "response_code": result,
                "timestamp": datetime.now().isoformat()
            }

        except Exception as e:
            logging.debug(f"Device availability check failed for {cast_info['name']}: {e}")
            return {
                "success": False,
                "available": False,
                "device_name": cast_info['name'],
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def _is_device_available_by_cast(self, device):
        """
        Check if a Chromecast device instance is still available and responsive.

        Args:
            device: Chromecast device instance

        Returns:
            dict: JSON response with availability status
        """
        try:
            # Quick network connectivity check
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            sock.settimeout(3)  # 3 second timeout
            result = sock.connect_ex((device.host, device.port))
            sock.close()

            available = result == 0
            return {
                "success": True,
                "available": available,
                "device_name": device.name,
                "host": device.host,
                "port": device.port,
                "response_code": result,
                "timestamp": datetime.now().isoformat()
            }

        except Exception as e:
            logging.debug(f"Device availability check failed for {device.name}: {e}")
            return {
                "success": False,
                "available": False,
                "device_name": getattr(device, 'name', 'unknown'),
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def _connect_with_retry(self, device, max_retries=3):
        """
        Connect to device with retry logic and error handling.

        Args:
            device: Chromecast device instance
            max_retries (int): Maximum retry attempts

        Returns:
            dict: JSON response with connection status
        """
        connection_attempts = []

        for attempt in range(max_retries):
            attempt_start = time.time()
            try:
                logging.info(f"Attempting to connect to {device.name} (attempt {attempt + 1}/{max_retries})")
                device.wait(timeout=10)  # 10 second timeout for connection

                connection_time = time.time() - attempt_start
                logging.info(f"Successfully connected to Chromecast: {device.name}")

                return {
                    "success": True,
                    "device_name": device.name,
                    "attempts": attempt + 1,
                    "connection_time": round(connection_time, 2),
                    "connection_attempts": connection_attempts,
                    "timestamp": datetime.now().isoformat()
                }

            except Exception as e:
                connection_time = time.time() - attempt_start
                error_msg = str(e)

                attempt_info = {
                    "attempt": attempt + 1,
                    "error": error_msg,
                    "connection_time": round(connection_time, 2)
                }
                connection_attempts.append(attempt_info)

                logging.warning(f"Connection attempt {attempt + 1} failed for {device.name}: {error_msg}")

                # If it's a threading error, clear the cached device and force rediscovery
                if "threads can only be started once" in error_msg:
                    logging.info("Threading error detected, clearing cached device")
                    self.target_device = None
                    if attempt < max_retries - 1:
                        time.sleep(2)
                        discovery_result = self.discover_devices(force_rediscovery=True)
                        candidate_result = self._find_casting_candidate(retry_discovery=False)
                        if not candidate_result.get('success', False):
                            logging.error("Device no longer available after rediscovery")
                            return {
                                "success": False,
                                "error": "Device no longer available after rediscovery",
                                "connection_attempts": connection_attempts,
                                "discovery_result": discovery_result,
                                "timestamp": datetime.now().isoformat()
                            }
                        device = candidate_result.get('cast_device')
                        if not device:
                            return {
                                "success": False,
                                "error": "No device returned from candidate search",
                                "connection_attempts": connection_attempts,
                                "timestamp": datetime.now().isoformat()
                            }
                elif attempt < max_retries - 1:
                    time.sleep(2)  # Wait before retry

        return {
            "success": False,
            "error": f"Failed to connect to {device.name} after {max_retries} attempts",
            "device_name": device.name,
            "connection_attempts": connection_attempts,
            "timestamp": datetime.now().isoformat()
        }

    def play_url_on_cast(self, url, max_retries=2, preserve_target=False):
        """
        Plays an MP3 URL on the selected Chromecast device with robust error handling.

        Args:
            url (str): The media URL to play
            max_retries (int): Maximum number of retries for playback
            preserve_target (bool): If True, don't clear target_device on retry

        Returns:
            dict: JSON response with playback status
        """
        playback_attempts = []

        for retry in range(max_retries + 1):
            attempt_start = time.time()

            # Use existing target device if set, otherwise find one
            if hasattr(self, 'target_device') and self.target_device:
                target_device = self.target_device
                logging.debug("Using pre-set target device for playback")
                device_source = "cached"
            else:
                candidate_result = self._find_casting_candidate()
                if not candidate_result.get('success', False):
                    return {
                        "success": False,
                        "error": "No available Chromecast device to play the media",
                        "candidate_result": candidate_result,
                        "timestamp": datetime.now().isoformat()
                    }
                target_device = candidate_result.get('cast_device')
                device_source = candidate_result.get('source', 'discovered')

            if not target_device:
                attempt_info = {
                    "attempt": retry + 1,
                    "error": "No target device available",
                    "attempt_time": time.time() - attempt_start
                }
                playback_attempts.append(attempt_info)
                continue

            try:
                # Connect to the selected Chromecast device with retry logic
                connection_result = self._connect_with_retry(target_device)
                if not connection_result.get('success', False):
                    if retry < max_retries:
                        attempt_info = {
                            "attempt": retry + 1,
                            "error": "Connection failed",
                            "connection_result": connection_result,
                            "attempt_time": time.time() - attempt_start
                        }
                        playback_attempts.append(attempt_info)
                        logging.info(f"Retrying playback... (attempt {retry + 2}/{max_retries + 1})")
                        continue
                    else:
                        return {
                            "success": False,
                            "error": "Failed to connect to device for playback",
                            "connection_result": connection_result,
                            "playback_attempts": playback_attempts,
                            "timestamp": datetime.now().isoformat()
                        }

                # Get media controller
                media_controller = target_device.media_controller

                # Only stop previous media if it's actually playing something different
                try:
                    media_controller.update_status()
                    time.sleep(0.5)  # Shorter wait
                    current_content = media_controller.status.content_id
                    current_state = media_controller.status.player_state

                    if current_content and current_content != url and current_state not in ["IDLE", "UNKNOWN"]:
                        logging.info(f"Stopping different media session: {current_content}")
                        media_controller.stop()
                        time.sleep(1.5)  # Shorter wait after stop
                    elif current_content == url:
                        logging.info("Same media already loaded, restarting...")
                        media_controller.stop()
                        time.sleep(1)
                except Exception as e:
                    logging.debug(f"Error checking previous media (continuing anyway): {e}")

                # Send the media play request
                logging.info(f"Streaming {url} on {target_device.name}...")
                logging.info(f"Device details - Host: {getattr(target_device, 'host', 'unknown')}, "
                            f"Port: {getattr(target_device, 'port', 'unknown')}, "
                            f"Model: {getattr(target_device, 'model_name', 'unknown')}")
                logging.info(f"Media URL: {url}")

                # Use more robust media type detection
                content_type = "audio/mpeg"
                if url.endswith('.mp3'):
                    content_type = "audio/mpeg"
                elif url.endswith('.m4a'):
                    content_type = "audio/mp4"

                logging.info(f"About to call play_media with URL: {url}, content_type: {content_type}")

                # Add timeout wrapper around the potentially hanging play_media call
                def play_media_with_timeout():
                    media_controller.play_media(url, content_type)

                try:
                    import signal
                    import threading

                    # Use threading with timeout as signal doesn't work reliably in Docker
                    result_container = [None]
                    exception_container = [None]

                    def target():
                        try:
                            media_controller.play_media(url, content_type)
                            result_container[0] = "success"
                        except Exception as e:
                            exception_container[0] = e

                    thread = threading.Thread(target=target)
                    thread.daemon = True
                    thread.start()
                    thread.join(timeout=10)  # 10 second timeout

                    if thread.is_alive():
                        logging.error("play_media call timed out after 10 seconds - this is the source of the hang!")
                        return {
                            "success": False,
                            "error": "play_media call timed out after 10 seconds",
                            "url": url,
                            "device": target_device.name,
                            "timestamp": datetime.now().isoformat()
                        }

                    if exception_container[0]:
                        raise exception_container[0]

                    logging.info("play_media call completed, now waiting for media to load...")

                except Exception as e:
                    logging.error(f"play_media call failed with error: {e}")
                    return {
                        "success": False,
                        "error": f"Failed to call play_media: {str(e)}",
                        "url": url,
                        "device": target_device.name,
                        "timestamp": datetime.now().isoformat()
                    }

                # Wait for Chromecast to fully load the media with better error handling
                logging.info("Starting _wait_for_media_load...")
                load_result = self._wait_for_media_load(media_controller, url)
                logging.info(f"_wait_for_media_load completed with result: {load_result.get('success')}")

                attempt_time = time.time() - attempt_start
                if load_result.get('success', False):
                    logging.info("Media playback started successfully.")
                    return {
                        "success": True,
                        "url": url,
                        "device": {
                            "name": target_device.name,
                            "host": getattr(target_device, 'host', 'unknown'),
                            "model": getattr(target_device, 'model_name', 'unknown')
                        },
                        "device_source": device_source,
                        "attempts": retry + 1,
                        "total_time": round(attempt_time, 2),
                        "connection_result": connection_result,
                        "load_result": load_result,
                        "playback_attempts": playback_attempts,
                        "timestamp": datetime.now().isoformat()
                    }
                else:
                    attempt_info = {
                        "attempt": retry + 1,
                        "error": "Media failed to load",
                        "load_result": load_result,
                        "attempt_time": round(attempt_time, 2)
                    }
                    playback_attempts.append(attempt_info)

                    logging.warning(f"Media failed to load on attempt {retry + 1}")
                    if retry < max_retries:
                        if preserve_target:
                            logging.info("Retrying with same target device...")
                        else:
                            logging.info("Retrying with fresh device discovery...")
                            self.target_device = None  # Clear cached device
                        time.sleep(2)
                        continue
                    else:
                        return {
                            "success": False,
                            "error": "Media playback failed after all retries",
                            "url": url,
                            "playback_attempts": playback_attempts,
                            "timestamp": datetime.now().isoformat()
                        }

            except Exception as e:
                attempt_time = time.time() - attempt_start
                attempt_info = {
                    "attempt": retry + 1,
                    "error": str(e),
                    "attempt_time": round(attempt_time, 2)
                }
                playback_attempts.append(attempt_info)

                logging.error(f"Error during playback attempt {retry + 1}: {e}")
                if retry < max_retries:
                    if not preserve_target:
                        self.target_device = None  # Clear cached device on error
                    time.sleep(2)
                    continue
                else:
                    return {
                        "success": False,
                        "error": "Playback failed after all retries",
                        "url": url,
                        "playback_attempts": playback_attempts,
                        "timestamp": datetime.now().isoformat()
                    }

        return {
            "success": False,
            "error": "Unexpected end of retry loop",
            "url": url,
            "playback_attempts": playback_attempts,
            "timestamp": datetime.now().isoformat()
        }

    def _wait_for_media_load(self, media_controller, url, max_attempts=10):
        """
        Wait for media to load with improved logic and timeout.

        Args:
            media_controller: The media controller instance
            url (str): Expected content URL
            max_attempts (int): Maximum attempts to wait (reduced from 15)

        Returns:
            dict: JSON response with load status
        """
        attempts = 0
        status_checks = []
        last_player_state = None
        consecutive_playing_states = 0
        start_time = time.time()
        max_wait_time = 20  # Maximum 20 seconds total wait time

        logging.info(f"Starting _wait_for_media_load: max_attempts={max_attempts}, max_wait_time={max_wait_time}s")

        while attempts < max_attempts and (time.time() - start_time) < max_wait_time:
            attempt_start = time.time()
            elapsed_total = time.time() - start_time
            logging.info(f"_wait_for_media_load: Starting attempt {attempts+1}/{max_attempts} (elapsed: {elapsed_total:.1f}s)")

            try:
                logging.debug("About to call media_controller.update_status()...")
                media_controller.update_status()
                logging.debug("media_controller.update_status() completed successfully")

                logging.debug("Sleeping 0.5 seconds after update_status...")
                time.sleep(0.5)  # Shorter initial wait
                logging.debug("Sleep completed")

                logging.debug("Getting player_state and content_id from status...")
                player_state = media_controller.status.player_state
                content_id = media_controller.status.content_id
                logging.debug(f"Retrieved status - player_state: {player_state}, content_id: {content_id}")

                status_check = {
                    "attempt": attempts + 1,
                    "player_state": player_state,
                    "content_id": content_id,
                    "expected_url": url,
                    "content_matches": content_id == url,
                    "check_time": round(time.time() - attempt_start, 2)
                }
                status_checks.append(status_check)

                logging.info(f"Media load attempt {attempts+1}: State={player_state}, ContentID={'✓' if content_id else '✗'}")

                # Success conditions - be more flexible
                if player_state in ["BUFFERING", "PLAYING"]:
                    # Count consecutive playing/buffering states
                    if last_player_state in ["BUFFERING", "PLAYING"]:
                        consecutive_playing_states += 1
                    else:
                        consecutive_playing_states = 1

                    # If we have a good state for 2+ consecutive checks, consider it successful
                    if consecutive_playing_states >= 2 or player_state == "PLAYING":
                        logging.info(f"Media appears to be loading/playing successfully (state: {player_state})")

                        # Try to ensure playback is active
                        try:
                            if player_state == "BUFFERING":
                                media_controller.play()
                        except Exception as e:
                            logging.debug(f"Error calling play(): {e}")

                        return {
                            "success": True,
                            "player_state": player_state,
                            "content_id": content_id,
                            "attempts": attempts + 1,
                            "consecutive_good_states": consecutive_playing_states,
                            "status_checks": status_checks,
                            "timestamp": datetime.now().isoformat()
                        }
                else:
                    consecutive_playing_states = 0

                # If we're in IDLE state for too long, it might have failed
                if player_state == "IDLE" and attempts >= 8:
                    logging.warning(f"Media stuck in IDLE state after {attempts} attempts")
                    # Don't give up immediately, but log the concern

                last_player_state = player_state

                # Progressive wait times: start short, get longer
                if attempts < 3:
                    sleep_time = 1
                elif attempts < 6:
                    sleep_time = 2
                else:
                    sleep_time = 3

                logging.debug(f"Sleeping {sleep_time} seconds before next attempt...")
                time.sleep(sleep_time)
                logging.debug(f"Sleep of {sleep_time} seconds completed")

                attempts += 1
                logging.debug(f"Completed attempt {attempts}, continuing loop...")

            except Exception as e:
                check_time = time.time() - attempt_start
                status_check = {
                    "attempt": attempts + 1,
                    "error": str(e),
                    "check_time": round(check_time, 2)
                }
                status_checks.append(status_check)

                logging.warning(f"Error checking media status (attempt {attempts+1}): {e}")
                attempts += 1
                time.sleep(1)

        # Timeout reached - log details for debugging
        elapsed_time = time.time() - start_time
        timeout_reason = "max_attempts" if attempts >= max_attempts else "max_wait_time"
        logging.error(f"Media load timeout after {elapsed_time:.1f}s and {attempts} attempts. Final state: {last_player_state}")
        logging.error(f"Timeout reason: {timeout_reason} (attempts: {attempts}/{max_attempts}, time: {elapsed_time:.1f}/{max_wait_time}s)")
        logging.info(f"_wait_for_media_load exiting with failure")

        return {
            "success": False,
            "error": f"Media failed to load within {elapsed_time:.1f}s timeout period ({timeout_reason})",
            "attempts": attempts,
            "final_state": last_player_state,
            "elapsed_time": round(elapsed_time, 2),
            "timeout_reason": timeout_reason,
            "status_checks": status_checks[-3:] if len(status_checks) > 3 else status_checks,  # Include only last 3 checks
            "timestamp": datetime.now().isoformat()
        }

    def _is_athan_playing(self):
        """
        Check if Athan is currently playing and manage playback state.

        NOTE: This method assumes it's called from within a playback_lock context.

        Returns:
            dict: JSON response with Athan playing status
        """
        logging.info(f"[DEBUG] _is_athan_playing() called")
        # Note: No lock needed here as this is called from within a locked context
        if not self.athan_playing:
            return {
                "success": True,
                "is_playing": False,
                "message": "No Athan currently playing",
                "timestamp": datetime.now().isoformat()
            }

        # Check if enough time has passed for Athan to finish
        # Most Athan recordings are 3-5 minutes, so we'll use 8 minutes as a safe timeout
        if self.athan_start_time:
            elapsed_time = time.time() - self.athan_start_time
            if elapsed_time > 480:  # 8 minutes timeout
                logging.info("Athan playback timeout reached, clearing playback state")
                self.athan_playing = False
                self.athan_start_time = None
                return {
                    "success": True,
                    "is_playing": False,
                    "reason": "timeout_reached",
                    "elapsed_time": round(elapsed_time, 1),
                    "timeout_threshold": 480,
                    "timestamp": datetime.now().isoformat()
                }

        # Check if target device is actually playing something
        logging.info(f"[DEBUG] About to check target device status...")
        if self.target_device:
            try:
                logging.info(f"[DEBUG] About to call media_controller.update_status()...")
                media_controller = self.target_device.media_controller

                # Add timeout wrapper around the hanging update_status call
                import threading
                exception_container = [None]

                def update_status_target():
                    try:
                        media_controller.update_status()
                    except Exception as e:
                        exception_container[0] = e

                thread = threading.Thread(target=update_status_target)
                thread.daemon = True
                thread.start()
                thread.join(timeout=5)  # 5 second timeout

                if thread.is_alive():
                    logging.error("media_controller.update_status() timed out after 5 seconds - this is the source of the hang!")
                    return {
                        "success": True,
                        "is_playing": False,
                        "reason": "status_check_timeout",
                        "message": "Unable to check device status due to timeout",
                        "timestamp": datetime.now().isoformat()
                    }

                if exception_container[0]:
                    raise exception_container[0]

                logging.info(f"[DEBUG] media_controller.update_status() completed successfully")

                player_state = media_controller.status.player_state
                # If device is idle or stopped, clear our playback state
                if player_state in ["IDLE", "UNKNOWN"]:
                    logging.info("Target device shows IDLE/UNKNOWN state, clearing Athan playback state")
                    self.athan_playing = False
                    self.athan_start_time = None
                    return {
                        "success": True,
                        "is_playing": False,
                        "reason": "device_idle",
                        "device_state": player_state,
                        "elapsed_time": round(elapsed_time, 1) if self.athan_start_time else 0,
                        "timestamp": datetime.now().isoformat()
                    }

            except Exception as e:
                logging.debug(f"Error checking device playback state: {e}")
                # If we can't check status, assume playback might still be active

            elapsed_time = time.time() - self.athan_start_time if self.athan_start_time else 0
            return {
                "success": True,
                "is_playing": self.athan_playing,
                "elapsed_time": round(elapsed_time, 1),
                "device_available": self.target_device is not None,
                "timestamp": datetime.now().isoformat()
            }

    def _start_athan_playback(self, prayer_type="regular"):
        """
        Start Athan playback with collision protection.

        Args:
            prayer_type (str): Type of prayer ("regular" or "fajr")

        Returns:
            dict: JSON response with playback start status
        """
        logging.info(f"[DEBUG] _start_athan_playback called with prayer_type: {prayer_type}")

        logging.info(f"[DEBUG] About to acquire playback_lock...")
        with self.playback_lock:
            logging.info(f"[DEBUG] playback_lock acquired successfully")

            # Check if Athan is already playing
            logging.info(f"[DEBUG] Checking if Athan is already playing...")
            playing_status = self._is_athan_playing()
            logging.info(f"[DEBUG] _is_athan_playing() completed with result: {playing_status}")

            if playing_status.get('is_playing', False):
                logging.warning(f"Athan is already playing, skipping {prayer_type} Athan request")
                return {
                    "success": True,
                    "skipped": True,
                    "reason": "already_playing",
                    "prayer_type": prayer_type,
                    "current_status": playing_status,
                    "timestamp": datetime.now().isoformat()
                }

            # Get media URL
            if prayer_type == "fajr":
                url_result = self._get_media_url("media_adhan_al_fajr.mp3")
                logging.info("Starting Fajr Adhan playback...")
            else:
                url_result = self._get_media_url("media_Athan.mp3")
                logging.info("Starting regular Adhan playback...")
            

            if not url_result.get('success', False):
                return {
                    "success": False,
                    "error": "Failed to get media URL",
                    "prayer_type": prayer_type,
                    "url_result": url_result,
                    "timestamp": datetime.now().isoformat()
                }

            adahn_url = url_result['media_url']
            logging.debug(f"got the prayer file link:{adahn_url}")

            # Mark Athan as starting
            self.athan_playing = True
            self.athan_start_time = time.time()

            # Attempt playback
            playback_result = self.play_url_on_cast(adahn_url)

            if not playback_result.get('success', False):
                # Reset state if playback failed
                self.athan_playing = False
                self.athan_start_time = None
                logging.error(f"Failed to play {prayer_type} Adhan")

                return {
                    "success": False,
                    "error": f"Failed to play {prayer_type} Adhan",
                    "prayer_type": prayer_type,
                    "media_url": adahn_url,
                    "playback_result": playback_result,
                    "timestamp": datetime.now().isoformat()
                }
            else:
                logging.info(f"Successfully started {prayer_type} Adhan playback")

                return {
                    "success": True,
                    "prayer_type": prayer_type,
                    "media_url": adahn_url,
                    "start_time": datetime.fromtimestamp(self.athan_start_time).isoformat(),
                    "playback_result": playback_result,
                    "message": f"Successfully started {prayer_type} Adhan playback",
                    "timestamp": datetime.now().isoformat()
                }

    def start_quran_radio(self):
        """
        Start Quran radio stream with error handling.

        Returns:
            dict: JSON response with Quran radio start status
        """
        quran_radio_streaming = "https://n03.radiojar.com/8s5u5tpdtwzuv?rj-ttl=5&rj-tok=AAABlVflrAAAJLe-IOoD4VTShA"

        playback_result = self.play_url_on_cast(quran_radio_streaming)

        if not playback_result.get('success', False):
            logging.error("Failed to start Quran radio stream")
            return {
                "success": False,
                "error": "Failed to start Quran radio stream",
                "stream_url": quran_radio_streaming,
                "playback_result": playback_result,
                "timestamp": datetime.now().isoformat()
            }
        else:
            return {
                "success": True,
                "stream_url": quran_radio_streaming,
                "playback_result": playback_result,
                "message": "Quran radio stream started successfully",
                "timestamp": datetime.now().isoformat()
            }

    def start_adahn_alfajr(self):
        """
        Start Fajr Adhan with error handling and collision protection.

        Returns:
            dict: JSON response with Fajr Adhan start status
        """
        return self._start_athan_playback("fajr")

    def start_adahn(self):
        """
        Start regular Adhan with error handling and collision protection.

        Returns:
            dict: JSON response with regular Adhan start status
        """
        return self._start_athan_playback("regular")

    def stop_athan(self):
        """
        Stop Athan playback and clear playback state.

        Returns:
            dict: JSON response with stop status
        """
        with self.playback_lock:
            try:
                if self.target_device and self.athan_playing:
                    media_controller = self.target_device.media_controller
                    media_controller.stop()
                    logging.info("Stopped Athan playback")

                # Clear playback state
                was_playing = self.athan_playing
                elapsed_time = time.time() - self.athan_start_time if self.athan_start_time else 0

                self.athan_playing = False
                self.athan_start_time = None

                return {
                    "success": True,
                    "was_playing": was_playing,
                    "elapsed_time": round(elapsed_time, 1) if was_playing else 0,
                    "device_name": self.target_device.name if self.target_device else None,
                    "message": "Athan playback stopped successfully" if was_playing else "No Athan was playing",
                    "timestamp": datetime.now().isoformat()
                }

            except Exception as e:
                logging.error(f"Error stopping Athan playback: {e}")
                # Still clear the state even if stop failed
                was_playing = self.athan_playing
                self.athan_playing = False
                self.athan_start_time = None

                return {
                    "success": False,
                    "error": str(e),
                    "was_playing": was_playing,
                    "state_cleared": True,
                    "message": "Error stopping Athan but playback state cleared",
                    "timestamp": datetime.now().isoformat()
                }

    def get_athan_status(self):
        """
        Get current Athan playback status.

        Returns:
            dict: JSON response with Athan status
        """
        with self.playback_lock:
            if not self.athan_playing:
                return {
                    "success": True,
                    "playing": False,
                    "message": "No Athan currently playing",
                    "timestamp": datetime.now().isoformat()
                }

            elapsed_time = time.time() - self.athan_start_time if self.athan_start_time else 0
            return {
                "success": True,
                "playing": True,
                "elapsed_time": round(elapsed_time, 1),
                "start_time": datetime.fromtimestamp(self.athan_start_time).isoformat() if self.athan_start_time else None,
                "device_name": self.target_device.name if self.target_device else None,
                "message": f"Athan playing for {round(elapsed_time, 1)} seconds",
                "timestamp": datetime.now().isoformat()
            }

    def get_device_status(self):
        """
        Get status information about the current target device.

        Returns:
            dict: JSON response with device status
        """
        if not self.target_device:
            return {
                "success": True,
                "status": "no_device",
                "message": "No target device selected",
                "timestamp": datetime.now().isoformat()
            }

        try:
            availability_check = self._is_device_available_by_cast(self.target_device)

            if availability_check.get('available', False):
                return {
                    "success": True,
                    "status": "available",
                    "device": {
                        "name": self.target_device.name,
                        "host": self.target_device.host,
                        "port": getattr(self.target_device, 'port', 'unknown'),
                        "model": self.target_device.model_name
                    },
                    "availability_check": availability_check,
                    "timestamp": datetime.now().isoformat()
                }
            else:
                return {
                    "success": True,
                    "status": "unavailable",
                    "device": {
                        "name": self.target_device.name,
                        "host": self.target_device.host,
                        "port": getattr(self.target_device, 'port', 'unknown'),
                        "model": self.target_device.model_name
                    },
                    "availability_check": availability_check,
                    "message": "Device is not responding",
                    "timestamp": datetime.now().isoformat()
                }

        except Exception as e:
            return {
                "success": False,
                "status": "error",
                "error": str(e),
                "device_name": getattr(self.target_device, 'name', 'unknown'),
                "timestamp": datetime.now().isoformat()
            }

    def get_system_status(self):
        """
        Get comprehensive system status including devices, Athan state, and connections.

        Returns:
            dict: JSON response with complete system status
        """
        try:
            # Get device information
            devices_info = self.get_discovered_devices()
            device_status = self.get_device_status()
            athan_status = self.get_athan_status()

            # Get discovery information
            discovery_info = {
                "last_discovery_time": datetime.fromtimestamp(self.last_discovery_time).isoformat() if self.last_discovery_time else None,
                "discovery_cooldown": self.discovery_cooldown,
                "browser_active": self.browser is not None
            }

            return {
                "success": True,
                "system_status": {
                    "devices": devices_info,
                    "target_device": device_status,
                    "athan_playback": athan_status,
                    "discovery": discovery_info
                },
                "timestamp": datetime.now().isoformat()
            }

        except Exception as e:
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def cleanup(self):
        """
        Clean up resources.

        Returns:
            dict: JSON response with cleanup status
        """
        cleanup_actions = []

        try:
            # Stop browser
            if self.browser:
                try:
                    self.browser.stop_discovery()
                    cleanup_actions.append({"action": "stop_browser", "success": True})
                except Exception as e:
                    cleanup_actions.append({"action": "stop_browser", "success": False, "error": str(e)})
                    logging.warning(f"Error stopping browser: {e}")

            # Clear devices
            devices_cleared = len(self.chromecasts)
            self.chromecasts.clear()
            cleanup_actions.append({"action": "clear_devices", "success": True, "devices_cleared": devices_cleared})

            # Clear target device
            target_device_name = self.target_device.name if self.target_device else None
            self.target_device = None
            cleanup_actions.append({"action": "clear_target_device", "success": True, "device_name": target_device_name})

            # Clear Athan playback state
            with self.playback_lock:
                was_playing = self.athan_playing
                self.athan_playing = False
                self.athan_start_time = None
                cleanup_actions.append({"action": "clear_athan_state", "success": True, "was_playing": was_playing})

            return {
                "success": True,
                "cleanup_actions": cleanup_actions,
                "message": "ChromecastManager cleanup completed successfully",
                "timestamp": datetime.now().isoformat()
            }

        except Exception as e:
            cleanup_actions.append({"action": "general_cleanup", "success": False, "error": str(e)})
            return {
                "success": False,
                "error": str(e),
                "cleanup_actions": cleanup_actions,
                "timestamp": datetime.now().isoformat()
            }

    def __del__(self):
        """Destructor to ensure cleanup."""
        try:
            self.cleanup()
        except:
            pass  # Ignore errors during cleanup in destructor


# Example Usage and Testing
if __name__ == "__main__":
    print("=== ChromecastManager JSON API Demo ===\n")

    chromecast_manager = ChromecastManager()

    # Get discovered devices
    devices_result = chromecast_manager.get_discovered_devices()
    print("Discovered devices:")
    print(json.dumps(devices_result, indent=2))

    # Get system status
    status_result = chromecast_manager.get_system_status()
    print("\nSystem status:")
    print(json.dumps(status_result, indent=2))

    # Test media URL generation
    url_result = chromecast_manager._get_media_url("test.mp3")
    print("\nMedia URL generation:")
    print(json.dumps(url_result, indent=2))

    # Example MP3 stream URL
    mp3_url = "https://n03.radiojar.com/8s5u5tpdtwzuv?rj-ttl=5&rj-tok=AAABlVflrAAAJLe-IOoD4VTShA"

    # Test playback (commented out to avoid actual playback during demo)
    # playback_result = chromecast_manager.play_url_on_cast(mp3_url)
    # print("\nPlayback result:")
    # print(json.dumps(playback_result, indent=2))

    print("\n=== ChromecastManager JSON API Demo Complete ===")
