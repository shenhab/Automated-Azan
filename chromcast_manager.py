import pychromecast
import logging
import time

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
        self.chromecasts = []
        self.discover_devices()

    def discover_devices(self):
        """Discovers all available Chromecast devices using get_chromecasts()."""
        logging.info("Discovering Chromecast devices...")

        # Discover devices using the deprecated method (since CastBrowser is unreliable)
        self.chromecasts, browser = pychromecast.get_chromecasts()

        if not self.chromecasts:
            logging.warning("No Chromecast devices found.")
        else:
            logging.info(f"Found {len(self.chromecasts)} Chromecast devices.")
            for cast in self.chromecasts:
                logging.info(f" - {cast.name} ({cast.model_name})")

    def _find_casting_candidate(self):
        """
        Finds a suitable Chromecast device to cast to.

        :return: A Chromecast device or None if no suitable device is found.
        """
        candidate_list = []  # Stores Google Nest Mini devices as a fallback

        for candidate in self.chromecasts:
            logging.debug(f"Checking device: {candidate.name} ({candidate.model_name})")

            # Check if the device name matches 'Adahn' (case-insensitive)
            if candidate.name.lower() == 'adahn':
                logging.info(f"Found target Chromecast: {candidate.name}")
                return candidate  # Return the device immediately

            # If it's a Google Nest Mini, add it to the candidate list
            elif candidate.model_name.strip() in ["Google Nest Mini", "Google Nest Hub"]:
                logging.info(f"Adding Google Nest Mini to candidate list: {candidate.name}")
                candidate_list.append(candidate)

        # If no 'Adahn' was found but we have Google Nest Mini devices, return the first one
        if candidate_list:
            logging.info(f"Using fallback Chromecast: {candidate_list[0].name}")
            return candidate_list[0]

        # No suitable devices found
        logging.warning("No suitable Chromecast candidate found.")
        return None

    def play_url_on_cast(self, url):
        """
        Plays an MP3 URL on the selected Chromecast device.

        :param url: The media URL to play.
        """
        target_device = self._find_casting_candidate()

        if not target_device:
            logging.error("No available Chromecast device to play the media.")
            return

        # Connect to the selected Chromecast device
        target_device.wait()
        logging.info(f"Connected to Chromecast: {target_device.name}")

        # Get media controller
        media_controller = target_device.media_controller

        # Check if a media session is already active before stopping
        media_controller.update_status()
        if media_controller.status.content_id:
            logging.info("Stopping previous media session...")
            media_controller.stop()
            time.sleep(2)  # Allow Chromecast to process stop request

        # Send the media play request
        logging.info(f"Streaming {url} on {target_device.name}...")
        media_controller.play_media(url, "audio/mp3")

        # Wait for Chromecast to fully load the media
        max_attempts = 15  # Max retries (~30 seconds)
        attempts = 0

        while attempts < max_attempts:
            media_controller.update_status()

            # Debugging: Show player state
            logging.info(f"Attempt {attempts+1}: Player State - {media_controller.status.player_state}")

            # Ensure Chromecast has received the media
            if media_controller.status.content_id == url and media_controller.status.player_state in ["BUFFERING", "PLAYING"]:
                logging.info("Media session is active. Starting playback...")
                media_controller.play()
                logging.info("Playback started.")
                return

            logging.info("Waiting for media to load...")
            time.sleep(2)  # Give Chromecast time to process
            attempts += 1

        logging.error("Media failed to load. Chromecast did not start playing.")
        
    def start_quran_radio(self):
        quran_radio_streaming = "https://n03.radiojar.com/8s5u5tpdtwzuv?rj-ttl=5&rj-tok=AAABlVflrAAAJLe-IOoD4VTShA"
        self.play_url_on_cast(quran_radio_streaming)
        
    def start_adahn_alfajr(self):
        adahn_url = "https://www.gurutux.com/media/adhan_al_fajr.mp3"
        self.play_url_on_cast(adahn_url)
    
    def start_adahn(self):
        adahn_url = "https://www.gurutux.com/media/Athan.mp3"
        self.play_url_on_cast(adahn_url)
        



# Example Usage
if __name__ == "__main__":
    chromecast_manager = ChromecastManager()

    # Example MP3 stream URL
    mp3_url = "https://n03.radiojar.com/8s5u5tpdtwzuv?rj-ttl=5&rj-tok=AAABlVflrAAAJLe-IOoD4VTShA"

    chromecast_manager.play_url_on_cast(mp3_url)
