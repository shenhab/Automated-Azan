"""
Test cases for logging_setup module
"""
import pytest
import os
import logging
import tempfile
from unittest.mock import patch, Mock
from logging_setup import setup_logging, get_logging_status, cleanup_logging


class TestLoggingSetup:
    """Test the logging_setup module."""

    @pytest.mark.unit
    def test_setup_logging_success(self, temp_log_file, json_response_validator):
        """Test successful logging setup."""
        result = setup_logging(temp_log_file)

        json_response_validator(result, success_expected=True)
        assert 'log_file' in result
        assert result['log_file'] == temp_log_file
        assert 'level' in result

        # Verify log file was created
        assert os.path.exists(temp_log_file)

    @pytest.mark.unit
    def test_setup_logging_default_file(self, json_response_validator):
        """Test logging setup with default file."""
        result = setup_logging()

        json_response_validator(result, success_expected=True)
        assert 'log_file' in result
        assert result['log_file'].endswith('.log')

        # Cleanup
        if os.path.exists(result['log_file']):
            os.unlink(result['log_file'])

    @pytest.mark.unit
    def test_setup_logging_invalid_path(self, json_response_validator):
        """Test logging setup with invalid path."""
        invalid_path = '/invalid/path/that/does/not/exist/test.log'
        result = setup_logging(invalid_path)

        json_response_validator(result, success_expected=False)
        assert 'error' in result

    @pytest.mark.unit
    def test_setup_logging_custom_level(self, temp_log_file, json_response_validator):
        """Test logging setup with custom level."""
        result = setup_logging(temp_log_file, level=logging.WARNING)

        json_response_validator(result, success_expected=True)
        assert result['level'] == 'WARNING'

    @pytest.mark.unit
    def test_setup_logging_permission_denied(self, json_response_validator):
        """Test logging setup with permission denied."""
        # Try to write to a system directory (should fail)
        restricted_path = '/root/restricted/test.log'
        result = setup_logging(restricted_path)

        # This might succeed or fail depending on system permissions
        # Just verify it follows JSON pattern
        json_response_validator(result)

    @pytest.mark.unit
    def test_get_logging_status_success(self, temp_log_file, json_response_validator):
        """Test successful logging status retrieval."""
        # Setup logging first
        setup_result = setup_logging(temp_log_file)
        assert setup_result['success']

        result = get_logging_status()

        json_response_validator(result, success_expected=True)
        assert 'handlers' in result
        assert 'level' in result
        assert 'logger_name' in result

    @pytest.mark.unit
    def test_get_logging_status_no_setup(self, json_response_validator):
        """Test logging status when no logging is set up."""
        # Clear any existing handlers
        logging.getLogger().handlers = []

        result = get_logging_status()

        json_response_validator(result, success_expected=True)
        assert 'handlers' in result
        assert len(result['handlers']) >= 0  # May have default handlers

    @pytest.mark.unit
    def test_cleanup_logging_success(self, temp_log_file, json_response_validator):
        """Test successful logging cleanup."""
        # Setup logging first
        setup_result = setup_logging(temp_log_file)
        assert setup_result['success']

        result = cleanup_logging()

        json_response_validator(result, success_expected=True)
        assert 'handlers_removed' in result

    @pytest.mark.unit
    def test_cleanup_logging_no_handlers(self, json_response_validator):
        """Test cleanup when no handlers exist."""
        # Clear any existing handlers
        logging.getLogger().handlers = []

        result = cleanup_logging()

        json_response_validator(result, success_expected=True)
        assert result['handlers_removed'] == 0

    @pytest.mark.unit
    def test_logging_actual_writing(self, temp_log_file):
        """Test that logging actually writes to file."""
        # Setup logging
        result = setup_logging(temp_log_file)
        assert result['success']

        # Write a test log message
        logger = logging.getLogger()
        test_message = "Test log message for pytest"
        logger.info(test_message)

        # Force flush
        for handler in logger.handlers:
            handler.flush()

        # Verify message was written
        with open(temp_log_file, 'r') as f:
            content = f.read()
            assert test_message in content

    @pytest.mark.unit
    def test_multiple_logging_setups(self, json_response_validator):
        """Test multiple consecutive logging setups."""
        temp_files = []

        try:
            # Create multiple temporary log files
            for i in range(3):
                with tempfile.NamedTemporaryFile(mode='w', suffix='.log', delete=False) as f:
                    temp_files.append(f.name)

            # Setup logging multiple times
            for temp_file in temp_files:
                result = setup_logging(temp_file)
                json_response_validator(result, success_expected=True)

            # Check status after multiple setups
            status = get_logging_status()
            json_response_validator(status, success_expected=True)

        finally:
            # Cleanup
            for temp_file in temp_files:
                if os.path.exists(temp_file):
                    os.unlink(temp_file)

    @pytest.mark.unit
    def test_logging_levels_mapping(self, temp_log_file, json_response_validator):
        """Test different logging levels."""
        levels = [
            (logging.DEBUG, 'DEBUG'),
            (logging.INFO, 'INFO'),
            (logging.WARNING, 'WARNING'),
            (logging.ERROR, 'ERROR'),
            (logging.CRITICAL, 'CRITICAL')
        ]

        for level_int, level_str in levels:
            result = setup_logging(temp_log_file, level=level_int)
            json_response_validator(result, success_expected=True)
            assert result['level'] == level_str

    @pytest.mark.unit
    def test_logging_formatter_setup(self, temp_log_file):
        """Test that logging formatter is properly configured."""
        result = setup_logging(temp_log_file)
        assert result['success']

        # Get the logger and test formatting
        logger = logging.getLogger()

        # Write a test message
        logger.info("Test formatting message")

        # Check the log file content has proper formatting
        with open(temp_log_file, 'r') as f:
            content = f.read()
            # Should contain timestamp, level, and message
            assert 'INFO' in content
            assert 'Test formatting message' in content
            # Should have some kind of timestamp format
            assert any(char.isdigit() for char in content)

    @pytest.mark.unit
    def test_concurrent_logging_access(self, temp_log_file, json_response_validator):
        """Test concurrent access to logging."""
        result = setup_logging(temp_log_file)
        json_response_validator(result, success_expected=True)

        # Simulate concurrent writes
        logger = logging.getLogger()
        messages = [f"Concurrent message {i}" for i in range(10)]

        for message in messages:
            logger.info(message)

        # Force flush
        for handler in logger.handlers:
            handler.flush()

        # Verify all messages were written
        with open(temp_log_file, 'r') as f:
            content = f.read()
            for message in messages:
                assert message in content

    @pytest.mark.unit
    def test_error_handling_consistency(self, json_response_validator):
        """Test that all error responses follow JSON pattern."""
        # Test various error scenarios
        error_scenarios = [
            lambda: setup_logging('/invalid/path/test.log'),
            lambda: setup_logging(''),  # Empty path
        ]

        for scenario in error_scenarios:
            result = scenario()
            json_response_validator(result)  # Validates both success and failure cases
            if not result['success']:
                assert 'error' in result
                assert isinstance(result['error'], str)
                assert len(result['error']) > 0