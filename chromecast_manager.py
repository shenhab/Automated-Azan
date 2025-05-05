import pychromecast
from pychromecast.discovery import CastBrowser, SimpleCastListener
import logging
import time
import socket
import zeroconf

# Configure logging
logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s")

# Enable more detailed logging for pychromecast
DEBUG_MODE = False
if DEBUG_MODE:
    logging.getLogger('pychromecast').setLevel(logging.DEBUG)

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
        self.chromecasts = []
        self.browser = None
        self.zconf = None
        # self.discover_devices()

    def discover_devices(self):
        """Discovers all available Chromecast devices using CastBrowser."""
        logging.info("Discovering Chromecast devices...")
        
        # Try discovering using the modern approach exactly as shown in the README
        try:
            # Initialize zeroconf
            self.zconf = zeroconf.Zeroconf()
            
            # Create a callback function to handle discovered devices
            def device_callback(uuid, service):
                if hasattr(self.browser, 'devices') and uuid in self.browser.devices:
                    device = self.browser.devices[uuid]
                    logging.info(f"Found device: {device.friendly_name}")
            
            # Create the listener with the callback
            listener = SimpleCastListener(device_callback)
            
            # Create the browser and start discovery
            self.browser = CastBrowser(listener, self.zconf)
            self.browser.start_discovery()
            
            # Wait for devices to be discovered
            logging.info("Waiting for devices to be discovered...")
            time.sleep(8)
            
            # Get the device information from the browser
            if hasattr(self.browser, 'devices') and self.browser.devices:
                logging.info(f"CastBrowser found {len(self.browser.devices)} devices")
                
                # Get chromecast instances using get_listed_chromecasts 
                # This approach uses the browser data but creates proper Chromecast instances
                device_names = [device.friendly_name for device in self.browser.devices.values()]
                logging.info(f"Getting Chromecast instances for: {', '.join(device_names)}")
                
                self.chromecasts, browser = pychromecast.get_listed_chromecasts(
                    friendly_names=device_names,
                    timeout=10
                )
                
                if self.chromecasts:
                    logging.info(f"Successfully created {len(self.chromecasts)} Chromecast instances")
                    return
                else:
                    logging.warning("Failed to create Chromecast instances from discovered devices")
            else:
                logging.warning("CastBrowser found no devices or is missing device info")
                
            # Stop the browser since we'll try a different approach
            self._stop_browser()
            
        except Exception as e:
            logging.warning(f"Error using CastBrowser for discovery: {e}")
            self._stop_browser()
        
        # If the modern approach failed, try the get_chromecasts method
        logging.info("Using get_chromecasts() method to discover devices")
        try:
            self.chromecasts, browser = pychromecast.get_chromecasts(timeout=10)
            if browser:
                self.browser = browser
        except Exception as e:
            logging.error(f"Error in get_chromecasts(): {e}")
            self.chromecasts = []

        # Log the results
        if not self.chromecasts:
            logging.warning("No Chromecast devices found with any method.")
        else:
            logging.info(f"Found {len(self.chromecasts)} Chromecast devices.")
            for cast in self.chromecasts:
                logging.info(f" - {cast.name} ({cast.model_name})")
    
    def _stop_browser(self):
        """Stop the browser safely if it exists."""
        if self.browser:
            try:
                pychromecast.discovery.stop_discovery(self.browser)
                self.browser = None
            except Exception as e:
                logging.error(f"Error stopping browser: {e}")
                
        if self.zconf:
            try:
                self.zconf.close()
                self.zconf = None
            except Exception as e:
                logging.error(f"Error closing zeroconf: {e}")
    
    def cleanup(self):
        """Clean up resources when done."""
        self._stop_browser()

    def _find_casting_candidate(self, device_name=None):
        """
        Finds a suitable Chromecast device to cast to.

        :return: A Chromecast device or None if no suitable device is found.
        """
        candidate_list = []  # Stores Google Nest Mini devices as a fallback

        if not self.chromecasts:
            logging.warning("No Chromecast devices found.")
            return None

        for candidate in self.chromecasts:
            # Handle cases where name or model_name might be missing
            if not hasattr(candidate, 'name') or not candidate.name:
                continue
                
            logging.debug(f"Checking device: {candidate.name} ({getattr(candidate, 'model_name', 'Unknown')})")

            # If a specific device name is provided, check for it
            if device_name and candidate.name.lower() == device_name.lower():
                logging.info(f"Found target Chromecast: {candidate.name}")
                return candidate
            # If no specific device name is provided, check for 'Adahn'
            elif not device_name and candidate.name.lower() == 'adahn':
                logging.info(f"Found target Chromecast: {candidate.name}")
                return candidate
            # If it's a Google Nest Mini or Hub, add it to the candidate list
            model_name = getattr(candidate, 'model_name', '')
            if model_name and model_name.strip() in ["Google Nest Mini", "Google Nest Hub"]:
                logging.info(f"Adding Google Nest Mini/Hub to candidate list: {candidate.name}")
                candidate_list.append(candidate)

        # If no 'Adahn' was found but we have Google Nest Mini devices, return the first one
        if len(candidate_list) > 0:
            logging.info(f"Using fallback Chromecast: {candidate_list[0].name}")
            return candidate_list[0]
        else:
            logging.info("No suitable Chromecast candidate found.")
            # If no 'Adahn' or Google Nest Mini was found, return first Chromecast in the list
            return self.chromecasts[0]

    def play_url_on_cast(self, url, device_name=None):
        """
        Plays an MP3 URL on the selected Chromecast device.

        :param url: The media URL to play.
        :param device_name: Optional name of the device to cast to.
        """
        # If no devices were found during initialization, try discovering again
        if not self.chromecasts:
            logging.info("No devices available. Attempting to rediscover...")
            self.discover_devices()
            
        if device_name:
            target_device = self._find_casting_candidate(device_name)
        else:
            target_device = self._find_casting_candidate()

        if not target_device:
            logging.error("No available Chromecast device to play the media.")
            return False

        # Connect to the selected Chromecast device
        try:
            target_device.wait()
            logging.info(f"Connected to Chromecast: {target_device.name}")
        except Exception as e:
            logging.error(f"Error connecting to Chromecast: {e}")
            return False

        # Get media controller
        media_controller = target_device.media_controller

        # Check if a media session is already active before stopping
        try:
            media_controller.update_status()
            if media_controller.status.content_id:
                logging.info("Stopping previous media session...")
                media_controller.stop()
                time.sleep(2)  # Allow Chromecast to process stop request
        except:
            pass  # Ignore if we can't check or stop previous media

        # Send the media play request
        logging.info(f"Streaming {url} on {target_device.name}...")
        try:
            media_controller.play_media(url, "audio/mp3")
        except Exception as e:
            logging.error(f"Error playing media: {e}")
            return False

        # Wait for Chromecast to fully load the media
        max_attempts = 15  # Max retries (~30 seconds)
        attempts = 0

        while attempts < max_attempts:
            try:
                media_controller.update_status()

                # Debugging: Show player state
                logging.info(f"Attempt {attempts+1}: Player State - {media_controller.status.player_state}")

                # Ensure Chromecast has received the media
                if media_controller.status.content_id == url and media_controller.status.player_state in ["BUFFERING", "PLAYING"]:
                    logging.info("Media session is active. Starting playback...")
                    media_controller.play()
                    logging.info("Playback started.")
                    return True
            except Exception as e:
                logging.error(f"Error checking media status: {e}")
                
            logging.info("Waiting for media to load...")
            time.sleep(2)  # Give Chromecast time to process
            attempts += 1

        logging.error("Media failed to load. Chromecast did not start playing.")
        return False
        
    def start_quran_radio(self, device_name=None):
        """
        Starts streaming Quran radio on the Chromecast device.
        :param device_name: Optional name of the device to cast to.
        """
        quran_radio_streaming = "https://n03.radiojar.com/8s5u5tpdtwzuv?rj-ttl=5&rj-tok=AAABlVflrAAAJLe-IOoD4VTShA"
        return self.play_url_on_cast(quran_radio_streaming, device_name)
        
    def start_adahn_alfajr(self, device_name=None):
        """ 
        Starts streaming Adhan Al Fajr on the Chromecast device.
        :param device_name: Optional name of the device to cast to.
        """
        adahn_url = "https://www.gurutux.com/media/adhan_al_fajr.mp3"
        return self.play_url_on_cast(adahn_url, device_name)
    
    def start_adahn(self, device_name=None):
        adahn_url = "https://www.gurutux.com/media/Athan.mp3"
        return self.play_url_on_cast(adahn_url, device_name)
        
    def __del__(self):
        """Clean up resources when the object is destroyed."""
        self.cleanup()


# Example Usage
if __name__ == "__main__":
    chromecast_manager = ChromecastManager()

    # Example MP3 stream URL
    mp3_url = "https://n03.radiojar.com/8s5u5tpdtwzuv?rj-ttl=5&rj-tok=AAABlVflrAAAJLe-IOoD4VTShA"

    chromecast_manager.play_url_on_cast(mp3_url)
