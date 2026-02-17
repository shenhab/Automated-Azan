"""
Test cases for config_manager module
"""
import pytest
import os
import tempfile
from unittest.mock import patch, mock_open
from config_manager import ConfigManager


class TestConfigManager:
    """Test the ConfigManager class."""

    def test_init_default_config(self):
        """Test initialization with default config file."""
        config = ConfigManager()
        assert config.config_file == 'adahn.config'

    def test_init_custom_config(self, temp_config_file):
        """Test initialization with custom config file."""
        config = ConfigManager(temp_config_file)
        assert config.config_file == temp_config_file

    @pytest.mark.unit
    def test_get_all_settings_success(self, temp_config_file, json_response_validator):
        """Test successful retrieval of all settings."""
        config = ConfigManager(temp_config_file)
        result = config.get_all_settings()

        json_response_validator(result, success_expected=True)
        assert 'settings' in result

    @pytest.mark.unit
    def test_get_all_settings_file_not_found(self, json_response_validator):
        """Test get_all_settings with non-existent config file - returns success with empty settings."""
        config = ConfigManager('nonexistent.config')
        result = config.get_all_settings()

        # ConfigManager returns success=True even for missing files (graceful handling)
        json_response_validator(result, success_expected=True)
        assert result['settings'] == {}
        assert result['sections_count'] == 0

    @pytest.mark.unit
    def test_get_location_success(self, temp_config_file, json_response_validator):
        """Test successful location retrieval."""
        config = ConfigManager(temp_config_file)
        result = config.get_location()

        json_response_validator(result, success_expected=True)
        assert result['location'] == 'new-york-usa'

    @pytest.mark.unit
    def test_get_location_missing(self, json_response_validator):
        """Test location retrieval with missing configuration."""
        config = ConfigManager('nonexistent.config')
        result = config.get_location()

        json_response_validator(result, success_expected=False)
        assert 'location' in result['error'].lower()

    @pytest.mark.unit
    def test_get_speakers_group_name_success(self, temp_config_file, json_response_validator):
        """Test successful speakers group name retrieval."""
        config = ConfigManager(temp_config_file)
        result = config.get_speakers_group_name()

        json_response_validator(result, success_expected=True)
        assert result['speakers_group_name'] == 'Test Speakers'

    @pytest.mark.unit
    def test_get_speakers_group_name_missing(self, json_response_validator):
        """Test speakers group name retrieval with missing configuration."""
        config = ConfigManager('nonexistent.config')
        result = config.get_speakers_group_name()

        json_response_validator(result, success_expected=False)
        assert 'speakers-group-name' in result['error'].lower()

    @pytest.mark.unit
    def test_validate_config_success(self, temp_config_file, json_response_validator):
        """Test successful config validation."""
        config = ConfigManager(temp_config_file)
        result = config.validate_config()

        json_response_validator(result, success_expected=True)
        assert 'validated_settings' in result  # Actual field name

    @pytest.mark.unit
    def test_validate_config_missing_file(self):
        """Test config validation with missing file - should fail."""
        config = ConfigManager('nonexistent.config')
        result = config.validate_config()

        # validate_config returns success=False but with validated_settings instead of error field
        assert result['success'] is False
        assert 'validated_settings' in result
        assert len(result['validated_settings']) > 0
        # Check that validation failed for required settings
        for setting in result['validated_settings']:
            assert setting['valid'] is False

    @pytest.mark.unit
    def test_update_setting_success(self, temp_config_file, json_response_validator):
        """Test successful setting update."""
        config = ConfigManager(temp_config_file)

        # Update a setting in the existing Settings section
        result = config.update_setting('Settings', 'location', 'london-uk')

        json_response_validator(result, success_expected=True)

        # Verify the setting was actually updated
        updated_result = config.get_location()
        assert updated_result['location'] == 'london-uk'

    @pytest.mark.unit
    def test_update_setting_invalid_section(self, temp_config_file, json_response_validator):
        """Test setting update with invalid section - actually creates the section."""
        config = ConfigManager(temp_config_file)
        result = config.update_setting('InvalidSection', 'key', 'value')

        # update_setting actually creates new sections, so it succeeds
        json_response_validator(result, success_expected=True)
        assert result['success'] is True

    @pytest.mark.unit
    def test_get_config_info(self, temp_config_file, json_response_validator):
        """Test configuration info retrieval."""
        config = ConfigManager(temp_config_file)
        result = config.get_config_info()

        json_response_validator(result, success_expected=True)
        assert 'config_file' in result
        assert 'file_exists' in result
        assert 'sections' in result

    @pytest.mark.unit
    def test_get_setting(self, temp_config_file, json_response_validator):
        """Test getting a specific setting."""
        config = ConfigManager(temp_config_file)
        result = config.get_setting('Settings', 'location')

        json_response_validator(result, success_expected=True)
        assert 'value' in result
        assert result['value'] == 'new-york-usa'

    @pytest.mark.unit
    def test_get_setting_missing(self, temp_config_file, json_response_validator):
        """Test getting a non-existent setting."""
        config = ConfigManager(temp_config_file)
        result = config.get_setting('Settings', 'nonexistent-key')

        json_response_validator(result, success_expected=False)
        assert 'error' in result

    @pytest.mark.unit
    def test_reload_config(self, temp_config_file, json_response_validator):
        """Test config reload functionality."""
        config = ConfigManager(temp_config_file)
        result = config.reload_config()

        json_response_validator(result, success_expected=True)
        assert 'message' in result

    @pytest.mark.unit
    def test_get_prayer_source(self, json_response_validator):
        """Test getting prayer source setting."""
        # Create a config with prayer source
        with tempfile.NamedTemporaryFile(mode='w', suffix='.config', delete=False) as f:
            config_content = """[Settings]
location = new-york-usa
speakers-group-name = Test Speakers
prayer-source = naas
"""
            f.write(config_content)
            temp_path = f.name

        try:
            config = ConfigManager(temp_path)
            result = config.get_prayer_source()
            json_response_validator(result, success_expected=True)
            assert 'prayer_source' in result
        finally:
            os.unlink(temp_path)

    @pytest.mark.unit
    def test_is_pre_fajr_enabled(self, json_response_validator):
        """Test pre-fajr setting check."""
        # Create a config with pre-fajr setting
        with tempfile.NamedTemporaryFile(mode='w', suffix='.config', delete=False) as f:
            config_content = """[Settings]
location = new-york-usa
speakers-group-name = Test Speakers
pre-fajr-enabled = True
"""
            f.write(config_content)
            temp_path = f.name

        try:
            config = ConfigManager(temp_path)
            result = config.is_pre_fajr_enabled()
            json_response_validator(result, success_expected=True)
            assert 'pre_fajr_enabled' in result
        finally:
            os.unlink(temp_path)

    @pytest.mark.unit
    def test_get_log_file(self, json_response_validator):
        """Test getting log file configuration."""
        config = ConfigManager()
        result = config.get_log_file()
        json_response_validator(result, success_expected=True)
        assert 'log_file' in result

    @pytest.mark.unit
    def test_save_config(self, temp_config_file, json_response_validator):
        """Test saving configuration."""
        config = ConfigManager(temp_config_file)

        # Modify and save
        config.update_setting('Settings', 'location', 'test-location')
        result = config.save_config()

        json_response_validator(result, success_expected=True)
        assert 'message' in result

        # Verify saved
        config2 = ConfigManager(temp_config_file)
        location = config2.get_location()
        assert location['location'] == 'test-location'

    @pytest.mark.unit
    def test_error_handling_consistency(self):
        """Test that error responses follow JSON pattern."""
        config = ConfigManager('nonexistent.config')

        # Test various error scenarios
        methods_to_test = [
            lambda: config.get_location(),
            lambda: config.get_speakers_group_name(),
            lambda: config.get_setting('Settings', 'nonexistent'),
        ]

        for method in methods_to_test:
            result = method()
            # All should return proper JSON
            assert isinstance(result, dict)
            assert 'success' in result
            assert 'timestamp' in result

            # These methods should fail
            assert result['success'] is False
            assert 'error' in result
            assert isinstance(result['error'], str)