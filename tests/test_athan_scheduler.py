"""
Test cases for athan_scheduler module
"""
import pytest
from datetime import datetime, timedelta
from unittest.mock import patch, Mock, MagicMock
from athan_scheduler import AthanScheduler


class TestAthanScheduler:
    """Test the AthanScheduler class."""

    @pytest.mark.unit
    def test_init_default(self):
        """Test initialization with default parameters."""
        scheduler = AthanScheduler()
        assert scheduler.location == "icci"
        assert scheduler.google_device == "athan"

    @pytest.mark.unit
    def test_init_custom(self):
        """Test initialization with custom parameters."""
        scheduler = AthanScheduler(
            location="naas",
            google_device="My Speaker"
        )
        assert scheduler.location == "naas"
        assert scheduler.google_device == "My Speaker"

    @pytest.mark.unit
    @patch('prayer_times_fetcher.PrayerTimesFetcher.fetch_prayer_times')
    def test_load_prayer_times_success(self, mock_fetch, json_response_validator, sample_prayer_times):
        """Test successful prayer times loading."""
        mock_fetch.return_value = {
            'success': True,
            'prayer_times': sample_prayer_times,
            'source': 'naas',
            'timestamp': '2023-01-01T00:00:00'
        }

        scheduler = AthanScheduler()
        result = scheduler.load_prayer_times()

        json_response_validator(result, success_expected=True)
        assert 'prayer_times' in result
        assert 'fetch_result' in result

    @pytest.mark.unit
    @patch('prayer_times_fetcher.PrayerTimesFetcher.fetch_prayer_times')
    def test_load_prayer_times_failure(self, mock_fetch, json_response_validator):
        """Test prayer times loading failure."""
        mock_fetch.return_value = {
            'success': False,
            'error': 'Failed to fetch prayer times',
            'timestamp': '2023-01-01T00:00:00'
        }

        scheduler = AthanScheduler()
        result = scheduler.load_prayer_times()

        json_response_validator(result, success_expected=False)

    @pytest.mark.unit
    def test_get_prayer_times_cached(self, json_response_validator, sample_prayer_times):
        """Test getting prayer times from cache."""
        scheduler = AthanScheduler()
        scheduler.prayer_times = sample_prayer_times
        scheduler.prayer_times_loaded = True

        result = scheduler.get_prayer_times()

        json_response_validator(result, success_expected=True)
        assert 'prayer_times' in result
        assert result['prayer_times'] == sample_prayer_times

    @pytest.mark.unit
    @patch('prayer_times_fetcher.PrayerTimesFetcher.fetch_prayer_times')
    def test_get_prayer_times_load_on_demand(self, mock_fetch, json_response_validator, sample_prayer_times):
        """Test getting prayer times with loading on demand."""
        mock_fetch.return_value = {
            'success': True,
            'prayer_times': sample_prayer_times,
            'source': 'naas',
            'timestamp': '2023-01-01T00:00:00'
        }

        scheduler = AthanScheduler()
        scheduler.prayer_times_loaded = False

        result = scheduler.get_prayer_times()

        json_response_validator(result, success_expected=True)
        assert 'prayer_times' in result

    @pytest.mark.unit
    def test_get_next_prayer_time_success(self, json_response_validator, sample_prayer_times, mock_datetime):
        """Test successful next prayer time calculation."""
        scheduler = AthanScheduler()
        scheduler.prayer_times = sample_prayer_times
        scheduler.prayer_times_loaded = True

        result = scheduler.get_next_prayer_time()

        json_response_validator(result, success_expected=True)
        assert 'prayer' in result
        assert 'time' in result
        assert 'formatted_time' in result

    @pytest.mark.unit
    def test_get_next_prayer_time_no_times_loaded(self, json_response_validator):
        """Test next prayer time when no times are loaded."""
        scheduler = AthanScheduler()
        scheduler.prayer_times_loaded = False

        result = scheduler.get_next_prayer_time()

        json_response_validator(result, success_expected=False)
        assert 'prayer times' in result['error'].lower()

    @pytest.mark.unit
    def test_get_next_prayer_time_end_of_day(self, json_response_validator, sample_prayer_times):
        """Test next prayer time calculation at end of day."""
        # Mock current time as late evening (after Isha)
        with patch('datetime.datetime') as mock_dt:
            mock_dt.now.return_value = datetime(2023, 6, 15, 23, 30, 0)  # 11:30 PM
            mock_dt.strptime = datetime.strptime

            scheduler = AthanScheduler()
            scheduler.prayer_times = sample_prayer_times
            scheduler.prayer_times_loaded = True

            result = scheduler.get_next_prayer_time()

            json_response_validator(result, success_expected=True)
            # Should return Fajr of next day
            assert result['prayer'] == 'Fajr'

    @pytest.mark.unit
    @patch('schedule.every')
    def test_schedule_prayers_success(self, mock_every, json_response_validator, sample_prayer_times, mock_schedule):
        """Test successful prayer scheduling."""
        scheduler = AthanScheduler()
        scheduler.prayer_times = sample_prayer_times
        scheduler.prayer_times_loaded = True

        result = scheduler.schedule_prayers()

        json_response_validator(result, success_expected=True)
        assert 'scheduled_prayers' in result
        assert 'total_jobs' in result

    @pytest.mark.unit
    def test_schedule_prayers_no_times(self, json_response_validator):
        """Test prayer scheduling when no times are loaded."""
        scheduler = AthanScheduler()
        scheduler.prayer_times_loaded = False

        result = scheduler.schedule_prayers()

        json_response_validator(result, success_expected=False)

    @pytest.mark.unit
    @patch('schedule.cancel_job')
    @patch('schedule.get_jobs')
    def test_clear_scheduled_prayers(self, mock_get_jobs, mock_cancel, json_response_validator):
        """Test clearing scheduled prayers."""
        mock_job = Mock()
        mock_job.tags = {'athan'}
        mock_get_jobs.return_value = [mock_job]

        scheduler = AthanScheduler()
        result = scheduler.clear_scheduled_prayers()

        json_response_validator(result, success_expected=True)
        assert 'cleared_jobs' in result

    @pytest.mark.unit
    @patch('schedule.get_jobs')
    def test_get_scheduler_status(self, mock_get_jobs, json_response_validator):
        """Test scheduler status retrieval."""
        mock_job1 = Mock()
        mock_job1.tags = {'athan'}
        mock_job1.job_func = Mock()
        mock_job1.job_func.__name__ = 'play_athan'

        mock_job2 = Mock()
        mock_job2.tags = {'athan'}
        mock_job2.job_func = Mock()
        mock_job2.job_func.__name__ = 'play_athan'

        mock_get_jobs.return_value = [mock_job1, mock_job2]

        scheduler = AthanScheduler()
        result = scheduler.get_scheduler_status()

        json_response_validator(result, success_expected=True)
        assert 'total_jobs' in result
        assert 'athan_jobs' in result
        assert result['athan_jobs'] == 2

    @pytest.mark.unit
    @patch('chromecast_manager.ChromecastManager.play_url_on_cast')
    def test_play_athan_success(self, mock_play, json_response_validator):
        """Test successful Athan playing."""
        mock_play.return_value = {
            'success': True,
            'device_name': 'Test Speaker',
            'url': 'http://example.com/athan.mp3',
            'timestamp': '2023-01-01T00:00:00'
        }

        scheduler = AthanScheduler(google_device="Test Speaker")
        result = scheduler.play_athan("Fajr", "http://example.com/athan.mp3")

        json_response_validator(result, success_expected=True)
        assert 'prayer_name' in result
        assert 'device' in result

    @pytest.mark.unit
    @patch('chromecast_manager.ChromecastManager.play_url_on_cast')
    def test_play_athan_device_failure(self, mock_play, json_response_validator):
        """Test Athan playing with device failure."""
        mock_play.return_value = {
            'success': False,
            'error': 'Device not found',
            'timestamp': '2023-01-01T00:00:00'
        }

        scheduler = AthanScheduler()
        result = scheduler.play_athan("Fajr", "http://example.com/athan.mp3")

        json_response_validator(result, success_expected=False)

    @pytest.mark.unit
    def test_get_media_url_success(self, json_response_validator):
        """Test successful media URL generation."""
        scheduler = AthanScheduler()
        result = scheduler.get_media_url("Fajr")

        json_response_validator(result, success_expected=True)
        assert 'prayer_name' in result
        assert 'media_url' in result
        assert 'media_file' in result

    @pytest.mark.unit
    def test_get_media_url_invalid_prayer(self, json_response_validator):
        """Test media URL generation for invalid prayer."""
        scheduler = AthanScheduler()
        result = scheduler.get_media_url("InvalidPrayer")

        json_response_validator(result, success_expected=False)
        assert 'prayer' in result['error'].lower()

    @pytest.mark.unit
    def test_validate_prayer_times_valid(self, json_response_validator, sample_prayer_times):
        """Test validation of valid prayer times."""
        scheduler = AthanScheduler()
        result = scheduler.validate_prayer_times(sample_prayer_times)

        json_response_validator(result, success_expected=True)
        assert 'valid' in result
        assert result['valid'] is True

    @pytest.mark.unit
    def test_validate_prayer_times_missing_prayer(self, json_response_validator):
        """Test validation of incomplete prayer times."""
        incomplete_times = {'Fajr': '05:30', 'Dhuhr': '12:30'}

        scheduler = AthanScheduler()
        result = scheduler.validate_prayer_times(incomplete_times)

        json_response_validator(result, success_expected=False)
        assert 'missing' in result['error'].lower()

    @pytest.mark.unit
    def test_validate_prayer_times_invalid_format(self, json_response_validator):
        """Test validation of prayer times with invalid format."""
        invalid_times = {
            'Fajr': 'invalid_time',
            'Dhuhr': '12:30',
            'Asr': '15:30',
            'Maghrib': '18:30',
            'Isha': '20:30'
        }

        scheduler = AthanScheduler()
        result = scheduler.validate_prayer_times(invalid_times)

        json_response_validator(result, success_expected=False)

    @pytest.mark.unit
    def test_get_time_until_next_prayer(self, json_response_validator, sample_prayer_times):
        """Test time calculation until next prayer."""
        with patch('datetime.datetime') as mock_dt:
            mock_dt.now.return_value = datetime(2023, 6, 15, 10, 0, 0)  # 10:00 AM
            mock_dt.strptime = datetime.strptime

            scheduler = AthanScheduler()
            scheduler.prayer_times = sample_prayer_times
            scheduler.prayer_times_loaded = True

            result = scheduler.get_time_until_next_prayer()

            json_response_validator(result, success_expected=True)
            assert 'next_prayer' in result
            assert 'time_remaining' in result

    @pytest.mark.unit
    def test_get_prayer_history(self, json_response_validator):
        """Test prayer history retrieval."""
        scheduler = AthanScheduler()
        result = scheduler.get_prayer_history()

        json_response_validator(result, success_expected=True)
        assert 'history' in result
        assert isinstance(result['history'], list)

    @pytest.mark.unit
    def test_update_prayer_settings_success(self, json_response_validator):
        """Test successful prayer settings update."""
        scheduler = AthanScheduler()
        result = scheduler.update_prayer_settings(
            location="london-uk",
            device="New Speaker",
            source="islamicfinder"
        )

        json_response_validator(result, success_expected=True)
        assert 'updated_settings' in result
        assert scheduler.location == "london-uk"
        assert scheduler.google_device == "New Speaker"

    @pytest.mark.unit
    def test_test_athan_playback(self, json_response_validator):
        """Test Athan playback testing."""
        with patch.object(AthanScheduler, 'play_athan') as mock_play:
            mock_play.return_value = {
                'success': True,
                'prayer_name': 'Test',
                'device': 'Test Speaker',
                'timestamp': '2023-01-01T00:00:00'
            }

            scheduler = AthanScheduler()
            result = scheduler.test_athan_playback()

            json_response_validator(result, success_expected=True)
            assert 'test_result' in result

    @pytest.mark.unit
    def test_get_scheduler_statistics(self, json_response_validator):
        """Test scheduler statistics retrieval."""
        scheduler = AthanScheduler()
        result = scheduler.get_scheduler_statistics()

        json_response_validator(result, success_expected=True)
        assert 'statistics' in result

    @pytest.mark.integration
    @patch('prayer_times_fetcher.PrayerTimesFetcher.fetch_prayer_times')
    @patch('schedule.every')
    def test_full_workflow_integration(self, mock_every, mock_fetch, json_response_validator, sample_prayer_times, mock_schedule):
        """Test complete scheduler workflow integration."""
        mock_fetch.return_value = {
            'success': True,
            'prayer_times': sample_prayer_times,
            'source': 'naas',
            'timestamp': '2023-01-01T00:00:00'
        }

        scheduler = AthanScheduler()

        # 1. Load prayer times
        load_result = scheduler.load_prayer_times()
        json_response_validator(load_result, success_expected=True)

        # 2. Get prayer times
        times_result = scheduler.get_prayer_times()
        json_response_validator(times_result, success_expected=True)

        # 3. Get next prayer
        next_result = scheduler.get_next_prayer_time()
        json_response_validator(next_result, success_expected=True)

        # 4. Schedule prayers
        schedule_result = scheduler.schedule_prayers()
        json_response_validator(schedule_result, success_expected=True)

        # 5. Get scheduler status
        status_result = scheduler.get_scheduler_status()
        json_response_validator(status_result, success_expected=True)

    @pytest.mark.unit
    def test_error_handling_consistency(self, json_response_validator):
        """Test that all error responses follow JSON pattern."""
        scheduler = AthanScheduler()

        # Test various error scenarios
        error_scenarios = [
            lambda: scheduler.get_next_prayer_time(),  # No prayer times loaded
            lambda: scheduler.schedule_prayers(),  # No prayer times loaded
            lambda: scheduler.validate_prayer_times({}),  # Empty prayer times
            lambda: scheduler.get_media_url("InvalidPrayer"),
        ]

        for scenario in error_scenarios:
            result = scenario()
            json_response_validator(result)
            if not result['success']:
                assert 'error' in result
                assert isinstance(result['error'], str)
                assert len(result['error']) > 0

    @pytest.mark.unit
    def test_schedule_specific_prayer(self, json_response_validator, mock_schedule):
        """Test scheduling a specific prayer."""
        with patch('schedule.every') as mock_every:
            scheduler = AthanScheduler()
            result = scheduler.schedule_specific_prayer("Fajr", "05:30")

            json_response_validator(result, success_expected=True)
            assert 'prayer_name' in result
            assert 'scheduled_time' in result

    @pytest.mark.unit
    def test_reschedule_prayers(self, json_response_validator, sample_prayer_times):
        """Test rescheduling prayers."""
        scheduler = AthanScheduler()
        scheduler.prayer_times = sample_prayer_times
        scheduler.prayer_times_loaded = True

        with patch.object(scheduler, 'clear_scheduled_prayers') as mock_clear:
            mock_clear.return_value = {'success': True, 'cleared_jobs': 5, 'timestamp': '2023-01-01T00:00:00'}

            with patch.object(scheduler, 'schedule_prayers') as mock_schedule:
                mock_schedule.return_value = {'success': True, 'scheduled_prayers': 5, 'timestamp': '2023-01-01T00:00:00'}

                result = scheduler.reschedule_prayers()

                json_response_validator(result, success_expected=True)
                assert 'rescheduled' in result

    @pytest.mark.unit
    def test_prayer_time_parsing(self, json_response_validator):
        """Test prayer time parsing and validation."""
        scheduler = AthanScheduler()

        valid_times = ["05:30", "12:45", "23:59"]
        invalid_times = ["25:00", "12:60", "invalid"]

        for time_str in valid_times:
            result = scheduler.parse_prayer_time(time_str)
            json_response_validator(result, success_expected=True)

        for time_str in invalid_times:
            result = scheduler.parse_prayer_time(time_str)
            json_response_validator(result, success_expected=False)