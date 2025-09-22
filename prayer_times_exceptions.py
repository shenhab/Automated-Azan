"""
Custom exception classes for the Prayer Times Fetcher module.
These provide more specific error handling and better debugging information.
"""

from typing import Optional, Any, Dict


class PrayerTimesError(Exception):
    """Base exception for all prayer times related errors."""

    def __init__(self, message: str, details: Optional[Dict[str, Any]] = None):
        super().__init__(message)
        self.message = message
        self.details = details or {}

    def to_dict(self) -> Dict[str, Any]:
        """Convert exception to dictionary format for API responses."""
        return {
            "success": False,
            "error": self.message,
            "error_type": self.__class__.__name__,
            "details": self.details
        }


class InvalidLocationError(PrayerTimesError):
    """Raised when an invalid location is specified."""

    def __init__(self, location: str, valid_locations: list):
        message = f"Invalid location '{location}'. Valid locations are: {', '.join(valid_locations)}"
        super().__init__(
            message,
            {"location": location, "valid_locations": valid_locations}
        )


class DataNotFoundError(PrayerTimesError):
    """Raised when prayer data is not found for a specific date."""

    def __init__(self, location: str, date: str, reason: str = ""):
        message = f"Prayer data not found for {location} on {date}"
        if reason:
            message += f": {reason}"
        super().__init__(
            message,
            {"location": location, "date": date, "reason": reason}
        )


class NetworkError(PrayerTimesError):
    """Raised when a network request fails."""

    def __init__(self, url: str, original_error: Exception):
        message = f"Network request failed for {url}: {str(original_error)}"
        super().__init__(
            message,
            {"url": url, "original_error": str(original_error)}
        )


class FileOperationError(PrayerTimesError):
    """Raised when file operations fail (read/write)."""

    def __init__(self, file_path: str, operation: str, original_error: Exception):
        message = f"File {operation} failed for {file_path}: {str(original_error)}"
        super().__init__(
            message,
            {
                "file_path": file_path,
                "operation": operation,
                "original_error": str(original_error)
            }
        )


class ValidationError(PrayerTimesError):
    """Raised when data validation fails."""

    def __init__(self, validation_type: str, details: Dict[str, Any]):
        message = f"Validation failed: {validation_type}"
        super().__init__(message, details)


class ParsingError(PrayerTimesError):
    """Raised when parsing data fails (JSON, HTML scraping, etc.)."""

    def __init__(self, source: str, data_type: str, original_error: Optional[Exception] = None):
        message = f"Failed to parse {data_type} from {source}"
        if original_error:
            message += f": {str(original_error)}"
        super().__init__(
            message,
            {
                "source": source,
                "data_type": data_type,
                "original_error": str(original_error) if original_error else None
            }
        )


class ConfigurationError(PrayerTimesError):
    """Raised when configuration is invalid or missing."""

    def __init__(self, config_item: str, reason: str):
        message = f"Configuration error for '{config_item}': {reason}"
        super().__init__(
            message,
            {"config_item": config_item, "reason": reason}
        )


class RefreshRequiredError(PrayerTimesError):
    """Raised when data needs to be refreshed but refresh fails."""

    def __init__(self, location: str, reason: str):
        message = f"Data refresh required for {location}: {reason}"
        super().__init__(
            message,
            {"location": location, "reason": reason}
        )