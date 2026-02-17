"""
Unit Tests for Improved Chromecast Manager Components

This module contains comprehensive tests for the new modular Chromecast system.
"""

import unittest
from unittest.mock import Mock, MagicMock, patch, PropertyMock
import time
from datetime import datetime, timedelta
import sys
import os

# Add parent directory to path
sys.path.append(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from chromecast_config import ChromecastConfig
from chromecast_exceptions import (
    NoDevicesFoundError, DeviceConnectionError, CircuitBreakerOpenError,
    MediaLoadError, AthanAlreadyPlayingError
)
from chromecast_models import CastDevice, PlayerState, PrayerType, PlaybackState
from chromecast.circuit_breaker import CircuitBreaker, CircuitState
from chromecast.discovery import ChromecastDiscovery
from chromecast.connection import DeviceConnectionPool
from chromecast.playback import MediaController, AthanController
from chromecast.manager import ChromecastManager


class TestCircuitBreaker(unittest.TestCase):
    """Test circuit breaker pattern implementation"""

    def setUp(self):
        """Set up test circuit breaker"""
        self.breaker = CircuitBreaker(
            name="test_breaker",
            failure_threshold=3,
            recovery_timeout=1,
            expected_exception=ValueError
        )

    def test_circuit_breaker_closes_after_threshold(self):
        """Test that circuit opens after failure threshold"""
        def failing_func():
            raise ValueError("Test error")

        # First failures should pass through
        for i in range(3):
            with self.assertRaises(ValueError):
                self.breaker.call(failing_func)

        # Circuit should now be open
        self.assertEqual(self.breaker.state, CircuitState.OPEN)

        # Next call should raise CircuitBreakerOpenError
        with self.assertRaises(CircuitBreakerOpenError):
            self.breaker.call(failing_func)

    def test_circuit_breaker_half_open_recovery(self):
        """Test circuit breaker recovery through half-open state"""
        def sometimes_failing_func(should_fail):
            if should_fail:
                raise ValueError("Test error")
            return "success"

        # Open the circuit
        for i in range(3):
            with self.assertRaises(ValueError):
                self.breaker.call(sometimes_failing_func, True)

        self.assertEqual(self.breaker.state, CircuitState.OPEN)

        # Wait for recovery timeout
        time.sleep(1.1)

        # Next call should attempt half-open
        result = self.breaker.call(sometimes_failing_func, False)
        self.assertEqual(result, "success")
        self.assertEqual(self.breaker.state, CircuitState.CLOSED)

    def test_circuit_breaker_statistics(self):
        """Test circuit breaker statistics tracking"""
        def test_func(should_fail):
            if should_fail:
                raise ValueError("Test error")
            return "success"

        # Mix of successes and failures
        self.breaker.call(test_func, False)  # Success
        with self.assertRaises(ValueError):
            self.breaker.call(test_func, True)  # Failure

        status = self.breaker.get_status()
        self.assertEqual(status['successful_calls'], 1)
        self.assertEqual(status['total_calls'], 2)
        self.assertEqual(status['failure_count'], 1)


class TestDeviceDiscovery(unittest.TestCase):
    """Test device discovery functionality"""

    def setUp(self):
        """Set up test discovery"""
        self.config = ChromecastConfig()
        self.config.DISCOVERY_COOLDOWN_SECONDS = 1
        self.discovery = ChromecastDiscovery(self.config)

    @patch('pychromecast.get_chromecasts')
    def test_device_discovery_success(self, mock_get_chromecasts):
        """Test successful device discovery"""
        # Create mock devices
        mock_device1 = Mock()
        mock_device1.name = "Living Room"
        mock_device1.host = "192.168.1.100"
        mock_device1.port = 8009
        mock_device1.uuid = "uuid-1"
        mock_device1.model_name = "Chromecast"

        mock_device2 = Mock()
        mock_device2.name = "Adahn"
        mock_device2.host = "192.168.1.101"
        mock_device2.port = 8009
        mock_device2.uuid = "uuid-2"
        mock_device2.model_name = "Google Nest Mini"

        mock_browser = Mock()
        mock_browser.stop_discovery = Mock()

        mock_get_chromecasts.return_value = ([mock_device1, mock_device2], mock_browser)

        # Perform discovery
        result = self.discovery.discover_devices(force=True)

        self.assertTrue(result['success'])
        self.assertEqual(result['devices_found'], 2)
        self.assertIn('uuid-1', result['devices'])
        self.assertIn('uuid-2', result['devices'])

    def test_device_discovery_cooldown(self):
        """Test discovery cooldown period"""
        # First discovery should work
        with patch('pychromecast.get_chromecasts') as mock_get:
            mock_get.return_value = ([], Mock())
            self.discovery.discover_devices(force=True)

        # Second discovery within cooldown should be skipped
        result = self.discovery.discover_devices(force=False)
        self.assertTrue(result['skipped'])
        self.assertIn('Cooldown', result['reason'])

    def test_find_best_device_priority(self):
        """Test device selection priority"""
        # Create devices
        device1 = CastDevice(
            uuid="uuid-1",
            name="Living Room",
            host="192.168.1.100",
            port=8009,
            model_name="Chromecast",
            manufacturer="Google"
        )

        device2 = CastDevice(
            uuid="uuid-2",
            name="Adahn",  # Primary device name
            host="192.168.1.101",
            port=8009,
            model_name="Google Nest Mini",
            manufacturer="Google"
        )

        device3 = CastDevice(
            uuid="uuid-3",
            name="Kitchen",
            host="192.168.1.102",
            port=8009,
            model_name="Google Home Mini",  # Fallback device
            manufacturer="Google"
        )

        self.discovery.devices = {
            "uuid-1": device1,
            "uuid-2": device2,
            "uuid-3": device3
        }

        # Should select primary device "Adahn"
        best_device = self.discovery.find_best_device()
        self.assertEqual(best_device.name, "Adahn")

        # Remove primary device
        del self.discovery.devices["uuid-2"]

        # Should select fallback device
        best_device = self.discovery.find_best_device()
        self.assertEqual(best_device.name, "Kitchen")


class TestConnectionPool(unittest.TestCase):
    """Test connection pool functionality"""

    def setUp(self):
        """Set up test connection pool"""
        self.config = ChromecastConfig()
        self.pool = DeviceConnectionPool(self.config)

        # Create test device
        self.device = CastDevice(
            uuid="test-uuid",
            name="Test Device",
            host="192.168.1.100",
            port=8009,
            model_name="Test Model",
            manufacturer="Test"
        )

    @patch('socket.socket')
    def test_device_reachability_check(self, mock_socket_class):
        """Test device reachability checking"""
        mock_socket = Mock()
        mock_socket_class.return_value = mock_socket

        # Simulate successful connection
        mock_socket.connect_ex.return_value = 0

        result = self.pool.check_availability(self.device)
        self.assertTrue(result['available'])

        # Simulate failed connection
        mock_socket.connect_ex.return_value = 1

        result = self.pool.check_availability(self.device)
        self.assertFalse(result['available'])

    @patch('pychromecast.get_chromecast_from_host')
    @patch('socket.socket')
    def test_connection_caching(self, mock_socket_class, mock_get_chromecast):
        """Test that connections are cached and reused"""
        mock_socket = Mock()
        mock_socket_class.return_value = mock_socket
        mock_socket.connect_ex.return_value = 0

        mock_cast = Mock()
        mock_cast.wait = Mock()
        mock_cast.media_controller = Mock()
        mock_cast.media_controller.update_status = Mock()
        mock_get_chromecast.return_value = mock_cast

        # First connection should create new
        conn1 = self.pool.get_connection(self.device)
        self.assertEqual(len(self.pool.connections), 1)

        # Second connection should reuse
        conn2 = self.pool.get_connection(self.device)
        self.assertEqual(len(self.pool.connections), 1)
        self.assertEqual(conn1, conn2)

    def test_connection_stats_tracking(self):
        """Test connection statistics tracking"""
        # Record some connection results
        self.pool._update_stats("Test Device", True, 1.5)
        self.pool._update_stats("Test Device", True, 2.0)
        self.pool._update_stats("Test Device", False, 0.5)

        stats = self.pool.get_stats("Test Device")
        self.assertEqual(stats['total_attempts'], 3)
        self.assertEqual(stats['successful_connections'], 2)
        self.assertEqual(stats['failed_connections'], 1)
        self.assertAlmostEqual(stats['success_rate'], 0.667, places=2)


class TestMediaController(unittest.TestCase):
    """Test media playback functionality"""

    def setUp(self):
        """Set up test media controller"""
        self.config = ChromecastConfig()
        self.mock_pool = Mock(spec=DeviceConnectionPool)
        self.controller = MediaController(self.mock_pool, self.config)

        self.device = CastDevice(
            uuid="test-uuid",
            name="Test Device",
            host="192.168.1.100",
            port=8009,
            model_name="Test Model",
            manufacturer="Test"
        )

    def test_validate_media_url(self):
        """Test media URL validation"""
        # Valid URLs
        self.assertTrue(self.controller._validate_media_url("http://example.com/media.mp3"))
        self.assertTrue(self.controller._validate_media_url("https://example.com/stream"))

        # Invalid URLs
        self.assertFalse(self.controller._validate_media_url(""))
        self.assertFalse(self.controller._validate_media_url("not-a-url"))
        self.assertFalse(self.controller._validate_media_url("file:///local/file.mp3"))

    def test_play_media_success(self):
        """Test successful media playback"""
        # Setup mock cast object
        mock_cast = Mock()
        mock_media = Mock()
        mock_cast.media_controller = mock_media
        mock_media.status = Mock()
        mock_media.status.player_state = "PLAYING"
        mock_media.status.content_id = "http://example.com/media.mp3"
        self.mock_pool.get_connection.return_value = mock_cast

        result = self.controller.play_media(
            self.device,
            "http://example.com/media.mp3"
        )

        self.assertTrue(result['success'])
        self.assertEqual(result['url'], "http://example.com/media.mp3")
        mock_media.play_media.assert_called_once()

    def test_play_media_with_retry(self):
        """Test media playback with retry logic"""
        mock_cast = Mock()
        mock_media = Mock()
        mock_cast.media_controller = mock_media
        mock_media.status = Mock()

        # First attempt fails, second succeeds
        mock_media.status.player_state = "IDLE"
        self.mock_pool.get_connection.side_effect = [
            Exception("Connection failed"),
            mock_cast
        ]

        # After retry, media loads successfully
        def update_status_side_effect():
            mock_media.status.player_state = "PLAYING"

        mock_media.update_status = Mock(side_effect=update_status_side_effect)

        result = self.controller.play_media(
            self.device,
            "http://example.com/media.mp3",
            retry_on_failure=True
        )

        # Should succeed after retry
        self.assertEqual(result['attempts'], 2)


class TestAthanController(unittest.TestCase):
    """Test Athan playback functionality"""

    def setUp(self):
        """Set up test Athan controller"""
        self.config = ChromecastConfig()
        self.mock_media_controller = Mock(spec=MediaController)
        self.controller = AthanController(self.mock_media_controller, self.config)

        self.device = CastDevice(
            uuid="test-uuid",
            name="Test Device",
            host="192.168.1.100",
            port=8009,
            model_name="Test Model",
            manufacturer="Test"
        )

    def test_athan_collision_prevention(self):
        """Test that Athan doesn't play if already playing"""
        # Set playback state to playing
        self.controller.playback_state.is_playing = True
        self.controller.playback_state.start_time = datetime.now()
        self.controller.playback_state.prayer_type = PrayerType.REGULAR

        # Attempt to play should raise error
        with self.assertRaises(AthanAlreadyPlayingError):
            self.controller.play_athan(self.device, PrayerType.FAJR)

    def test_athan_timeout_handling(self):
        """Test Athan timeout after configured period"""
        # Set playback state with old timestamp
        self.controller.playback_state.is_playing = True
        self.controller.playback_state.start_time = datetime.now() - timedelta(seconds=500)

        # Mock successful playback
        self.mock_media_controller.play_media.return_value = {
            'success': True,
            'url': 'http://localhost/athan.mp3'
        }

        # Should allow playing after timeout
        result = self.controller.play_athan(self.device, PrayerType.REGULAR)
        self.assertTrue(result['success'])

    def test_athan_status_tracking(self):
        """Test Athan status reporting"""
        # Initially not playing
        status = self.controller.get_status()
        self.assertFalse(status['playing'])

        # Start playing
        self.controller.playback_state.is_playing = True
        self.controller.playback_state.start_time = datetime.now()
        self.controller.playback_state.device_name = "Test Device"

        status = self.controller.get_status()
        self.assertTrue(status['playing'])
        self.assertEqual(status['device_name'], "Test Device")
        self.assertIsNotNone(status['elapsed_time'])


class TestChromecastManager(unittest.TestCase):
    """Test main ChromecastManager integration"""

    @patch('chromecast.manager.ChromecastDiscovery')
    @patch('chromecast.manager.DeviceConnectionPool')
    def setUp(self, mock_pool_class, mock_discovery_class):
        """Set up test manager"""
        self.config = ChromecastConfig()

        # Setup mock discovery
        self.mock_discovery = Mock()
        mock_discovery_class.return_value = self.mock_discovery
        self.mock_discovery.discover_devices.return_value = {
            'success': True,
            'devices_found': 1
        }
        self.mock_discovery.find_best_device.return_value = CastDevice(
            uuid="test-uuid",
            name="Test Device",
            host="192.168.1.100",
            port=8009,
            model_name="Test Model",
            manufacturer="Test"
        )

        # Setup mock pool
        self.mock_pool = Mock()
        mock_pool_class.return_value = self.mock_pool

        self.manager = ChromecastManager(self.config)

    def test_manager_initialization(self):
        """Test manager initializes all components"""
        self.assertIsNotNone(self.manager.discovery)
        self.assertIsNotNone(self.manager.connection_pool)
        self.assertIsNotNone(self.manager.media_controller)
        self.assertIsNotNone(self.manager.athan_controller)
        self.assertIsNotNone(self.manager.target_device)

    def test_system_status_collection(self):
        """Test comprehensive system status collection"""
        # Setup mock returns
        self.mock_discovery.get_devices.return_value = []
        self.mock_discovery.get_stats.return_value = {
            'total_discoveries': 5,
            'successful_discoveries': 4
        }
        self.mock_pool.get_pool_status.return_value = {
            'active_connections': 1
        }
        self.mock_pool.check_availability.return_value = {
            'available': True,
            'device_name': 'Test Device'
        }

        status = self.manager.get_system_status()
        self.assertTrue(status['success'])
        self.assertIn('devices', status['system_status'])
        self.assertIn('connection_pool', status['system_status'])
        self.assertIn('config', status['system_status'])

    def test_cleanup_process(self):
        """Test cleanup releases all resources"""
        result = self.manager.cleanup()
        self.assertTrue(result['success'])
        self.assertIsNone(self.manager.target_device)

        # Verify cleanup was called on components
        self.mock_pool.cleanup.assert_called_once()
        self.mock_discovery.cleanup.assert_called_once()


if __name__ == '__main__':
    unittest.main()