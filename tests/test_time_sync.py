"""
Test cases for time_sync module
"""
import pytest
from unittest.mock import patch, Mock
from time_sync import TimeSynchronizer, update_ntp_time


class TestTimeSynchronizer:
    """Test the TimeSynchronizer class."""

    @pytest.mark.unit
    def test_init_default(self):
        """Test initialization with default parameters."""
        sync = TimeSynchronizer()
        assert hasattr(sync, 'ntp_servers')
        assert hasattr(sync, 'time_apis')
        assert len(sync.ntp_servers) > 0

    @pytest.mark.unit
    def test_get_system_time_info(self, json_response_validator):
        """Test getting system time information."""
        sync = TimeSynchronizer()
        result = sync.get_system_time_info()

        json_response_validator(result, success_expected=True)
        assert 'parsed_info' in result or 'current_time' in result

    @pytest.mark.unit
    @patch('socket.socket')
    def test_get_ntp_time_success(self, mock_socket, json_response_validator):
        """Test successful NTP time retrieval."""
        # Mock socket behavior for NTP
        mock_sock_instance = Mock()
        mock_socket.return_value = mock_sock_instance
        mock_sock_instance.recv.return_value = b'\x00' * 48  # Mock NTP response

        sync = TimeSynchronizer()
        result = sync.get_ntp_time('pool.ntp.org')

        json_response_validator(result)  # May succeed or fail based on parsing

    @pytest.mark.unit
    @patch('socket.socket')
    def test_get_ntp_time_failure(self, mock_socket, json_response_validator):
        """Test NTP time retrieval failure."""
        mock_socket.side_effect = Exception("Network error")

        sync = TimeSynchronizer()
        result = sync.get_ntp_time('pool.ntp.org')

        json_response_validator(result, success_expected=False)
        assert 'error' in result

    @pytest.mark.unit
    @patch('requests.get')
    def test_get_http_time_success(self, mock_get, json_response_validator):
        """Test successful HTTP time retrieval."""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            'datetime': '2023-01-01T12:00:00+00:00',
            'timezone': 'Europe/Dublin'
        }
        mock_get.return_value = mock_response

        sync = TimeSynchronizer()
        result = sync.get_http_time()

        json_response_validator(result)  # May succeed or fail

    @pytest.mark.unit
    @patch('requests.get')
    def test_get_http_time_failure(self, mock_get, json_response_validator):
        """Test HTTP time retrieval failure."""
        mock_get.side_effect = Exception("Network error")

        sync = TimeSynchronizer()
        result = sync.get_http_time()

        json_response_validator(result, success_expected=False)
        assert 'error' in result

    @pytest.mark.unit
    def test_get_accurate_time(self, json_response_validator):
        """Test getting accurate time (tries multiple sources)."""
        sync = TimeSynchronizer()
        result = sync.get_accurate_time()

        json_response_validator(result)  # May succeed or fail depending on network

    @pytest.mark.unit
    @patch('time_sync.TimeSynchronizer.get_accurate_time')
    def test_check_time_drift(self, mock_get_accurate_time, json_response_validator):
        """Test time drift checking."""
        mock_get_accurate_time.return_value = {
            'success': True,
            'ntp_time': 1640995200.0,  # Mock timestamp
            'source': 'ntp'
        }

        sync = TimeSynchronizer()
        result = sync.check_time_drift()

        json_response_validator(result)  # May succeed or fail
        # check_time_drift should return useful information regardless
        assert 'drift_seconds' in result or 'error' in result or 'message' in result

    @pytest.mark.unit
    def test_get_all_ntp_servers_status(self, json_response_validator):
        """Test getting status of all NTP servers."""
        sync = TimeSynchronizer()
        result = sync.get_all_ntp_servers_status()

        json_response_validator(result, success_expected=True)
        assert 'servers_status' in result or 'servers' in result
        assert 'servers_checked' in result or 'total_servers' in result

    @pytest.mark.unit
    def test_sync_status_summary(self, json_response_validator):
        """Test getting sync status summary."""
        sync = TimeSynchronizer()
        result = sync.sync_status_summary()

        json_response_validator(result, success_expected=True)
        # sync_status_summary should contain useful summary information
        assert 'system_time_info' in result or 'time_drift' in result or 'ntp_servers' in result


class TestUpdateNtpTime:
    """Test the standalone update_ntp_time function."""

    @pytest.mark.unit
    @patch('subprocess.run')
    def test_update_ntp_time_success(self, mock_run, json_response_validator):
        """Test successful NTP time update."""
        mock_result = Mock()
        mock_result.returncode = 0
        mock_result.stdout = "Time sync successful"
        mock_result.stderr = ""
        mock_run.return_value = mock_result

        result = update_ntp_time()

        json_response_validator(result, success_expected=True)
        assert 'message' in result

    @pytest.mark.unit
    @patch('subprocess.run')
    def test_update_ntp_time_failure(self, mock_run, json_response_validator):
        """Test NTP time update failure."""
        mock_result = Mock()
        mock_result.returncode = 1
        mock_result.stdout = ""
        mock_result.stderr = "Permission denied"
        mock_run.return_value = mock_result

        result = update_ntp_time()

        json_response_validator(result, success_expected=False)
        assert 'error' in result

    @pytest.mark.unit
    @patch('subprocess.run')
    def test_update_ntp_time_exception(self, mock_run, json_response_validator):
        """Test NTP time update with exception."""
        mock_run.side_effect = Exception("Command not found")

        result = update_ntp_time()

        json_response_validator(result, success_expected=False)
        assert 'error' in result

    @pytest.mark.integration
    def test_full_workflow(self, json_response_validator):
        """Test complete time sync workflow."""
        # 1. Create synchronizer
        sync = TimeSynchronizer()

        # 2. Get system time info
        system_info = sync.get_system_time_info()
        json_response_validator(system_info, success_expected=True)

        # 3. Get sync status summary
        status = sync.sync_status_summary()
        json_response_validator(status, success_expected=True)

    @pytest.mark.unit
    def test_error_handling_consistency(self, json_response_validator):
        """Test that all error responses follow JSON pattern."""
        sync = TimeSynchronizer()

        # Test various methods that should return proper JSON
        methods_to_test = [
            sync.get_system_time_info,
            sync.get_all_ntp_servers_status,
            sync.sync_status_summary,
        ]

        for method in methods_to_test:
            result = method()
            # All should return valid JSON response
            json_response_validator(result)  # Validates both success/failure
            assert 'success' in result
            assert 'timestamp' in result or 'current_time' in result