"""
Integration tests for the Automated Azan system
"""
import pytest
import os
import tempfile
import json
from unittest.mock import patch, Mock, MagicMock
from datetime import datetime, timedelta

# Import all modules for integration testing
from config_manager import ConfigManager
from logging_setup import setup_logging, get_logging_status
from prayer_times_fetcher import PrayerTimesFetcher
from chromecast_manager import ChromecastManager
from time_sync import TimeSynchronizer, update_ntp_time
from web_interface_api import WebInterfaceAPI
from athan_scheduler import AthanScheduler
from main import get_application_status


class TestSystemIntegration:
    """Test integration between different system components."""

    @pytest.mark.integration
    def test_config_to_scheduler_integration(self, temp_config_file, json_response_validator):
        """Test integration from configuration to scheduler."""
        # 1. Setup configuration
        config = ConfigManager(temp_config_file)
        config_result = config.validate_config()
        json_response_validator(config_result, success_expected=True)

        # 2. Get location and device from config
        location_result = config.get_location()
        speakers_result = config.get_speakers_group_name()
        json_response_validator(location_result, success_expected=True)
        json_response_validator(speakers_result, success_expected=True)

        # 3. Initialize scheduler with config values
        scheduler = AthanScheduler(
            location=location_result['location'],
            google_device=speakers_result['speakers_group_name']
        )

        # Verify scheduler was initialized with config values
        assert scheduler.location == location_result['location']
        assert scheduler.google_device == speakers_result['speakers_group_name']

    @pytest.mark.integration
    @patch('requests.get')
    def test_prayer_times_to_scheduler_integration(self, mock_get, json_response_validator, mock_requests, sample_prayer_times):
        """Test integration from prayer times fetcher to scheduler."""
        # 1. Fetch prayer times
        fetcher = PrayerTimesFetcher(save_to_file=False)
        fetch_result = fetcher.fetch_prayer_times("naas")
        json_response_validator(fetch_result, success_expected=True)

        # 2. Initialize scheduler and load the fetched times
        scheduler = AthanScheduler()
        with patch.object(scheduler, '_fetch_prayer_times') as mock_fetch:
            mock_fetch.return_value = fetch_result

            load_result = scheduler.load_prayer_times()
            json_response_validator(load_result, success_expected=True)

        # 3. Verify scheduler can get next prayer
        next_prayer_result = scheduler.get_next_prayer_time()
        json_response_validator(next_prayer_result, success_expected=True)

    @pytest.mark.integration
    def test_scheduler_to_chromecast_integration(self, json_response_validator, mock_pychromecast, sample_prayer_times):
        """Test integration from scheduler to chromecast manager."""
        with patch('pychromecast.get_chromecasts', mock_pychromecast):
            # 1. Setup scheduler with prayer times
            scheduler = AthanScheduler(google_device="Test Speaker")
            scheduler.prayer_times = sample_prayer_times
            scheduler.prayer_times_loaded = True

            # 2. Test Athan playback through chromecast manager
            with patch('chromecast_manager.ChromecastManager.play_url_on_cast') as mock_play:
                mock_play.return_value = {
                    'success': True,
                    'device_name': 'Test Speaker',
                    'url': 'http://example.com/athan.mp3',
                    'timestamp': '2023-01-01T00:00:00'
                }

                # 3. Play Athan
                media_result = scheduler.get_media_url("Fajr")
                json_response_validator(media_result, success_expected=True)

                play_result = scheduler.play_athan("Fajr", media_result['media_url'])
                json_response_validator(play_result, success_expected=True)

    @pytest.mark.integration
    def test_logging_across_modules(self, temp_log_file, json_response_validator):
        """Test logging integration across all modules."""
        # 1. Setup logging
        logging_result = setup_logging(temp_log_file)
        json_response_validator(logging_result, success_expected=True)

        # 2. Test logging in different modules
        config = ConfigManager()
        config.get_all_settings()  # This should log

        fetcher = PrayerTimesFetcher()
        fetcher.get_available_sources()  # This should log

        manager = ChromecastManager()
        manager.get_system_status()  # This should log

        # 3. Verify log file has content from multiple modules
        with open(temp_log_file, 'r') as f:
            log_content = f.read()
            assert len(log_content) > 0

        # 4. Get logging status
        status_result = get_logging_status()
        json_response_validator(status_result, success_expected=True)

    @pytest.mark.integration
    def test_web_api_integration(self, temp_config_file, mock_media_files, json_response_validator):
        """Test web interface API integration with other modules."""
        # 1. Setup web API with config and media
        web_api = WebInterfaceAPI(
            media_directory=mock_media_files,
            config_file=temp_config_file
        )

        # 2. Test system status (integrates with multiple modules)
        system_result = web_api.get_system_status()
        json_response_validator(system_result, success_expected=True)

        # 3. Test configuration access
        config_result = web_api.get_configuration()
        json_response_validator(config_result, success_expected=True)

        # 4. Test media files
        media_result = web_api.get_media_files()
        json_response_validator(media_result, success_expected=True)

        # 5. Test service health (integrates health checks)
        health_result = web_api.get_service_health()
        json_response_validator(health_result, success_expected=True)

    @pytest.mark.integration
    @patch('subprocess.run')
    def test_time_sync_integration(self, mock_run, json_response_validator):
        """Test time synchronization integration."""
        # Mock successful subprocess calls
        mock_result = Mock()
        mock_result.returncode = 0
        mock_result.stdout = "Time sync successful"
        mock_run.return_value = mock_result

        # 1. Test individual time sync
        sync_result = update_ntp_time()
        json_response_validator(sync_result, success_expected=True)

        # 2. Test detailed time synchronizer
        synchronizer = TimeSynchronizer()
        status_result = synchronizer.sync_status_summary()
        json_response_validator(status_result, success_expected=True)

        # 3. Verify integration with application status
        app_status = get_application_status()
        json_response_validator(app_status, success_expected=True)

    @pytest.mark.integration
    def test_full_application_workflow(self, temp_config_file, json_response_validator, mock_pychromecast, mock_requests, sample_prayer_times):
        """Test complete application workflow integration."""
        with patch('pychromecast.get_chromecasts', mock_pychromecast):
            with patch('requests.get', mock_requests):
                # 1. Configuration validation
                config = ConfigManager(temp_config_file)
                config_validation = config.validate_config()
                json_response_validator(config_validation, success_expected=True)

                # 2. Get configuration values
                location_result = config.get_location()
                speakers_result = config.get_speakers_group_name()
                json_response_validator(location_result, success_expected=True)
                json_response_validator(speakers_result, success_expected=True)

                # 3. Initialize scheduler with config
                scheduler = AthanScheduler(
                    location=location_result['location'],
                    google_device=speakers_result['speakers_group_name']
                )

                # 4. Load prayer times
                with patch.object(scheduler, '_fetch_prayer_times') as mock_fetch:
                    mock_fetch.return_value = {
                        'success': True,
                        'prayer_times': sample_prayer_times,
                        'source': 'naas',
                        'timestamp': '2023-01-01T00:00:00'
                    }
                    load_result = scheduler.load_prayer_times()
                    json_response_validator(load_result, success_expected=True)

                # 5. Schedule prayers
                with patch('schedule.every') as mock_every:
                    mock_job = Mock()
                    mock_every.return_value = mock_job
                    mock_job.tag.return_value = mock_job
                    mock_job.do.return_value = mock_job

                    schedule_result = scheduler.schedule_prayers()
                    json_response_validator(schedule_result, success_expected=True)

                # 6. Get application status (integrates everything)
                app_status = get_application_status()
                json_response_validator(app_status, success_expected=True)
                assert app_status['status'] == 'healthy'

    @pytest.mark.integration
    def test_error_propagation_integration(self, json_response_validator):
        """Test how errors propagate through the system integration."""
        # 1. Test with invalid configuration
        config = ConfigManager('nonexistent.config')
        config_result = config.validate_config()
        json_response_validator(config_result, success_expected=False)

        # 2. Test how this affects application status
        app_status = get_application_status()
        json_response_validator(app_status, success_expected=True)  # Should succeed but show issues
        assert app_status['status'] != 'healthy'

        # 3. Test scheduler with invalid configuration
        scheduler = AthanScheduler(location="invalid-location")
        load_result = scheduler.load_prayer_times()
        json_response_validator(load_result, success_expected=False)

    @pytest.mark.integration
    def test_concurrent_operations(self, temp_config_file, json_response_validator, mock_pychromecast):
        """Test concurrent operations integration."""
        with patch('pychromecast.get_chromecasts', mock_pychromecast):
            # Simulate concurrent access to different modules
            config = ConfigManager(temp_config_file)
            scheduler = AthanScheduler()
            manager = ChromecastManager()
            web_api = WebInterfaceAPI()

            # Test concurrent operations
            results = []

            # Configuration operations
            results.append(config.get_all_settings())
            results.append(config.validate_config())

            # Scheduler operations
            results.append(scheduler.get_scheduler_status())

            # Chromecast operations
            results.append(manager.get_system_status())
            results.append(manager.discover_devices())

            # Web API operations
            results.append(web_api.get_system_status())
            results.append(web_api.get_api_endpoints())

            # Verify all operations completed successfully
            for result in results:
                json_response_validator(result, success_expected=True)

    @pytest.mark.integration
    def test_data_flow_consistency(self, temp_config_file, json_response_validator, sample_prayer_times):
        """Test data consistency across module boundaries."""
        # 1. Setup configuration
        config = ConfigManager(temp_config_file)
        location_result = config.get_location()
        speakers_result = config.get_speakers_group_name()

        # 2. Initialize scheduler with same config
        scheduler = AthanScheduler(
            location=location_result['location'],
            google_device=speakers_result['speakers_group_name']
        )

        # 3. Setup prayer times
        scheduler.prayer_times = sample_prayer_times
        scheduler.prayer_times_loaded = True

        # 4. Test data consistency
        prayer_times_result = scheduler.get_prayer_times()
        json_response_validator(prayer_times_result, success_expected=True)

        # Verify the data is consistent
        assert prayer_times_result['prayer_times'] == sample_prayer_times
        assert scheduler.location == location_result['location']
        assert scheduler.google_device == speakers_result['speakers_group_name']

    @pytest.mark.integration
    def test_service_modules_integration_demo(self, json_response_validator):
        """Test the service modules integration demo functionality."""
        # This tests the actual service_modules_integration.py functionality
        # by importing and running key demonstration functions

        # Import the integration demo functions
        from service_modules_integration import (
            demonstrate_core_modules,
            demonstrate_service_modules,
            demonstrate_api_endpoints
        )

        # Test core modules demo (with mocked dependencies)
        with patch('config_manager.ConfigManager.get_all_settings') as mock_settings:
            with patch('config_manager.ConfigManager.validate_config') as mock_validate:
                mock_settings.return_value = {'success': True, 'settings': {}, 'timestamp': '2023-01-01T00:00:00'}
                mock_validate.return_value = {'success': True, 'validation_details': {}, 'timestamp': '2023-01-01T00:00:00'}

                # Should not raise exceptions
                try:
                    demonstrate_core_modules()
                except Exception as e:
                    pytest.fail(f"Core modules demo failed: {e}")

        # Test API endpoints demo
        with patch('config_manager.ConfigManager.get_all_settings') as mock_settings:
            with patch('prayer_times_fetcher.PrayerTimesFetcher.get_available_sources') as mock_sources:
                mock_settings.return_value = {'success': True, 'settings': {}, 'timestamp': '2023-01-01T00:00:00'}
                mock_sources.return_value = {'success': True, 'sources': {}, 'timestamp': '2023-01-01T00:00:00'}

                # Should not raise exceptions
                try:
                    demonstrate_api_endpoints()
                except Exception as e:
                    pytest.fail(f"API endpoints demo failed: {e}")

    @pytest.mark.integration
    @pytest.mark.slow
    def test_performance_integration(self, temp_config_file, json_response_validator):
        """Test performance characteristics of integrated operations."""
        import time

        start_time = time.time()

        # Perform multiple operations in sequence
        config = ConfigManager(temp_config_file)
        scheduler = AthanScheduler()
        web_api = WebInterfaceAPI()

        operations = [
            config.get_all_settings,
            config.validate_config,
            scheduler.get_scheduler_status,
            web_api.get_system_status,
            web_api.get_api_endpoints
        ]

        results = []
        for operation in operations:
            op_start = time.time()
            result = operation()
            op_end = time.time()

            json_response_validator(result, success_expected=True)
            results.append({
                'operation': operation.__name__,
                'duration': op_end - op_start,
                'success': result['success']
            })

        total_time = time.time() - start_time

        # Performance assertions
        assert total_time < 5.0, f"Integration operations took too long: {total_time}s"
        assert all(r['success'] for r in results), "Some operations failed"

        # Individual operation performance
        for result in results:
            assert result['duration'] < 2.0, f"{result['operation']} took too long: {result['duration']}s"

    @pytest.mark.integration
    def test_resource_cleanup_integration(self, temp_config_file, temp_log_file, json_response_validator):
        """Test resource cleanup across integrated components."""
        # 1. Setup resources
        logging_result = setup_logging(temp_log_file)
        json_response_validator(logging_result, success_expected=True)

        config = ConfigManager(temp_config_file)
        scheduler = AthanScheduler()

        # 2. Use resources
        config.get_all_settings()
        scheduler.get_scheduler_status()

        # 3. Cleanup logging
        from logging_setup import cleanup_logging
        cleanup_result = cleanup_logging()
        json_response_validator(cleanup_result, success_expected=True)

        # 4. Verify cleanup
        status_result = get_logging_status()
        json_response_validator(status_result, success_expected=True)

    @pytest.mark.integration
    def test_json_api_consistency_integration(self, json_response_validator):
        """Test JSON API consistency across all integrated modules."""
        # Test that all modules return consistent JSON format
        modules_and_methods = [
            (ConfigManager(), 'get_all_settings'),
            (ConfigManager(), 'validate_config'),
            (PrayerTimesFetcher(), 'get_available_sources'),
            (ChromecastManager(), 'get_system_status'),
            (TimeSynchronizer(), 'sync_status_summary'),
            (WebInterfaceAPI(), 'get_system_status'),
            (AthanScheduler(), 'get_scheduler_status'),
        ]

        for module, method_name in modules_and_methods:
            method = getattr(module, method_name)
            result = method()

            # All should follow JSON API pattern
            json_response_validator(result, success_expected=True)

            # Additional consistency checks
            assert 'timestamp' in result
            assert isinstance(result['timestamp'], str)
            assert len(result['timestamp']) > 0