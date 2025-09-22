"""
Test cases for chromecast_manager module
"""
import pytest
from unittest.mock import patch, Mock, MagicMock
from chromecast_manager import ChromecastManager


class TestChromecastManager:
    """Test the ChromecastManager class."""

    @pytest.mark.unit
    @patch('chromecast_manager.ChromecastManager.discover_devices')
    def test_init_default(self, mock_discover, json_response_validator):
        """Test initialization with default parameters."""
        mock_discover.return_value = {
            "success": True,
            "devices_found": 0,
            "devices": {},
            "timestamp": "2023-01-01T00:00:00"
        }
        manager = ChromecastManager()
        assert manager.target_device is None
        assert manager.discovery_cooldown == 30
        assert manager.athan_playing is False

    @pytest.mark.unit
    @patch('chromecast_manager.ChromecastManager.discover_devices')
    def test_get_discovered_devices(self, mock_discover, json_response_validator):
        """Test getting discovered devices."""
        mock_discover.return_value = {
            "success": True,
            "devices_found": 0,
            "devices": {},
            "timestamp": "2023-01-01T00:00:00"
        }
        manager = ChromecastManager()
        result = manager.get_discovered_devices()

        json_response_validator(result, success_expected=True)
        assert 'devices_count' in result
        assert 'devices' in result

    @pytest.mark.unit
    @patch('chromecast_manager.ChromecastManager.discover_devices')
    def test_get_athan_status_not_playing(self, mock_discover, json_response_validator):
        """Test getting Athan status when not playing."""
        mock_discover.return_value = {
            "success": True,
            "devices_found": 0,
            "devices": {},
            "timestamp": "2023-01-01T00:00:00"
        }
        manager = ChromecastManager()
        result = manager.get_athan_status()

        json_response_validator(result, success_expected=True)
        assert 'playing' in result
        assert result['playing'] is False

    @pytest.mark.unit
    @patch('chromecast_manager.ChromecastManager.discover_devices')
    def test_get_device_status(self, mock_discover, json_response_validator):
        """Test getting device status."""
        mock_discover.return_value = {
            "success": True,
            "devices_found": 0,
            "devices": {},
            "timestamp": "2023-01-01T00:00:00"
        }
        manager = ChromecastManager()
        result = manager.get_device_status()

        json_response_validator(result, success_expected=True)

    @pytest.mark.unit
    @patch('chromecast_manager.ChromecastManager.discover_devices')
    def test_get_system_status(self, mock_discover, json_response_validator):
        """Test getting system status."""
        mock_discover.return_value = {
            "success": True,
            "devices_found": 0,
            "devices": {},
            "timestamp": "2023-01-01T00:00:00"
        }
        manager = ChromecastManager()
        result = manager.get_system_status()

        json_response_validator(result, success_expected=True)
        assert 'system_status' in result

    @pytest.mark.unit
    @patch('chromecast_manager.ChromecastManager.discover_devices')
    def test_discover_devices_mock(self, mock_discover, json_response_validator):
        """Test device discovery with mocked response."""
        mock_discover.return_value = {
            "success": True,
            "devices_found": 1,
            "devices": {
                "test-uuid": {
                    "name": "Test Device",
                    "host": "192.168.1.100",
                    "model_name": "Google Home"
                }
            },
            "timestamp": "2023-01-01T00:00:00"
        }
        manager = ChromecastManager()
        result = manager.discover_devices(force_rediscovery=True)

        json_response_validator(result, success_expected=True)
        assert 'devices_found' in result

    @pytest.mark.unit
    @patch('chromecast_manager.ChromecastManager.discover_devices')
    def test_error_handling_consistency(self, mock_discover, json_response_validator):
        """Test that all responses follow JSON pattern."""
        mock_discover.return_value = {
            "success": True,
            "devices_found": 0,
            "devices": {},
            "timestamp": "2023-01-01T00:00:00"
        }
        manager = ChromecastManager()

        # Test various methods that should return proper JSON
        methods_to_test = [
            manager.get_discovered_devices,
            manager.get_athan_status,
            manager.get_device_status,
            manager.get_system_status,
        ]

        for method in methods_to_test:
            result = method()
            # All should return valid JSON response
            json_response_validator(result)  # Validates both success/failure
            assert 'success' in result
            assert 'timestamp' in result