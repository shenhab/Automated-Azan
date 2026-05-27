"""
Test cases for main module
"""
import pytest
from unittest.mock import patch, Mock, MagicMock
from main import get_application_status, main


class TestApplicationStatus:
    """Test the get_application_status function."""

    @pytest.mark.unit
    @patch('athan_scheduler.AthanScheduler.get_prayer_times')
    @patch('athan_scheduler.AthanScheduler.get_next_prayer_time')
    @patch('athan_scheduler.AthanScheduler.get_scheduler_status')
    def test_get_application_status_healthy(self, mock_scheduler_status, mock_next_prayer, mock_prayer_times, json_response_validator):
        """Test application status when everything is healthy."""
        mock_prayer_times.return_value = {
            'success': True,
            'prayer_times': {'Fajr': '05:30', 'Dhuhr': '12:30'},
            'timestamp': '2023-01-01T00:00:00',
        }
        mock_next_prayer.return_value = {
            'success': True,
            'prayer': 'Dhuhr',
            'formatted_time': '12:30',
            'timestamp': '2023-01-01T00:00:00',
        }
        mock_scheduler_status.return_value = {
            'success': True,
            'total_jobs': 5,
            'timestamp': '2023-01-01T00:00:00',
        }

        result = get_application_status()

        json_response_validator(result, success_expected=True)
        assert result['status'] == 'healthy'
        assert 'configuration' in result
        assert 'prayer_times' in result
        assert 'scheduler' in result

    @pytest.mark.unit
    def test_get_application_status_error_handling(self, json_response_validator):
        """Test application status error handling when scheduler raises."""
        with patch('athan_scheduler.AthanScheduler.__init__', side_effect=Exception("Init error")):
            result = get_application_status()

        json_response_validator(result, success_expected=False)
        assert 'error' in result

    @pytest.mark.unit
    def test_get_application_status_returns_dict(self):
        """get_application_status always returns a dict with required keys."""
        result = get_application_status()
        assert isinstance(result, dict)
        assert 'success' in result
        assert 'application' in result
        assert result['application'] == 'Automated Azan'
        assert 'timestamp' in result


class TestMainFunction:
    """Test the main function."""

    @pytest.mark.unit
    @patch('main.start_web_interface')
    @patch('main.ConfigWatcher')
    @patch('main.AthanScheduler')
    @patch('main.setup_logging')
    def test_main_exits_on_logging_failure(self, mock_logging, mock_scheduler, mock_watcher, mock_web):
        """main() calls sys.exit(1) when logging setup fails."""
        mock_logging.return_value = {'success': False, 'error': 'disk full'}

        with patch('sys.exit') as mock_exit:
            main()
            mock_exit.assert_called_once_with(1)

    @pytest.mark.unit
    @patch('main.start_web_interface')
    @patch('main.ConfigWatcher')
    @patch('main.AthanScheduler')
    @patch('main.setup_logging')
    def test_main_starts_web_thread(self, mock_logging, mock_scheduler_cls, mock_watcher_cls, mock_web):
        """main() starts a daemon thread for the web interface."""
        mock_logging.return_value = {'success': True, 'log_file': 'test.log', 'timestamp': '2023-01-01T00:00:00'}

        scheduler = Mock()
        scheduler.get_prayer_times.return_value = {'success': True, 'prayer_times': {}}
        scheduler.get_next_prayer_time.return_value = {'success': True, 'prayer': None}
        scheduler.run_scheduler.side_effect = KeyboardInterrupt()
        mock_scheduler_cls.return_value = scheduler

        watcher = Mock()
        watcher.start.return_value = {'success': True}
        mock_watcher_cls.return_value = watcher

        with patch('threading.Thread') as mock_thread_cls:
            mock_thread = Mock()
            mock_thread_cls.return_value = mock_thread
            try:
                main()
            except KeyboardInterrupt:
                pass

        mock_thread.start.assert_called_once()
        call_kwargs = mock_thread_cls.call_args[1]
        assert call_kwargs.get('daemon') is True
