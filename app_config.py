"""
Application Configuration Module

This module provides modern, type-safe configuration management for the Automated Azan application.
It replaces the old config_manager.py with a dataclass-based approach similar to chromecast_config.py.
"""

import os
import logging
from typing import Optional, Dict, Any, List
from dataclasses import dataclass, field
from datetime import datetime
import configparser
from dotenv import load_dotenv


@dataclass
class LocationConfig:
    """Location configuration for prayer time calculations"""
    latitude: float = float(os.getenv('AZAN_LATITUDE', '51.5074'))
    longitude: float = float(os.getenv('AZAN_LONGITUDE', '-0.1278'))
    city: str = os.getenv('AZAN_CITY', 'London')
    country: str = os.getenv('AZAN_COUNTRY', 'UK')
    timezone: str = os.getenv('AZAN_TIMEZONE', 'Europe/London')


@dataclass
class PrayerConfig:
    """Prayer times configuration"""
    source: str = os.getenv('AZAN_PRAYER_SOURCE', 'NAAS')
    calculation_method: str = os.getenv('AZAN_CALCULATION_METHOD', 'ISNA')
    madhab: str = os.getenv('AZAN_MADHAB', 'Shafi')
    pre_fajr_enabled: bool = os.getenv('AZAN_PRE_FAJR_ENABLED', 'false').lower() == 'true'
    pre_fajr_minutes: int = int(os.getenv('AZAN_PRE_FAJR_MINUTES', '30'))
    cache_duration_hours: int = int(os.getenv('AZAN_CACHE_DURATION_HOURS', '24'))


@dataclass
class AudioConfig:
    """Audio and speaker configuration"""
    speakers_group_name: str = os.getenv('AZAN_SPEAKERS_GROUP', 'Living Room')
    volume_level: float = float(os.getenv('AZAN_VOLUME_LEVEL', '0.7'))
    fade_in_duration: float = float(os.getenv('AZAN_FADE_IN_DURATION', '2.0'))
    fade_out_duration: float = float(os.getenv('AZAN_FADE_OUT_DURATION', '3.0'))
    test_audio_enabled: bool = os.getenv('AZAN_TEST_AUDIO_ENABLED', 'true').lower() == 'true'


@dataclass
class WebConfig:
    """Web interface configuration"""
    host: str = os.getenv('AZAN_WEB_HOST', '0.0.0.0')
    port: int = int(os.getenv('AZAN_WEB_PORT', '5000'))
    debug: bool = os.getenv('AZAN_WEB_DEBUG', 'false').lower() == 'true'
    secret_key: str = os.getenv('AZAN_WEB_SECRET_KEY', 'automated-azan-secret-key')
    auto_reload: bool = os.getenv('AZAN_WEB_AUTO_RELOAD', 'false').lower() == 'true'
    cors_enabled: bool = os.getenv('AZAN_WEB_CORS_ENABLED', 'true').lower() == 'true'


@dataclass
class LoggingConfig:
    """Logging configuration"""
    level: str = os.getenv('AZAN_LOG_LEVEL', 'INFO')
    file_enabled: bool = os.getenv('AZAN_LOG_FILE_ENABLED', 'true').lower() == 'true'
    file_path: str = os.getenv('AZAN_LOG_FILE_PATH', 'logs/azan.log')
    file_max_size: int = int(os.getenv('AZAN_LOG_FILE_MAX_SIZE', '10485760'))  # 10MB
    file_backup_count: int = int(os.getenv('AZAN_LOG_FILE_BACKUP_COUNT', '5'))
    console_enabled: bool = os.getenv('AZAN_LOG_CONSOLE_ENABLED', 'true').lower() == 'true'
    format_string: str = os.getenv('AZAN_LOG_FORMAT',
        '%(asctime)s - %(name)s - %(levelname)s - %(message)s')


@dataclass
class NotificationConfig:
    """Notification and alert configuration"""
    enabled: bool = os.getenv('AZAN_NOTIFICATIONS_ENABLED', 'false').lower() == 'true'
    email_enabled: bool = os.getenv('AZAN_EMAIL_ENABLED', 'false').lower() == 'true'
    email_smtp_server: str = os.getenv('AZAN_EMAIL_SMTP_SERVER', 'smtp.gmail.com')
    email_smtp_port: int = int(os.getenv('AZAN_EMAIL_SMTP_PORT', '587'))
    email_username: str = os.getenv('AZAN_EMAIL_USERNAME', '')
    email_password: str = os.getenv('AZAN_EMAIL_PASSWORD', '')
    email_recipients: List[str] = field(default_factory=lambda:
        os.getenv('AZAN_EMAIL_RECIPIENTS', '').split(',') if os.getenv('AZAN_EMAIL_RECIPIENTS') else [])


@dataclass
class SecurityConfig:
    """Security and authentication configuration"""
    api_key_required: bool = os.getenv('AZAN_API_KEY_REQUIRED', 'false').lower() == 'true'
    api_key: str = os.getenv('AZAN_API_KEY', '')
    rate_limit_enabled: bool = os.getenv('AZAN_RATE_LIMIT_ENABLED', 'true').lower() == 'true'
    rate_limit_requests: int = int(os.getenv('AZAN_RATE_LIMIT_REQUESTS', '100'))
    rate_limit_window: int = int(os.getenv('AZAN_RATE_LIMIT_WINDOW', '3600'))  # 1 hour
    cors_origins: List[str] = field(default_factory=lambda:
        os.getenv('AZAN_CORS_ORIGINS', '*').split(','))


@dataclass
class AppConfig:
    """Main application configuration combining all subsystem configs"""

    # Subsystem configurations
    location: LocationConfig = field(default_factory=LocationConfig)
    prayer: PrayerConfig = field(default_factory=PrayerConfig)
    audio: AudioConfig = field(default_factory=AudioConfig)
    web: WebConfig = field(default_factory=WebConfig)
    logging: LoggingConfig = field(default_factory=LoggingConfig)
    notification: NotificationConfig = field(default_factory=NotificationConfig)
    security: SecurityConfig = field(default_factory=SecurityConfig)

    # General application settings
    app_name: str = os.getenv('AZAN_APP_NAME', 'Automated Azan')
    app_version: str = os.getenv('AZAN_APP_VERSION', '2.0.0')
    environment: str = os.getenv('AZAN_ENVIRONMENT', 'production')
    data_directory: str = os.getenv('AZAN_DATA_DIRECTORY', 'data')
    cache_directory: str = os.getenv('AZAN_CACHE_DIRECTORY', 'cache')

    # Legacy config file support
    config_file: str = os.getenv('AZAN_CONFIG_FILE', 'adahn.config')

    def __post_init__(self):
        """Initialize configuration after dataclass creation"""
        # Load environment variables
        load_dotenv()

        # Load legacy config file if it exists and override env vars
        self._load_legacy_config()

        # Validate configuration
        self._validate_config()

    def _load_legacy_config(self) -> None:
        """Load settings from legacy INI config file"""
        if not os.path.exists(self.config_file):
            logging.debug(f"Legacy config file {self.config_file} not found, using environment variables")
            return

        try:
            config = configparser.ConfigParser()
            config.read(self.config_file)

            # Location settings
            if config.has_section('Location'):
                if config.has_option('Location', 'latitude'):
                    self.location.latitude = config.getfloat('Location', 'latitude')
                if config.has_option('Location', 'longitude'):
                    self.location.longitude = config.getfloat('Location', 'longitude')
                if config.has_option('Location', 'city'):
                    self.location.city = config.get('Location', 'city')
                if config.has_option('Location', 'country'):
                    self.location.country = config.get('Location', 'country')

            # Prayer settings
            if config.has_section('Prayer'):
                if config.has_option('Prayer', 'source'):
                    self.prayer.source = config.get('Prayer', 'source')
                if config.has_option('Prayer', 'calculation_method'):
                    self.prayer.calculation_method = config.get('Prayer', 'calculation_method')
                if config.has_option('Prayer', 'madhab'):
                    self.prayer.madhab = config.get('Prayer', 'madhab')
                if config.has_option('Prayer', 'pre_fajr_enabled'):
                    self.prayer.pre_fajr_enabled = config.getboolean('Prayer', 'pre_fajr_enabled')

            # Audio settings
            if config.has_section('Audio'):
                if config.has_option('Audio', 'speakers_group'):
                    self.audio.speakers_group_name = config.get('Audio', 'speakers_group')
                if config.has_option('Audio', 'volume'):
                    self.audio.volume_level = config.getfloat('Audio', 'volume')

            # Web settings
            if config.has_section('Web'):
                if config.has_option('Web', 'host'):
                    self.web.host = config.get('Web', 'host')
                if config.has_option('Web', 'port'):
                    self.web.port = config.getint('Web', 'port')
                if config.has_option('Web', 'debug'):
                    self.web.debug = config.getboolean('Web', 'debug')

            logging.info(f"Loaded legacy configuration from {self.config_file}")

        except Exception as e:
            logging.warning(f"Error loading legacy config file {self.config_file}: {e}")

    def _validate_config(self) -> None:
        """Validate configuration values"""
        # Validate location
        if not (-90 <= self.location.latitude <= 90):
            raise ValueError(f"Invalid latitude: {self.location.latitude}")
        if not (-180 <= self.location.longitude <= 180):
            raise ValueError(f"Invalid longitude: {self.location.longitude}")

        # Validate audio
        if not (0.0 <= self.audio.volume_level <= 1.0):
            raise ValueError(f"Invalid volume level: {self.audio.volume_level}")

        # Validate web
        if not (1 <= self.web.port <= 65535):
            raise ValueError(f"Invalid web port: {self.web.port}")

        # Create directories if they don't exist
        os.makedirs(self.data_directory, exist_ok=True)
        os.makedirs(self.cache_directory, exist_ok=True)
        os.makedirs(os.path.dirname(self.logging.file_path), exist_ok=True)

    def to_dict(self) -> Dict[str, Any]:
        """Convert configuration to dictionary for JSON serialization"""
        return {
            'app': {
                'name': self.app_name,
                'version': self.app_version,
                'environment': self.environment,
                'data_directory': self.data_directory,
                'cache_directory': self.cache_directory
            },
            'location': {
                'latitude': self.location.latitude,
                'longitude': self.location.longitude,
                'city': self.location.city,
                'country': self.location.country,
                'timezone': self.location.timezone
            },
            'prayer': {
                'source': self.prayer.source,
                'calculation_method': self.prayer.calculation_method,
                'madhab': self.prayer.madhab,
                'pre_fajr_enabled': self.prayer.pre_fajr_enabled,
                'pre_fajr_minutes': self.prayer.pre_fajr_minutes,
                'cache_duration_hours': self.prayer.cache_duration_hours
            },
            'audio': {
                'speakers_group_name': self.audio.speakers_group_name,
                'volume_level': self.audio.volume_level,
                'fade_in_duration': self.audio.fade_in_duration,
                'fade_out_duration': self.audio.fade_out_duration,
                'test_audio_enabled': self.audio.test_audio_enabled
            },
            'web': {
                'host': self.web.host,
                'port': self.web.port,
                'debug': self.web.debug,
                'auto_reload': self.web.auto_reload,
                'cors_enabled': self.web.cors_enabled
            },
            'logging': {
                'level': self.logging.level,
                'file_enabled': self.logging.file_enabled,
                'file_path': self.logging.file_path,
                'console_enabled': self.logging.console_enabled
            },
            'notification': {
                'enabled': self.notification.enabled,
                'email_enabled': self.notification.email_enabled,
                'email_smtp_server': self.notification.email_smtp_server,
                'email_smtp_port': self.notification.email_smtp_port
            },
            'security': {
                'api_key_required': self.security.api_key_required,
                'rate_limit_enabled': self.security.rate_limit_enabled,
                'rate_limit_requests': self.security.rate_limit_requests,
                'rate_limit_window': self.security.rate_limit_window
            }
        }

    @classmethod
    def from_dict(cls, config_dict: Dict[str, Any]) -> 'AppConfig':
        """Create configuration from dictionary"""
        config = cls()

        # Update from dictionary (simplified implementation)
        if 'location' in config_dict:
            location_data = config_dict['location']
            config.location = LocationConfig(
                latitude=location_data.get('latitude', config.location.latitude),
                longitude=location_data.get('longitude', config.location.longitude),
                city=location_data.get('city', config.location.city),
                country=location_data.get('country', config.location.country),
                timezone=location_data.get('timezone', config.location.timezone)
            )

        # Add similar updates for other sections as needed...

        return config

    def get_json_response(self, success: bool = True, message: str = "") -> Dict[str, Any]:
        """Get configuration as JSON API response"""
        return {
            'success': success,
            'message': message,
            'config': self.to_dict(),
            'timestamp': datetime.now().isoformat()
        }


# Create singleton instance
default_app_config = AppConfig()


def get_app_config() -> AppConfig:
    """Get the current application configuration instance"""
    return default_app_config


def update_app_config(config_dict: Dict[str, Any]) -> AppConfig:
    """Update application configuration from dictionary"""
    global default_app_config
    default_app_config = AppConfig.from_dict(config_dict)
    return default_app_config


# Backward compatibility functions for legacy code
def get_setting(section: str, key: str, fallback: Any = None) -> Any:
    """
    Legacy compatibility function for old config_manager.get_setting() calls
    """
    config = get_app_config()

    # Map legacy section.key to new config structure
    section_map = {
        'Location': config.location,
        'Prayer': config.prayer,
        'Audio': config.audio,
        'Web': config.web,
        'Logging': config.logging
    }

    if section not in section_map:
        return fallback

    section_obj = section_map[section]
    return getattr(section_obj, key, fallback)


def get_speakers_group_name() -> str:
    """Legacy compatibility function"""
    return get_app_config().audio.speakers_group_name


def get_location() -> Dict[str, Any]:
    """Legacy compatibility function"""
    location = get_app_config().location
    return {
        'latitude': location.latitude,
        'longitude': location.longitude,
        'city': location.city,
        'country': location.country
    }


def get_prayer_source() -> str:
    """Legacy compatibility function"""
    return get_app_config().prayer.source


def is_pre_fajr_enabled() -> bool:
    """Legacy compatibility function"""
    return get_app_config().prayer.pre_fajr_enabled