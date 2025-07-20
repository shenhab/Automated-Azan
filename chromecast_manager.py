import pychromecast
from pychromecast.discovery import CastBrowser, SimpleCastListener
import logging
import time
from typing import Optional
import socket
import threading

# Get logger (don't configure here - let main.py handle it)

class ChromecastManager:
    """
    A class to manage Chromecast devices and find the best candidate for casting.

    Priority order:
    1. If a Chromecast device named 'Adahn' is found, it is used.
    2. Otherwise, if any Google Nest Mini devices are found, the first one is used.
    3. If no suitable devices are found, an error is logged.

    Supports playing media URLs on the selected Chromecast.
    """

    def __init__(self):
        """Initialize the ChromecastManager and discover devices."""
        self.chromecasts = {}  # Dictionary to store discovered devices
        self.target_device = None  # Cache the target device
        self.last_discovery_time = 0
        self.discovery_cooldown = 30  # 30 seconds between rediscoveries
        self.browser = None
        self.listener = None
        self.discovery_complete = threading.Event()
        
        # Discover devices on initialization
        self.discover_devices()

    def _get_media_url(self, filename):
        """Get the appropriate media URL for the given filename."""
        # Try to determine the correct base URL
        import socket
        
        # Get the local IP address that the web interface is running on
        try:
            # Create a socket to determine local IP
            s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            s.settimeout(0)
            try:
                # Connect to a non-existent IP to trigger routing
                s.connect(('10.254.254.254', 1))
                local_ip = s.getsockname()[0]
            except Exception:
                local_ip = '127.0.0.1'
            finally:
                s.close()
        except Exception:
            local_ip = '127.0.0.1'
        
        # Return the local media URL
        return f"http://{local_ip}:5000/media/{filename}"

    def discover_devices(self, force_rediscovery=False):
        """Discovers all available Chromecast devices using CastBrowser with fallback."""
        current_time = time.time()
        
        # Skip discovery if within cooldown period (unless forced)
        if not force_rediscovery and (current_time - self.last_discovery_time) < self.discovery_cooldown:
            logging.debug(f"Skipping device discovery (cooldown: {self.discovery_cooldown}s)")
            return
            
        logging.info("Discovering Chromecast devices...")
        self.last_discovery_time = current_time

        # Try CastBrowser first (modern approach)
        if self._discover_with_castbrowser():
            return
            
        # Fallback to get_chromecasts() if CastBrowser fails
        logging.info("CastBrowser failed, falling back to get_chromecasts()...")
        self._discover_with_get_chromecasts()

    def _discover_with_castbrowser(self):
        """Try discovery with CastBrowser (modern approach) - FIXED VERSION."""
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
                    logging.info(f"✅ CastBrowser found {len(self.chromecasts)} Chromecast devices:")
                    for uuid, cast_info in self.chromecasts.items():
                        logging.info(f" - {cast_info['name']} ({cast_info['model_name']}) at {cast_info['host']}:{cast_info['port']}")
                    self.browser.stop_discovery()
                    return True
            
            # Method 2: If callbacks didn't work, use the working get_chromecasts() approach
            logging.debug("CastBrowser callbacks didn't work, using get_chromecasts() hybrid approach...")
            
            # Stop our non-working browser
            self.browser.stop_discovery()
            
            # Use get_chromecasts to get a properly working browser
            chromecasts, working_browser = pychromecast.get_chromecasts(timeout=8)
            logging.debug(f"get_chromecasts() found {len(chromecasts)} cast objects")
            
            if working_browser and hasattr(working_browser, 'devices') and working_browser.devices:
                logging.debug(f"Working browser has {len(working_browser.devices)} devices in storage")
                
                # Extract device information from the working browser
                devices_extracted = 0
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
                            logging.debug(f"Extracted: {device_info.friendly_name} ({cast_info['model_name']})")
                            
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
                        logging.debug(f"From cast object: {cast.name} ({cast_info['model_name']})")
                        
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
                logging.info(f"✅ CastBrowser (hybrid) found {len(self.chromecasts)} Chromecast devices:")
                for uuid, cast_info in self.chromecasts.items():
                    logging.info(f" - {cast_info['name']} ({cast_info['model_name']}) at {cast_info['host']}:{cast_info['port']}")
                return True
            else:
                logging.debug("CastBrowser hybrid approach found no devices")
                return False
                    
        except Exception as e:
            logging.debug(f"CastBrowser hybrid discovery failed: {e}")
            self.chromecasts.clear()
            return False

    def _poll_browser_devices(self):
        """Poll browser's internal device storage (fallback when callbacks don't work)."""
        # This method is no longer needed as we use the hybrid approach above
        # but keeping it for backwards compatibility
        return 0

    def _discover_with_get_chromecasts(self):
        """Fallback discovery using deprecated get_chromecasts()."""
        try:
            logging.debug("Using get_chromecasts() fallback method...")
            
            # Clear previous discoveries
            self.chromecasts.clear()
            
            # Use get_chromecasts for discovery
            chromecasts, browser = pychromecast.get_chromecasts()
            
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
                    
                except Exception as e:
                    logging.warning(f"Error processing cast device {cast.name}: {e}")
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
                logging.info(f"✅ Fallback method found {len(self.chromecasts)} Chromecast devices:")
                for uuid, cast_info in self.chromecasts.items():
                    logging.info(f" - {cast_info['name']} ({cast_info['model_name']}) at {cast_info['host']}:{cast_info['port']}")
            else:
                logging.warning("No Chromecast devices found with fallback method either.")
                
        except Exception as e:
            logging.error(f"Error during fallback device discovery: {e}")
            self.chromecasts.clear()
            
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

        :param retry_discovery: Whether to retry device discovery if no device is found
        :return: A Chromecast device or None if no suitable device is found.
        """
        # Try to use cached device first if it's still available
        if self.target_device and self._is_device_available_by_cast(self.target_device):
            logging.debug(f"Using cached device: {self.target_device.name}")
            return self.target_device
            
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
                        return cast_device

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
                return cast_device

        # If no suitable devices found and retry is enabled, try rediscovering
        if retry_discovery:
            logging.warning("No suitable device found, attempting rediscovery...")
            self.discover_devices(force_rediscovery=True)
            return self._find_casting_candidate(retry_discovery=False)  # Avoid infinite recursion

        # No suitable devices found
        logging.warning("No suitable Chromecast candidate found.")
        self.target_device = None
        return None

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
        """Check if a device info dict is still available and responsive."""
        try:
            # Quick network connectivity check
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            sock.settimeout(3)  # 3 second timeout
            result = sock.connect_ex((cast_info['host'], cast_info['port']))
            sock.close()
            return result == 0
        except Exception as e:
            logging.debug(f"Device availability check failed for {cast_info['name']}: {e}")
            return False
            
    def _is_device_available_by_cast(self, device):
        """Check if a Chromecast device instance is still available and responsive."""
        try:
            # Quick network connectivity check
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            sock.settimeout(3)  # 3 second timeout
            result = sock.connect_ex((device.host, device.port))
            sock.close()
            return result == 0
        except Exception as e:
            logging.debug(f"Device availability check failed for {device.name}: {e}")
            return False

    def _connect_with_retry(self, device, max_retries=3):
        """Connect to device with retry logic and error handling."""
        for attempt in range(max_retries):
            try:
                logging.info(f"Attempting to connect to {device.name} (attempt {attempt + 1}/{max_retries})")
                device.wait(timeout=10)  # 10 second timeout for connection
                logging.info(f"Successfully connected to Chromecast: {device.name}")
                return True
                
            except Exception as e:
                error_msg = str(e)
                logging.warning(f"Connection attempt {attempt + 1} failed for {device.name}: {error_msg}")
                
                # If it's a threading error, clear the cached device and force rediscovery
                if "threads can only be started once" in error_msg:
                    logging.info("Threading error detected, clearing cached device")
                    self.target_device = None
                    if attempt < max_retries - 1:
                        time.sleep(2)
                        self.discover_devices(force_rediscovery=True)
                        device = self._find_casting_candidate(retry_discovery=False)
                        if not device:
                            logging.error("Device no longer available after rediscovery")
                            return False
                elif attempt < max_retries - 1:
                    time.sleep(2)  # Wait before retry
                    # Try rediscovering the device for other types of errors
                    self.discover_devices(force_rediscovery=True)
                    device = self._find_casting_candidate(retry_discovery=False)
                    if not device:
                        logging.error("Device no longer available after rediscovery")
                        return False
        
        logging.error(f"Failed to connect to {device.name} after {max_retries} attempts")
        return False

    def play_url_on_cast(self, url, max_retries=2, preserve_target=False):
        """
        Plays an MP3 URL on the selected Chromecast device with robust error handling.

        :param url: The media URL to play.
        :param max_retries: Maximum number of retries for playback
        :param preserve_target: If True, don't clear target_device on retry (for specific device testing)
        """
        for retry in range(max_retries + 1):
            # Use existing target device if set, otherwise find one
            if hasattr(self, 'target_device') and self.target_device:
                target_device = self.target_device
                logging.debug("Using pre-set target device for playback")
            else:
                target_device = self._find_casting_candidate()

            if not target_device:
                logging.error("No available Chromecast device to play the media.")
                return False

            try:
                # Connect to the selected Chromecast device with retry logic
                if not self._connect_with_retry(target_device):
                    if retry < max_retries:
                        logging.info(f"Retrying playback... (attempt {retry + 2}/{max_retries + 1})")
                        continue
                    else:
                        logging.error("Failed to connect to device for playback")
                        return False

                # Get media controller
                media_controller = target_device.media_controller

                # Check if a media session is already active before stopping
                try:
                    media_controller.update_status()
                    time.sleep(1)  # Give time for status update
                    if media_controller.status.content_id:
                        logging.info("Stopping previous media session...")
                        media_controller.stop()
                        time.sleep(2)  # Allow Chromecast to process stop request
                except Exception as e:
                    logging.warning(f"Error checking/stopping previous media: {e}")

                # Send the media play request
                logging.info(f"Streaming {url} on {target_device.name}...")
                logging.debug(f"Device details - Host: {getattr(target_device, 'host', 'unknown')}, "
                            f"Port: {getattr(target_device, 'port', 'unknown')}, "
                            f"Model: {getattr(target_device, 'model_name', 'unknown')}")
                media_controller.play_media(url, "audio/mp3")

                # Wait for Chromecast to fully load the media with better error handling
                success = self._wait_for_media_load(media_controller, url)
                
                if success:
                    logging.info("Media playback started successfully.")
                    return True
                else:
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
                        logging.error("Media playback failed after all retries.")
                        return False

            except Exception as e:
                logging.error(f"Error during playback attempt {retry + 1}: {e}")
                if retry < max_retries:
                    if not preserve_target:
                        self.target_device = None  # Clear cached device on error
                    time.sleep(2)
                    continue
                else:
                    logging.error("Playback failed after all retries.")
                    return False

        return False

    def _wait_for_media_load(self, media_controller, url, max_attempts=15):
        """
        Wait for media to load with better error handling.
        
        :param media_controller: The media controller instance
        :param url: Expected content URL
        :param max_attempts: Maximum attempts to wait
        :return: True if media loaded successfully, False otherwise
        """
        attempts = 0

        while attempts < max_attempts:
            try:
                media_controller.update_status()
                time.sleep(0.5)  # Give time for status update

                # Debugging: Show player state
                logging.debug(f"Attempt {attempts+1}: Player State - {media_controller.status.player_state}")

                # Check if media is loading or playing
                if media_controller.status.content_id == url:
                    if media_controller.status.player_state in ["BUFFERING", "PLAYING"]:
                        logging.info("Media session is active. Starting playback...")
                        try:
                            media_controller.play()
                        except Exception as e:
                            logging.warning(f"Error calling play(): {e}")
                        return True
                    elif media_controller.status.player_state == "IDLE":
                        # Media might have failed to load
                        logging.warning("Media is in IDLE state, might have failed to load")
                
                logging.debug("Waiting for media to load...")
                time.sleep(2)  # Give Chromecast time to process
                attempts += 1
                
            except Exception as e:
                logging.warning(f"Error checking media status (attempt {attempts+1}): {e}")
                attempts += 1
                time.sleep(1)

        logging.error("Media failed to load within timeout period.")
        return False
        
    def start_quran_radio(self):
        """Start Quran radio stream with error handling."""
        quran_radio_streaming = "https://n03.radiojar.com/8s5u5tpdtwzuv?rj-ttl=5&rj-tok=AAABlVflrAAAJLe-IOoD4VTShA"
        success = self.play_url_on_cast(quran_radio_streaming)
        if not success:
            logging.error("Failed to start Quran radio stream")
        return success
        
    def start_adahn_alfajr(self):
        """Start Fajr Adhan with error handling."""
        # Use local media served by Flask web interface
        adahn_url = self._get_media_url("media_adhan_al_fajr.mp3")
        success = self.play_url_on_cast(adahn_url)
        if not success:
            logging.error("Failed to play Fajr Adhan")
        return success
    
    def start_adahn(self):
        """Start regular Adhan with error handling."""
        # Use local media served by Flask web interface
        adahn_url = self._get_media_url("media_Athan.mp3")
        success = self.play_url_on_cast(adahn_url)
        if not success:
            logging.error("Failed to play Adhan")
        return success

    def get_device_status(self):
        """Get status information about the current target device."""
        if not self.target_device:
            return {"status": "no_device", "message": "No target device selected"}
            
        try:
            if self._is_device_available_by_cast(self.target_device):
                return {
                    "status": "available",
                    "name": self.target_device.name,
                    "host": self.target_device.host,
                    "model": self.target_device.model_name
                }
            else:
                return {
                    "status": "unavailable",
                    "name": self.target_device.name,
                    "message": "Device is not responding"
                }
        except Exception as e:
            return {"status": "error", "message": str(e)}
            
    def cleanup(self):
        """Clean up resources."""
        if self.browser:
            try:
                self.browser.stop_discovery()
            except Exception as e:
                logging.warning(f"Error stopping browser: {e}")
        self.chromecasts.clear()
        self.target_device = None
        
    def __del__(self):
        """Destructor to ensure cleanup."""
        try:
            self.cleanup()
        except:
            pass  # Ignore errors during cleanup in destructor
        



# Example Usage
if __name__ == "__main__":
    chromecast_manager = ChromecastManager()

    # Example MP3 stream URL
    mp3_url = "https://n03.radiojar.com/8s5u5tpdtwzuv?rj-ttl=5&rj-tok=AAABlVflrAAAJLe-IOoD4VTShA"

    chromecast_manager.play_url_on_cast(mp3_url)
