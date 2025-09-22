"""
Configuration module for Prayer Times Fetcher.
Centralizes all configuration settings for easy management and testing.
"""

import os
from typing import Dict, List, Optional
from dataclasses import dataclass, field
from dateutil import tz


@dataclass
class PrayerTimesConfig:
    """Configuration class for Prayer Times Fetcher."""

    # Prayer time sources URLs
    sources: Dict[str, str] = field(default_factory=lambda: {
        "icci": "https://islamireland.ie/api/timetable/",
        "naas": "https://mawaqit.net/en/m/-34"
    })

    # Valid locations
    valid_locations: List[str] = field(default_factory=lambda: ["icci", "naas"])

    # Required prayers for validation
    required_prayers: List[str] = field(default_factory=lambda: [
        "Fajr", "Dhuhr", "Asr", "Maghrib", "Isha"
    ])

    # File paths
    data_directory: str = field(default_factory=lambda: "data" if os.path.exists("data") else ".")
    icci_filename: str = "icci_timetable.json"
    naas_filename: str = "naas_prayers_timetable.json"

    # Timezone
    timezone_str: str = "Europe/Dublin"

    # Network settings
    request_timeout: int = 10  # seconds
    max_retry_attempts: int = 3
    retry_backoff_seconds: int = 5

    # Cache settings
    cache_enabled: bool = True
    cache_ttl_seconds: int = 3600  # 1 hour

    # Logging settings
    log_level: str = "INFO"

    def __post_init__(self):
        """Post-initialization to set up derived attributes."""
        self.timezone = tz.gettz(self.timezone_str)
        self.icci_timetable_path = os.path.join(self.data_directory, self.icci_filename)
        self.naas_timetable_path = os.path.join(self.data_directory, self.naas_filename)

    def get_timetable_path(self, location: str) -> str:
        """
        Get the timetable file path for a specific location.

        Args:
            location: The location ('icci' or 'naas')

        Returns:
            The full path to the timetable file
        """
        if location == "icci":
            return self.icci_timetable_path
        elif location == "naas":
            return self.naas_timetable_path
        else:
            raise ValueError(f"Invalid location: {location}")

    def get_source_url(self, location: str) -> str:
        """
        Get the source URL for a specific location.

        Args:
            location: The location ('icci' or 'naas')

        Returns:
            The URL for the prayer times source
        """
        if location not in self.sources:
            raise ValueError(f"No source URL configured for location: {location}")
        return self.sources[location]

    def is_valid_location(self, location: str) -> bool:
        """
        Check if a location is valid.

        Args:
            location: The location to validate

        Returns:
            True if the location is valid, False otherwise
        """
        return location in self.valid_locations

    def to_dict(self) -> Dict:
        """
        Convert configuration to dictionary.

        Returns:
            Dictionary representation of the configuration
        """
        return {
            "sources": self.sources,
            "valid_locations": self.valid_locations,
            "required_prayers": self.required_prayers,
            "data_directory": self.data_directory,
            "timezone": self.timezone_str,
            "request_timeout": self.request_timeout,
            "max_retry_attempts": self.max_retry_attempts,
            "cache_enabled": self.cache_enabled,
            "cache_ttl_seconds": self.cache_ttl_seconds
        }


def load_config(config_file: Optional[str] = None) -> PrayerTimesConfig:
    """
    Load configuration from a file or use defaults.

    Args:
        config_file: Optional path to a configuration file (JSON)

    Returns:
        PrayerTimesConfig instance
    """
    if config_file and os.path.exists(config_file):
        import json
        with open(config_file, 'r') as f:
            config_data = json.load(f)
        return PrayerTimesConfig(**config_data)
    else:
        return PrayerTimesConfig()