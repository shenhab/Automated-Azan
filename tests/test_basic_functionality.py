"""
Basic functionality tests for the Automated Azan system
"""
import pytest
import os
from unittest.mock import patch, Mock

# Import all modules for basic testing
from config_manager import ConfigManager
from logging_setup import setup_logging, get_logging_status
from prayer_times_fetcher import PrayerTimesFetcher
from chromecast_manager import ChromecastManager
from time_sync import TimeSynchronizer, update_ntp_time
from web_interface_api import WebInterfaceAPI
from athan_scheduler import AthanScheduler
from main import get_application_status


class TestBasicModuleFunctionality:
    """Test basic functionality of all modules."""

    @pytest.mark.unit
    def test_config_manager_basic(self, json_response_validator):
        """Test basic ConfigManager functionality."""
        config = ConfigManager()

        # Test get_all_settings
        result = config.get_all_settings()
        json_response_validator(result, success_expected=True)

        # Test validate_config
        result = config.validate_config()
        json_response_validator(result, success_expected=True)

    @pytest.mark.unit
    def test_logging_setup_basic(self, json_response_validator, temp_log_file):
        """Test basic logging setup functionality."""
        # Test setup_logging
        result = setup_logging(temp_log_file)
        json_response_validator(result, success_expected=True)

        # Test get_logging_status
        result = get_logging_status()
        json_response_validator(result, success_expected=True)

    @pytest.mark.unit
    def test_prayer_times_fetcher_basic(self, json_response_validator):
        """Test basic PrayerTimesFetcher functionality."""
        fetcher = PrayerTimesFetcher()

        # Test get_available_sources
        result = fetcher.get_available_sources()
        json_response_validator(result, success_expected=True)
        assert 'sources' in result

        # Test get_file_status
        result = fetcher.get_file_status()
        json_response_validator(result, success_expected=True)

    @pytest.mark.unit
    def test_chromecast_manager_basic(self, json_response_validator):
        """Test basic ChromecastManager functionality."""
        manager = ChromecastManager()

        # Test get_system_status
        result = manager.get_system_status()
        json_response_validator(result, success_expected=True)

        # Test get_athan_status
        result = manager.get_athan_status()
        json_response_validator(result, success_expected=True)

    @pytest.mark.unit
    @patch('subprocess.run')
    def test_time_sync_basic(self, mock_run, json_response_validator):
        """Test basic time synchronization functionality."""
        # Mock subprocess calls
        mock_result = Mock()
        mock_result.returncode = 0
        mock_result.stdout = "Time sync successful"
        mock_run.return_value = mock_result

        # Test update_ntp_time
        result = update_ntp_time()
        json_response_validator(result, success_expected=True)

        # Test TimeSynchronizer
        sync = TimeSynchronizer()
        result = sync.sync_status_summary()
        json_response_validator(result, success_expected=True)

    @pytest.mark.unit
    def test_web_interface_api_basic(self, json_response_validator):
        """Test basic WebInterfaceAPI functionality."""
        api = WebInterfaceAPI()

        # Test get_system_status
        result = api.get_system_status()
        json_response_validator(result, success_expected=True)

        # Test get_media_files (this method actually exists)
        result = api.get_media_files()
        json_response_validator(result, success_expected=True)

    @pytest.mark.unit
    def test_athan_scheduler_basic(self, json_response_validator):
        """Test basic AthanScheduler functionality."""
        scheduler = AthanScheduler()

        # Test get_scheduler_status
        result = scheduler.get_scheduler_status()
        json_response_validator(result, success_expected=True)

        # Test get_prayer_times (this method actually exists)
        result = scheduler.get_prayer_times()
        json_response_validator(result, success_expected=True)

    @pytest.mark.unit
    def test_main_application_status(self, json_response_validator):
        """Test main application status function."""
        result = get_application_status()
        json_response_validator(result, success_expected=True)
        assert 'status' in result

    @pytest.mark.integration
    def test_json_api_consistency(self, json_response_validator):
        """Test that all modules return consistent JSON format."""
        # Test basic methods from each module
        test_cases = [
            (ConfigManager(), 'get_all_settings'),
            (PrayerTimesFetcher(), 'get_available_sources'),
            (ChromecastManager(), 'get_system_status'),
            (WebInterfaceAPI(), 'get_system_status'),
            (AthanScheduler(), 'get_scheduler_status'),
        ]

        for module, method_name in test_cases:
            method = getattr(module, method_name)
            result = method()

            # All should follow JSON API pattern
            json_response_validator(result, success_expected=True)

            # Check for either timestamp or current_time field
            has_time_field = 'timestamp' in result or 'current_time' in result
            assert has_time_field, f"Response from {method_name} missing time field"

    @pytest.mark.integration
    def test_service_modules_integration_demo_import(self):
        """Test that the service modules integration demo can be imported."""
        try:
            import service_modules_integration
            assert hasattr(service_modules_integration, 'main')
        except ImportError as e:
            pytest.fail(f"Could not import service_modules_integration: {e}")

    @pytest.mark.unit
    def test_all_modules_importable(self):
        """Test that all main modules can be imported without errors."""
        modules = [
            'config_manager',
            'logging_setup',
            'prayer_times_fetcher',
            'chromecast_manager',
            'time_sync',
            'web_interface_api',
            'athan_scheduler',
            'main'
        ]

        for module_name in modules:
            try:
                __import__(module_name)
            except ImportError as e:
                pytest.fail(f"Could not import {module_name}: {e}")

    @pytest.mark.integration
    def test_basic_workflow(self, json_response_validator):
        """Test basic workflow between modules."""
        # 1. Configuration
        config = ConfigManager()
        config_result = config.get_all_settings()
        json_response_validator(config_result, success_expected=True)

        # 2. Prayer times
        fetcher = PrayerTimesFetcher()
        sources_result = fetcher.get_available_sources()
        json_response_validator(sources_result, success_expected=True)

        # 3. Scheduler
        scheduler = AthanScheduler()
        scheduler_result = scheduler.get_scheduler_status()
        json_response_validator(scheduler_result, success_expected=True)

        # 4. Application status
        app_result = get_application_status()
        json_response_validator(app_result, success_expected=True)

    @pytest.mark.unit
    def test_error_handling_robustness(self, json_response_validator):
        """Test that modules handle errors gracefully and return JSON."""
        # Test some operations that might fail but should return proper JSON
        test_cases = [
            (ConfigManager('nonexistent.config'), 'get_all_settings'),
            (PrayerTimesFetcher(), 'get_file_status'),  # File might not exist
            (ChromecastManager(), 'get_system_status'),  # This method exists
            (WebInterfaceAPI(), 'get_system_status'),  # This method exists
        ]

        for module, method_name in test_cases:
            try:
                method = getattr(module, method_name)
                result = method()

                # Should always return valid JSON response
                json_response_validator(result)  # Validates both success/failure

                # Should have required fields
                assert 'success' in result
                assert 'timestamp' in result

            except Exception as e:
                pytest.fail(f"Method {method_name} raised exception instead of returning JSON: {e}")

    @pytest.mark.unit
    def test_module_instantiation(self):
        """Test that all modules can be instantiated without errors."""
        try:
            config = ConfigManager()
            fetcher = PrayerTimesFetcher()
            manager = ChromecastManager()
            synchronizer = TimeSynchronizer()
            api = WebInterfaceAPI()
            scheduler = AthanScheduler()

            # Basic verification they were created
            assert config is not None
            assert fetcher is not None
            assert manager is not None
            assert synchronizer is not None
            assert api is not None
            assert scheduler is not None

        except Exception as e:
            pytest.fail(f"Failed to instantiate modules: {e}")