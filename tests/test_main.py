"""
Test cases for main module
"""
import pytest
import os
import tempfile
from unittest.mock import patch, Mock, MagicMock
from main import get_application_status, main


class TestApplicationStatus:
    """Test the get_application_status function."""

    @pytest.mark.unit
    @patch('config_manager.ConfigManager.validate_config')
    @patch('config_manager.ConfigManager.get_location')
    @patch('config_manager.ConfigManager.get_speakers_group_name')
    def test_get_application_status_healthy(self, mock_speakers, mock_location, mock_validate, json_response_validator):
        """Test application status when everything is healthy."""
        mock_validate.return_value = {
            'success': True,
            'validation_details': {'valid': True},
            'timestamp': '2023-01-01T00:00:00'
        }
        mock_location.return_value = {
            'success': True,
            'location': 'new-york-usa',
            'timestamp': '2023-01-01T00:00:00'
        }
        mock_speakers.return_value = {
            'success': True,
            'speakers_group_name': 'Test Speakers',
            'timestamp': '2023-01-01T00:00:00'
        }

        result = get_application_status()

        json_response_validator(result, success_expected=True)
        assert 'status' in result
        assert result['status'] == 'healthy'
        assert 'configuration' in result
        assert 'prayer_times' in result

    @pytest.mark.unit
    @patch('config_manager.ConfigManager.validate_config')
    def test_get_application_status_config_invalid(self, mock_validate, json_response_validator):
        """Test application status when configuration is invalid."""
        mock_validate.return_value = {
            'success': False,
            'error': 'Configuration validation failed',
            'timestamp': '2023-01-01T00:00:00'
        }

        result = get_application_status()

        json_response_validator(result, success_expected=True)
        assert 'status' in result
        assert result['status'] != 'healthy'
        assert 'configuration' in result

    @pytest.mark.unit
    @patch('config_manager.ConfigManager.validate_config')
    @patch('config_manager.ConfigManager.get_location')
    @patch('config_manager.ConfigManager.get_speakers_group_name')
    @patch('athan_scheduler.AthanScheduler.get_prayer_times')
    def test_get_application_status_prayer_times_loaded(self, mock_prayer_times, mock_speakers, mock_location, mock_validate, json_response_validator, sample_prayer_times):
        """Test application status with prayer times loaded."""
        mock_validate.return_value = {'success': True, 'validation_details': {'valid': True}, 'timestamp': '2023-01-01T00:00:00'}
        mock_location.return_value = {'success': True, 'location': 'new-york-usa', 'timestamp': '2023-01-01T00:00:00'}
        mock_speakers.return_value = {'success': True, 'speakers_group_name': 'Test Speakers', 'timestamp': '2023-01-01T00:00:00'}
        mock_prayer_times.return_value = {
            'success': True,
            'prayer_times': sample_prayer_times,
            'source': 'naas',
            'timestamp': '2023-01-01T00:00:00'
        }

        result = get_application_status()

        json_response_validator(result, success_expected=True)
        assert 'prayer_times' in result
        assert result['prayer_times']['success'] is True

    @pytest.mark.unit
    @patch('config_manager.ConfigManager.validate_config')
    @patch('config_manager.ConfigManager.get_location')
    @patch('config_manager.ConfigManager.get_speakers_group_name')
    @patch('athan_scheduler.AthanScheduler.get_next_prayer_time')
    def test_get_application_status_next_prayer(self, mock_next_prayer, mock_speakers, mock_location, mock_validate, json_response_validator):
        """Test application status with next prayer information."""
        mock_validate.return_value = {'success': True, 'validation_details': {'valid': True}, 'timestamp': '2023-01-01T00:00:00'}
        mock_location.return_value = {'success': True, 'location': 'new-york-usa', 'timestamp': '2023-01-01T00:00:00'}
        mock_speakers.return_value = {'success': True, 'speakers_group_name': 'Test Speakers', 'timestamp': '2023-01-01T00:00:00'}
        mock_next_prayer.return_value = {
            'success': True,
            'prayer': 'Dhuhr',
            'time': '12:30',
            'formatted_time': '12:30 PM',
            'timestamp': '2023-01-01T00:00:00'
        }

        result = get_application_status()

        json_response_validator(result, success_expected=True)
        assert 'next_prayer' in result
        assert result['next_prayer']['success'] is True

    @pytest.mark.unit
    @patch('config_manager.ConfigManager.validate_config')
    @patch('config_manager.ConfigManager.get_location')
    @patch('config_manager.ConfigManager.get_speakers_group_name')
    @patch('athan_scheduler.AthanScheduler.get_scheduler_status')
    def test_get_application_status_scheduler_info(self, mock_scheduler_status, mock_speakers, mock_location, mock_validate, json_response_validator):
        """Test application status with scheduler information."""
        mock_validate.return_value = {'success': True, 'validation_details': {'valid': True}, 'timestamp': '2023-01-01T00:00:00'}
        mock_location.return_value = {'success': True, 'location': 'new-york-usa', 'timestamp': '2023-01-01T00:00:00'}
        mock_speakers.return_value = {'success': True, 'speakers_group_name': 'Test Speakers', 'timestamp': '2023-01-01T00:00:00'}
        mock_scheduler_status.return_value = {
            'success': True,
            'total_jobs': 5,
            'athan_jobs': 5,
            'timestamp': '2023-01-01T00:00:00'
        }

        result = get_application_status()

        json_response_validator(result, success_expected=True)
        assert 'scheduler' in result
        assert result['scheduler']['success'] is True

    @pytest.mark.unit
    def test_get_application_status_error_handling(self, json_response_validator):
        """Test application status error handling."""
        with patch('config_manager.ConfigManager.validate_config') as mock_validate:
            mock_validate.side_effect = Exception("Configuration manager error")

            result = get_application_status()

            json_response_validator(result, success_expected=False)
            assert 'error' in result

    @pytest.mark.unit
    def test_get_application_status_partial_failures(self, json_response_validator):
        """Test application status with partial failures."""
        with patch('config_manager.ConfigManager.validate_config') as mock_validate:
            with patch('config_manager.ConfigManager.get_location') as mock_location:
                with patch('config_manager.ConfigManager.get_speakers_group_name') as mock_speakers:
                    mock_validate.return_value = {'success': True, 'validation_details': {'valid': True}, 'timestamp': '2023-01-01T00:00:00'}
                    mock_location.return_value = {'success': False, 'error': 'Location not found', 'timestamp': '2023-01-01T00:00:00'}
                    mock_speakers.return_value = {'success': True, 'speakers_group_name': 'Test Speakers', 'timestamp': '2023-01-01T00:00:00'}

                    result = get_application_status()

                    json_response_validator(result, success_expected=True)
                    # Should still return success but with partial information
                    assert 'configuration' in result


class TestMainFunction:
    """Test the main function."""

    @pytest.mark.unit
    @patch('threading.Thread')
    @patch('schedule.run_pending')
    @patch('time.sleep')
    @patch('athan_scheduler.AthanScheduler.load_prayer_times')
    @patch('athan_scheduler.AthanScheduler.schedule_prayers')
    @patch('logging_setup.setup_logging')
    def test_main_initialization_success(self, mock_logging, mock_schedule, mock_load, mock_sleep, mock_run_pending, mock_thread, json_response_validator):
        """Test successful main function initialization."""
        # Mock successful initialization
        mock_logging.return_value = {'success': True, 'log_file': 'test.log', 'timestamp': '2023-01-01T00:00:00'}
        mock_load.return_value = {'success': True, 'prayer_times': {}, 'timestamp': '2023-01-01T00:00:00'}
        mock_schedule.return_value = {'success': True, 'scheduled_prayers': 5, 'timestamp': '2023-01-01T00:00:00'}

        # Mock thread
        mock_web_thread = Mock()
        mock_thread.return_value = mock_web_thread

        # Mock sleep to break the loop after first iteration
        mock_sleep.side_effect = [None, KeyboardInterrupt()]

        # Test main function - should handle KeyboardInterrupt gracefully
        try:
            main()
        except KeyboardInterrupt:
            pass

        # Verify logging was set up
        mock_logging.assert_called_once()

        # Verify prayer times were loaded
        mock_load.assert_called_once()

        # Verify prayers were scheduled
        mock_schedule.assert_called_once()

        # Verify web thread was started
        mock_thread.assert_called_once()
        mock_web_thread.start.assert_called_once()

    @pytest.mark.unit
    @patch('logging_setup.setup_logging')
    def test_main_logging_failure(self, mock_logging):
        """Test main function with logging setup failure."""
        mock_logging.return_value = {
            'success': False,
            'error': 'Failed to setup logging',
            'timestamp': '2023-01-01T00:00:00'
        }

        # Should handle logging failure gracefully
        try:
            with patch('sys.exit') as mock_exit:
                main()
                # Should exit on logging failure
                mock_exit.assert_called_once_with(1)
        except SystemExit:
            pass

    @pytest.mark.unit
    @patch('threading.Thread')
    @patch('athan_scheduler.AthanScheduler.load_prayer_times')
    @patch('logging_setup.setup_logging')
    def test_main_prayer_times_failure(self, mock_logging, mock_load, mock_thread):
        """Test main function with prayer times loading failure."""
        mock_logging.return_value = {'success': True, 'log_file': 'test.log', 'timestamp': '2023-01-01T00:00:00'}
        mock_load.return_value = {
            'success': False,
            'error': 'Failed to load prayer times',
            'timestamp': '2023-01-01T00:00:00'
        }

        # Should handle prayer times failure gracefully
        try:
            with patch('sys.exit') as mock_exit:
                main()
                # Should exit on prayer times failure
                mock_exit.assert_called_once_with(1)
        except SystemExit:
            pass

    @pytest.mark.unit
    @patch('threading.Thread')
    @patch('athan_scheduler.AthanScheduler.load_prayer_times')
    @patch('athan_scheduler.AthanScheduler.schedule_prayers')
    @patch('logging_setup.setup_logging')
    def test_main_scheduling_failure(self, mock_logging, mock_schedule, mock_load, mock_thread):
        """Test main function with prayer scheduling failure."""
        mock_logging.return_value = {'success': True, 'log_file': 'test.log', 'timestamp': '2023-01-01T00:00:00'}
        mock_load.return_value = {'success': True, 'prayer_times': {}, 'timestamp': '2023-01-01T00:00:00'}
        mock_schedule.return_value = {
            'success': False,
            'error': 'Failed to schedule prayers',
            'timestamp': '2023-01-01T00:00:00'
        }

        # Should handle scheduling failure gracefully
        try:
            with patch('sys.exit') as mock_exit:
                main()
                # Should exit on scheduling failure
                mock_exit.assert_called_once_with(1)
        except SystemExit:
            pass

    @pytest.mark.unit
    @patch('threading.Thread')
    @patch('schedule.run_pending')
    @patch('time.sleep')
    @patch('athan_scheduler.AthanScheduler.load_prayer_times')
    @patch('athan_scheduler.AthanScheduler.schedule_prayers')
    @patch('logging_setup.setup_logging')
    def test_main_web_interface_startup(self, mock_logging, mock_schedule, mock_load, mock_sleep, mock_run_pending, mock_thread):
        """Test main function web interface startup."""
        mock_logging.return_value = {'success': True, 'log_file': 'test.log', 'timestamp': '2023-01-01T00:00:00'}
        mock_load.return_value = {'success': True, 'prayer_times': {}, 'timestamp': '2023-01-01T00:00:00'}
        mock_schedule.return_value = {'success': True, 'scheduled_prayers': 5, 'timestamp': '2023-01-01T00:00:00'}

        # Mock thread for web interface
        mock_web_thread = Mock()
        mock_thread.return_value = mock_web_thread

        # Mock sleep to break loop
        mock_sleep.side_effect = KeyboardInterrupt()

        try:
            main()
        except KeyboardInterrupt:
            pass

        # Verify web thread was created and started
        mock_thread.assert_called_once()
        call_args = mock_thread.call_args
        assert 'target' in call_args[1]
        assert 'daemon' in call_args[1]
        assert call_args[1]['daemon'] is True

        mock_web_thread.start.assert_called_once()

    @pytest.mark.unit
    @patch('threading.Thread')
    @patch('schedule.run_pending')
    @patch('time.sleep')
    @patch('athan_scheduler.AthanScheduler.load_prayer_times')
    @patch('athan_scheduler.AthanScheduler.schedule_prayers')
    @patch('logging_setup.setup_logging')
    def test_main_schedule_running(self, mock_logging, mock_schedule, mock_load, mock_sleep, mock_run_pending, mock_thread):
        """Test main function schedule running loop."""
        mock_logging.return_value = {'success': True, 'log_file': 'test.log', 'timestamp': '2023-01-01T00:00:00'}
        mock_load.return_value = {'success': True, 'prayer_times': {}, 'timestamp': '2023-01-01T00:00:00'}
        mock_schedule.return_value = {'success': True, 'scheduled_prayers': 5, 'timestamp': '2023-01-01T00:00:00'}

        # Mock thread
        mock_web_thread = Mock()
        mock_thread.return_value = mock_web_thread

        # Mock sleep to run a few iterations then break
        sleep_count = 0
        def sleep_side_effect(duration):
            nonlocal sleep_count
            sleep_count += 1
            if sleep_count >= 3:
                raise KeyboardInterrupt()

        mock_sleep.side_effect = sleep_side_effect

        try:
            main()
        except KeyboardInterrupt:
            pass

        # Verify schedule.run_pending was called multiple times
        assert mock_run_pending.call_count >= 2

        # Verify sleep was called with correct interval
        assert all(call[0][0] == 1 for call in mock_sleep.call_args_list)

    @pytest.mark.unit
    @patch('threading.Thread')
    @patch('schedule.run_pending')
    @patch('time.sleep')
    @patch('athan_scheduler.AthanScheduler.load_prayer_times')
    @patch('athan_scheduler.AthanScheduler.schedule_prayers')
    @patch('logging_setup.setup_logging')
    def test_main_exception_handling(self, mock_logging, mock_schedule, mock_load, mock_sleep, mock_run_pending, mock_thread):
        """Test main function exception handling."""
        mock_logging.return_value = {'success': True, 'log_file': 'test.log', 'timestamp': '2023-01-01T00:00:00'}
        mock_load.return_value = {'success': True, 'prayer_times': {}, 'timestamp': '2023-01-01T00:00:00'}
        mock_schedule.return_value = {'success': True, 'scheduled_prayers': 5, 'timestamp': '2023-01-01T00:00:00'}

        # Mock thread
        mock_web_thread = Mock()
        mock_thread.return_value = mock_web_thread

        # Mock run_pending to raise an exception
        mock_run_pending.side_effect = Exception("Scheduler error")

        # Mock sleep to prevent infinite loop
        mock_sleep.side_effect = [None, KeyboardInterrupt()]

        try:
            main()
        except KeyboardInterrupt:
            pass

        # Should handle exceptions in the main loop gracefully
        # (The exact behavior depends on implementation)

    @pytest.mark.integration
    @patch('threading.Thread')
    @patch('time.sleep')
    def test_main_integration_flow(self, mock_sleep, mock_thread):
        """Test main function integration flow."""
        # Mock sleep to break after one iteration
        mock_sleep.side_effect = KeyboardInterrupt()

        # Mock thread
        mock_web_thread = Mock()
        mock_thread.return_value = mock_web_thread

        # Mock all the dependencies to return success
        with patch('logging_setup.setup_logging') as mock_logging:
            with patch('config_manager.ConfigManager.validate_config') as mock_config:
                with patch('athan_scheduler.AthanScheduler.load_prayer_times') as mock_load:
                    with patch('athan_scheduler.AthanScheduler.schedule_prayers') as mock_schedule:

                        mock_logging.return_value = {'success': True, 'log_file': 'test.log', 'timestamp': '2023-01-01T00:00:00'}
                        mock_config.return_value = {'success': True, 'validation_details': {'valid': True}, 'timestamp': '2023-01-01T00:00:00'}
                        mock_load.return_value = {'success': True, 'prayer_times': {}, 'timestamp': '2023-01-01T00:00:00'}
                        mock_schedule.return_value = {'success': True, 'scheduled_prayers': 5, 'timestamp': '2023-01-01T00:00:00'}

                        try:
                            main()
                        except KeyboardInterrupt:
                            pass

                        # Verify the integration flow
                        mock_logging.assert_called_once()
                        mock_load.assert_called_once()
                        mock_schedule.assert_called_once()
                        mock_thread.assert_called_once()

    @pytest.mark.unit
    def test_error_handling_consistency(self, json_response_validator):
        """Test that application status errors follow JSON pattern."""
        # Test error scenario in get_application_status
        with patch('config_manager.ConfigManager.validate_config') as mock_validate:
            mock_validate.side_effect = Exception("Test error")

            result = get_application_status()

            json_response_validator(result)
            if not result['success']:
                assert 'error' in result
                assert isinstance(result['error'], str)
                assert len(result['error']) > 0