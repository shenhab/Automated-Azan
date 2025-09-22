"""
Chromecast Media Playback Module

This module handles media playback operations with improved modularity
and error handling.
"""

import logging
import time
import socket
import threading
from typing import Optional, Dict, Any, List
from datetime import datetime
from enum import Enum

# Import from parent directory
import sys
import os
sys.path.append(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from chromecast_config import ChromecastConfig, get_config
from chromecast_exceptions import (
    MediaLoadError, MediaTimeoutError, PlaybackAlreadyActiveError,
    InvalidMediaURLError, AthanAlreadyPlayingError, AthanPlaybackFailedError
)
from chromecast_models import (
    CastDevice, PlayerState, PrayerType, PlaybackState,
    PlaybackResponse, MediaLoadResponse, StatusCheck,
    PlaybackAttempt, AthanPlaybackResponse, AthanStatusResponse
)


class MediaController:
    """
    Handles media playback operations on Chromecast devices.

    This class breaks down the monolithic play_url_on_cast method
    into smaller, focused methods for better maintainability.
    """

    def __init__(
        self,
        connection_pool,
        config: Optional[ChromecastConfig] = None
    ):
        """
        Initialize MediaController.

        Args:
            connection_pool: DeviceConnectionPool instance
            config: Configuration object
        """
        self.connection_pool = connection_pool
        self.config = config or get_config()
        self.playback_lock = threading.Lock()

    def play_media(
        self,
        device: CastDevice,
        url: str,
        content_type: str = "audio/mpeg",
        retry_on_failure: bool = True
    ) -> PlaybackResponse:
        """
        Play media on a Chromecast device.

        Args:
            device: Target device
            url: Media URL to play
            content_type: MIME type of media
            retry_on_failure: Whether to retry on failure

        Returns:
            PlaybackResponse with playback status
        """
        if not self._validate_media_url(url):
            raise InvalidMediaURLError(url)

        max_retries = self.config.PLAYBACK_MAX_RETRIES if retry_on_failure else 0
        playback_attempts: List[PlaybackAttempt] = []
        start_time = time.time()

        for attempt in range(max_retries + 1):
            attempt_start = time.time()

            try:
                # Get connection from pool
                cast_object = self.connection_pool.get_connection(device)
                if not cast_object:
                    raise MediaLoadError(url, device.name, "Failed to get connection")

                # Stop any existing media
                self._stop_existing_media(cast_object, url)

                # Load new media
                self._load_media(cast_object, url, content_type)

                # Wait for media to start
                load_result = self._wait_for_media_start(cast_object, url)

                if load_result['success']:
                    total_time = time.time() - start_time
                    return {
                        'success': True,
                        'timestamp': datetime.now().isoformat(),
                        'error': None,
                        'url': url,
                        'device': {
                            'name': device.name,
                            'model': device.model_name,
                            'host': device.host,
                            'port': device.port
                        },
                        'device_source': 'connection_pool',
                        'attempts': attempt + 1,
                        'total_time': round(total_time, 2),
                        'connection_result': None,
                        'load_result': load_result,
                        'playback_attempts': playback_attempts
                    }

                # Media failed to load
                attempt_info: PlaybackAttempt = {
                    'attempt': attempt + 1,
                    'error': "Media failed to load",
                    'attempt_time': round(time.time() - attempt_start, 2),
                    'connection_result': None,
                    'load_result': load_result
                }
                playback_attempts.append(attempt_info)

                if attempt < max_retries:
                    logging.warning(f"Retrying playback (attempt {attempt + 2}/{max_retries + 1})")
                    time.sleep(self.config.CONNECTION_RETRY_DELAY)

            except Exception as e:
                attempt_info = {
                    'attempt': attempt + 1,
                    'error': str(e),
                    'attempt_time': round(time.time() - attempt_start, 2),
                    'connection_result': None,
                    'load_result': None
                }
                playback_attempts.append(attempt_info)

                logging.error(f"Playback attempt {attempt + 1} failed: {e}")

                if attempt < max_retries:
                    time.sleep(self.config.CONNECTION_RETRY_DELAY)
                else:
                    return {
                        'success': False,
                        'timestamp': datetime.now().isoformat(),
                        'error': str(e),
                        'url': url,
                        'device': {
                            'name': device.name,
                            'model': device.model_name,
                            'host': device.host,
                            'port': device.port
                        },
                        'device_source': None,
                        'attempts': attempt + 1,
                        'total_time': None,
                        'connection_result': None,
                        'load_result': None,
                        'playback_attempts': playback_attempts
                    }

        # All retries exhausted
        return {
            'success': False,
            'timestamp': datetime.now().isoformat(),
            'error': "All playback attempts failed",
            'url': url,
            'device': None,
            'device_source': None,
            'attempts': max_retries + 1,
            'total_time': None,
            'connection_result': None,
            'load_result': None,
            'playback_attempts': playback_attempts
        }

    def _validate_media_url(self, url: str) -> bool:
        """Validate media URL format"""
        if not url:
            return False

        # Basic URL validation
        if not (url.startswith('http://') or url.startswith('https://')):
            return False

        return True

    def _stop_existing_media(self, cast_object: Any, new_url: str) -> None:
        """Stop existing media if different from new URL"""
        try:
            media_controller = cast_object.media_controller
            media_controller.update_status()
            time.sleep(self.config.MEDIA_LOAD_INITIAL_WAIT)

            current_content = media_controller.status.content_id
            current_state = media_controller.status.player_state

            if current_content and current_content != new_url:
                if current_state not in ["IDLE", "UNKNOWN"]:
                    logging.info(f"Stopping different media: {current_content}")
                    media_controller.stop()
                    time.sleep(self.config.MEDIA_STOP_WAIT)
            elif current_content == new_url:
                logging.info("Same media already loaded, restarting...")
                media_controller.stop()
                time.sleep(self.config.MEDIA_RESTART_WAIT)

        except Exception as e:
            logging.debug(f"Error checking previous media: {e}")

    def _load_media(self, cast_object: Any, url: str, content_type: str) -> None:
        """Load media on the cast device"""
        media_controller = cast_object.media_controller
        logging.info(f"Loading media: {url}")
        media_controller.play_media(url, content_type)

    def _wait_for_media_start(self, cast_object: Any, url: str) -> MediaLoadResponse:
        """
        Wait for media to start playing.

        Args:
            cast_object: Chromecast object
            url: Expected media URL

        Returns:
            MediaLoadResponse with load status
        """
        media_controller = cast_object.media_controller
        status_checks: List[StatusCheck] = []
        last_player_state = None
        consecutive_good_states = 0

        for attempt in range(self.config.MEDIA_LOAD_MAX_ATTEMPTS):
            check_start = time.time()

            try:
                media_controller.update_status()
                time.sleep(self.config.MEDIA_LOAD_INITIAL_WAIT)

                player_state = media_controller.status.player_state
                content_id = media_controller.status.content_id

                status_check: StatusCheck = {
                    'attempt': attempt + 1,
                    'player_state': player_state,
                    'content_id': content_id,
                    'expected_url': url,
                    'content_matches': content_id == url,
                    'check_time': round(time.time() - check_start, 2),
                    'error': None
                }
                status_checks.append(status_check)

                logging.debug(f"Media status check {attempt + 1}: {player_state}")

                # Check for success conditions
                if player_state in ["BUFFERING", "PLAYING"]:
                    if last_player_state in ["BUFFERING", "PLAYING"]:
                        consecutive_good_states += 1
                    else:
                        consecutive_good_states = 1

                    if consecutive_good_states >= self.config.CONSECUTIVE_PLAYING_THRESHOLD or \
                       player_state == "PLAYING":
                        # Try to ensure playback
                        if player_state == "BUFFERING":
                            try:
                                media_controller.play()
                            except Exception:
                                pass

                        return {
                            'success': True,
                            'timestamp': datetime.now().isoformat(),
                            'error': None,
                            'player_state': player_state,
                            'content_id': content_id,
                            'attempts': attempt + 1,
                            'consecutive_good_states': consecutive_good_states,
                            'status_checks': status_checks,
                            'final_state': None
                        }
                else:
                    consecutive_good_states = 0

                # Check for stuck IDLE state
                if player_state == "IDLE" and attempt >= self.config.IDLE_STATE_CONCERN_THRESHOLD:
                    logging.warning(f"Media stuck in IDLE after {attempt} attempts")

                last_player_state = player_state

                # Progressive wait times
                if attempt < 3:
                    time.sleep(self.config.MEDIA_LOAD_SHORT_WAIT)
                elif attempt < 6:
                    time.sleep(self.config.MEDIA_LOAD_MEDIUM_WAIT)
                else:
                    time.sleep(self.config.MEDIA_LOAD_LONG_WAIT)

            except Exception as e:
                status_check = {
                    'attempt': attempt + 1,
                    'player_state': None,
                    'content_id': None,
                    'expected_url': url,
                    'content_matches': False,
                    'check_time': round(time.time() - check_start, 2),
                    'error': str(e)
                }
                status_checks.append(status_check)
                logging.warning(f"Error checking media status: {e}")

        # Timeout reached
        return {
            'success': False,
            'timestamp': datetime.now().isoformat(),
            'error': "Media loading timeout",
            'player_state': None,
            'content_id': None,
            'attempts': self.config.MEDIA_LOAD_MAX_ATTEMPTS,
            'consecutive_good_states': None,
            'status_checks': status_checks,
            'final_state': last_player_state
        }

    def stop_media(self, device: CastDevice) -> Dict[str, Any]:
        """
        Stop media playback on a device.

        Args:
            device: Device to stop playback on

        Returns:
            Response with stop status
        """
        try:
            cast_object = self.connection_pool.get_connection(device)
            if cast_object:
                media_controller = cast_object.media_controller
                media_controller.stop()
                logging.info(f"Stopped media on {device.name}")
                return {
                    'success': True,
                    'device_name': device.name,
                    'message': "Media playback stopped",
                    'timestamp': datetime.now().isoformat()
                }
        except Exception as e:
            logging.error(f"Error stopping media on {device.name}: {e}")
            return {
                'success': False,
                'device_name': device.name,
                'error': str(e),
                'timestamp': datetime.now().isoformat()
            }


class AthanController:
    """
    Specialized controller for Athan playback with collision prevention.
    """

    def __init__(
        self,
        media_controller: MediaController,
        config: Optional[ChromecastConfig] = None
    ):
        """
        Initialize AthanController.

        Args:
            media_controller: MediaController instance
            config: Configuration object
        """
        self.media_controller = media_controller
        self.config = config or get_config()
        self.playback_state = PlaybackState()
        self.playback_lock = threading.Lock()

    def play_athan(
        self,
        device: CastDevice,
        prayer_type: PrayerType = PrayerType.REGULAR
    ) -> AthanPlaybackResponse:
        """
        Play Athan with collision prevention.

        Args:
            device: Device to play on
            prayer_type: Type of prayer

        Returns:
            AthanPlaybackResponse
        """
        with self.playback_lock:
            # Check if already playing
            if self.playback_state.is_playing:
                elapsed = self.playback_state.get_elapsed_time()

                # Check for timeout
                if elapsed > self.config.ATHAN_TIMEOUT_SECONDS:
                    logging.info("Athan timeout reached, clearing state")
                    self.playback_state.clear()
                else:
                    raise AthanAlreadyPlayingError(prayer_type, elapsed)

            # Get media URL
            media_url = self._get_media_url(prayer_type)

            # Mark as playing
            self.playback_state.is_playing = True
            self.playback_state.start_time = datetime.now()
            self.playback_state.media_url = media_url
            self.playback_state.prayer_type = prayer_type
            self.playback_state.device_name = device.name

        # Play media (outside lock to avoid blocking)
        try:
            playback_result = self.media_controller.play_media(
                device, media_url, "audio/mpeg"
            )

            if playback_result['success']:
                return {
                    'success': True,
                    'timestamp': datetime.now().isoformat(),
                    'error': None,
                    'prayer_type': prayer_type,
                    'media_url': media_url,
                    'start_time': self.playback_state.start_time.isoformat(),
                    'playback_result': playback_result,
                    'skipped': False,
                    'reason': None,
                    'current_status': None,
                    'message': f"Started {prayer_type} Athan successfully"
                }
            else:
                # Clear state on failure
                with self.playback_lock:
                    self.playback_state.clear()

                raise AthanPlaybackFailedError(
                    prayer_type,
                    playback_result.get('error', 'Unknown error')
                )

        except Exception as e:
            # Clear state on error
            with self.playback_lock:
                self.playback_state.clear()
            raise

    def stop_athan(self, device: CastDevice) -> Dict[str, Any]:
        """Stop Athan playback"""
        with self.playback_lock:
            was_playing = self.playback_state.is_playing
            elapsed_time = self.playback_state.get_elapsed_time()
            self.playback_state.clear()

        if was_playing:
            result = self.media_controller.stop_media(device)
            return {
                'success': result['success'],
                'was_playing': True,
                'elapsed_time': round(elapsed_time, 1),
                'device_name': device.name,
                'message': "Athan stopped successfully",
                'timestamp': datetime.now().isoformat()
            }

        return {
            'success': True,
            'was_playing': False,
            'elapsed_time': 0,
            'device_name': device.name,
            'message': "No Athan was playing",
            'timestamp': datetime.now().isoformat()
        }

    def get_status(self) -> AthanStatusResponse:
        """Get current Athan playback status"""
        with self.playback_lock:
            if not self.playback_state.is_playing:
                return {
                    'success': True,
                    'timestamp': datetime.now().isoformat(),
                    'error': None,
                    'playing': False,
                    'elapsed_time': None,
                    'start_time': None,
                    'device_name': None,
                    'message': "No Athan currently playing"
                }

            elapsed = self.playback_state.get_elapsed_time()

            # Check timeout
            if elapsed > self.config.ATHAN_TIMEOUT_SECONDS:
                self.playback_state.clear()
                return {
                    'success': True,
                    'timestamp': datetime.now().isoformat(),
                    'error': None,
                    'playing': False,
                    'elapsed_time': None,
                    'start_time': None,
                    'device_name': None,
                    'message': "Athan timeout reached"
                }

            return {
                'success': True,
                'timestamp': datetime.now().isoformat(),
                'error': None,
                'playing': True,
                'elapsed_time': round(elapsed, 1),
                'start_time': self.playback_state.start_time.isoformat(),
                'device_name': self.playback_state.device_name,
                'message': f"Athan playing for {round(elapsed, 1)} seconds"
            }

    def _get_media_url(self, prayer_type: PrayerType) -> str:
        """Get media URL for prayer type"""
        # Get local IP
        try:
            s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            s.settimeout(0)
            s.connect(('10.254.254.254', 1))
            local_ip = s.getsockname()[0]
            s.close()
        except Exception:
            local_ip = '127.0.0.1'

        # Get filename based on prayer type
        if prayer_type == PrayerType.FAJR:
            filename = self.config.ATHAN_FAJR_FILENAME
        else:
            filename = self.config.ATHAN_REGULAR_FILENAME

        return f"http://{local_ip}:{self.config.WEB_INTERFACE_PORT}/media/{filename}"