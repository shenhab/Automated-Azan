import pychromecast
from pychromecast.discovery import CastBrowser, SimpleCastListener
import logging
import time
from typing import Optional
import socket
import threading

# Configure logging
logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s")

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
        self.discover_devices()

    def discover_devices(self, force_rediscovery=False):
        """Discovers all available Chromecast devices using CastBrowser."""
        current_time = time.time()
        
        # Skip discovery if within cooldown period (unless forced)
        if not force_rediscovery and (current_time - self.last_discovery_time) < self.discovery_cooldown:
            logging.debug(f"Skipping device discovery (cooldown: {self.discovery_cooldown}s)")
            return
            
        logging.info("Discovering Chromecast devices using CastBrowser...")
        self.last_discovery_time = current_time

        try:
            # Stop previous browser if exists
            if self.browser:
                self.browser.stop_discovery()
                
            # Clear previous discoveries
            self.chromecasts.clear()
            self.discovery_complete.clear()
            
            # Create a cast listener
            self.listener = SimpleCastListener(self._add_cast, self._remove_cast)
            
            # Create and start browser
            self.browser = CastBrowser(self.listener, None, None)
            self.browser.start_discovery()
            
            # Wait for discovery to find devices (timeout after 10 seconds)
            if self.discovery_complete.wait(timeout=10):
                logging.info(f"Discovery completed. Found {len(self.chromecasts)} devices")
            else:
                logging.info(f"Discovery timeout reached. Found {len(self.chromecasts)} devices so far")
            
            if not self.chromecasts:
                logging.warning("No Chromecast devices found.")
            else:
                logging.info(f"Found {len(self.chromecasts)} Chromecast devices:")
                for uuid, cast_info in self.chromecasts.items():
                    logging.info(f" - {cast_info['name']} ({cast_info['model_name']}) at {cast_info['host']}:{cast_info['port']}")
                    
        except Exception as e:
            logging.error(f"Error during device discovery: {e}")
            self.chromecasts.clear()
            
    def _add_cast(self, uuid, service):
        """Callback when a new cast device is discovered."""
        try:
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
                    # Create Chromecast instance
                    cast_device = pychromecast.get_chromecast_from_service(
                        cast_info['service'],
                        zconf=self.browser.zc
                    )
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
            try:
                cast_device = pychromecast.get_chromecast_from_service(
                    cast_info['service'],
                    zconf=self.browser.zc
                )
                self.target_device = cast_device  # Cache the device
                return cast_device
            except Exception as e:
                logging.error(f"Error creating Chromecast instance for {cast_info['name']}: {e}")

        # If no suitable devices found and retry is enabled, try rediscovering
        if retry_discovery:
            logging.warning("No suitable device found, attempting rediscovery...")
            self.discover_devices(force_rediscovery=True)
            return self._find_casting_candidate(retry_discovery=False)  # Avoid infinite recursion

        # No suitable devices found
        logging.warning("No suitable Chromecast candidate found.")
        self.target_device = None
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
                logging.warning(f"Connection attempt {attempt + 1} failed for {device.name}: {e}")
                if attempt < max_retries - 1:
                    time.sleep(2)  # Wait before retry
                    # Try rediscovering the device
                    self.discover_devices(force_rediscovery=True)
                    device = self._find_casting_candidate(retry_discovery=False)
                    if not device:
                        logging.error("Device no longer available after rediscovery")
                        return False
        
        logging.error(f"Failed to connect to {device.name} after {max_retries} attempts")
        return False

    def play_url_on_cast(self, url, max_retries=2):
        """
        Plays an MP3 URL on the selected Chromecast device with robust error handling.

        :param url: The media URL to play.
        :param max_retries: Maximum number of retries for playback
        """
        for retry in range(max_retries + 1):
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
                    media_controller.update_status(blocking=True, timeout=5)
                    if media_controller.status.content_id:
                        logging.info("Stopping previous media session...")
                        media_controller.stop()
                        time.sleep(2)  # Allow Chromecast to process stop request
                except Exception as e:
                    logging.warning(f"Error checking/stopping previous media: {e}")

                # Send the media play request
                logging.info(f"Streaming {url} on {target_device.name}...")
                media_controller.play_media(url, "audio/mp3")

                # Wait for Chromecast to fully load the media with better error handling
                success = self._wait_for_media_load(media_controller, url)
                
                if success:
                    logging.info("Media playback started successfully.")
                    return True
                else:
                    logging.warning(f"Media failed to load on attempt {retry + 1}")
                    if retry < max_retries:
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
                media_controller.update_status(blocking=True, timeout=3)

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
        adahn_url = "https://www.gurutux.com/media/adhan_al_fajr.mp3"
        success = self.play_url_on_cast(adahn_url)
        if not success:
            logging.error("Failed to play Fajr Adhan")
        return success
    
    def start_adahn(self):
        """Start regular Adhan with error handling."""
        adahn_url = "https://www.gurutux.com/media/Athan.mp3"
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
