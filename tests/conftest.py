"""
Test configuration and fixtures for pytest
"""
import pytest
import os
import tempfile
import shutil
from unittest.mock import Mock, patch, MagicMock
import sys
import json
from datetime import datetime, timedelta

# Add the parent directory to sys.path so we can import our modules
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


@pytest.fixture
def temp_config_file():
    """Create a temporary config file for testing."""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.config', delete=False) as f:
        # Using the actual format expected by ConfigManager (Settings section)
        config_content = """[Settings]
location = new-york-usa
speakers-group-name = Test Speakers
default-athan-source = naas
volume-level = 70
"""
        f.write(config_content)
        temp_path = f.name

    yield temp_path

    # Cleanup
    if os.path.exists(temp_path):
        os.unlink(temp_path)


@pytest.fixture
def invalid_config_file():
    """Create an invalid config file for testing."""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.config', delete=False) as f:
        config_content = """[invalid-section]
invalid-key = invalid-value
"""
        f.write(config_content)
        temp_path = f.name

    yield temp_path

    # Cleanup
    if os.path.exists(temp_path):
        os.unlink(temp_path)


@pytest.fixture
def temp_log_file():
    """Create a temporary log file for testing."""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.log', delete=False) as f:
        temp_path = f.name

    yield temp_path

    # Cleanup
    if os.path.exists(temp_path):
        os.unlink(temp_path)


@pytest.fixture
def mock_pychromecast():
    """Mock pychromecast module for testing."""
    with patch('pychromecast.get_chromecasts') as mock_get_chromecasts:
        mock_device = Mock()
        mock_device.name = 'Test Speaker'
        mock_device.device.friendly_name = 'Test Speaker'
        mock_device.device.model_name = 'Google Home'
        mock_device.device.manufacturer = 'Google Inc.'
        mock_device.device.uuid = 'test-uuid-123'
        mock_device.status = Mock()
        mock_device.status.display_name = 'Test Display'
        mock_device.status.status_text = 'Ready to cast'
        mock_device.media_controller = Mock()
        mock_device.media_controller.status = Mock()
        mock_device.media_controller.status.player_state = 'IDLE'
        mock_device.socket_client = Mock()
        mock_device.socket_client.is_connected = True

        mock_get_chromecasts.return_value = ([mock_device], Mock())
        yield mock_get_chromecasts


@pytest.fixture
def mock_requests():
    """Mock requests module for testing."""
    with patch('requests.get') as mock_get:
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.text = '''
        <html>
        <table>
        <tr><td>Fajr</td><td>05:30</td></tr>
        <tr><td>Dhuhr</td><td>12:30</td></tr>
        <tr><td>Asr</td><td>15:30</td></tr>
        <tr><td>Maghrib</td><td>18:30</td></tr>
        <tr><td>Isha</td><td>20:30</td></tr>
        </table>
        </html>
        '''
        mock_get.return_value = mock_response
        yield mock_get


@pytest.fixture
def mock_subprocess():
    """Mock subprocess module for testing."""
    with patch('subprocess.run') as mock_run:
        mock_result = Mock()
        mock_result.returncode = 0
        mock_result.stdout = "Time sync successful"
        mock_result.stderr = ""
        mock_run.return_value = mock_result
        yield mock_run


@pytest.fixture
def sample_prayer_times():
    """Sample prayer times data for testing."""
    return {
        'Fajr': '05:30',
        'Dhuhr': '12:30',
        'Asr': '15:30',
        'Maghrib': '18:30',
        'Isha': '20:30'
    }


@pytest.fixture
def mock_schedule():
    """Mock schedule module for testing."""
    with patch('schedule.every') as mock_every:
        mock_job = Mock()
        mock_every.return_value = mock_job
        mock_job.tag.return_value = mock_job
        mock_job.do.return_value = mock_job
        yield mock_every


@pytest.fixture
def mock_flask_app():
    """Mock Flask application for testing."""
    with patch('flask.Flask') as mock_flask:
        mock_app = Mock()
        mock_flask.return_value = mock_app
        yield mock_app


@pytest.fixture
def mock_datetime():
    """Mock datetime for testing."""
    with patch('datetime.datetime') as mock_dt:
        # Set a fixed time for consistent testing
        fixed_time = datetime(2023, 6, 15, 10, 30, 0)  # 10:30 AM
        mock_dt.now.return_value = fixed_time
        mock_dt.strptime = datetime.strptime
        yield mock_dt


@pytest.fixture
def clean_environment():
    """Clean environment variables before and after tests."""
    # Store original environment
    original_env = dict(os.environ)

    # Clean test-related env vars
    test_vars = ['AUTOMATED_AZAN_CONFIG', 'AUTOMATED_AZAN_MEDIA_DIR']
    for var in test_vars:
        if var in os.environ:
            del os.environ[var]

    yield

    # Restore original environment
    os.environ.clear()
    os.environ.update(original_env)


@pytest.fixture
def mock_media_files():
    """Create mock media files for testing."""
    temp_dir = tempfile.mkdtemp()
    media_dir = os.path.join(temp_dir, 'Media')
    os.makedirs(media_dir)

    # Create mock media files
    mock_files = [
        'media_Athan.mp3',
        'media_adhan_al_fajr.mp3',
        'media_test.mp3'
    ]

    for filename in mock_files:
        filepath = os.path.join(media_dir, filename)
        with open(filepath, 'w') as f:
            f.write('mock audio content')

    yield media_dir

    # Cleanup
    shutil.rmtree(temp_dir)


@pytest.fixture
def json_response_validator():
    """Validator for JSON responses from our modules."""
    def validate_json_response(response, success_expected=True):
        """Validate that response follows our JSON API pattern."""
        assert isinstance(response, dict), "Response must be a dictionary"
        assert 'success' in response, "Response must contain 'success' field"
        assert isinstance(response['success'], bool), "'success' must be boolean"

        # Check for timestamp or current_time (modules use different field names)
        has_time_field = 'timestamp' in response or 'current_time' in response or 'query_timestamp' in response
        assert has_time_field, "Response must contain 'timestamp', 'current_time', or 'query_timestamp' field"

        if 'timestamp' in response:
            # Timestamp can be string, float, or int depending on module
            assert isinstance(response['timestamp'], (str, float, int)), "'timestamp' must be string, float, or int"
        if 'current_time' in response:
            assert isinstance(response['current_time'], str), "'current_time' must be string"

        if success_expected:
            assert response['success'] is True, "Expected successful response"
        else:
            assert response['success'] is False, "Expected failed response"
            assert 'error' in response, "Failed response must contain 'error' field"

    return validate_json_response