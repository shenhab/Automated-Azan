"""
Chromecast Manager Configuration Module

This module contains all configuration constants and settings for the Chromecast Manager.
Settings can be overridden using environment variables.
"""

import os
from typing import List, Dict, Any
from dataclasses import dataclass, field


@dataclass
class ChromecastConfig:
    """Configuration for Chromecast Manager"""

    # Discovery Settings
    DISCOVERY_COOLDOWN_SECONDS: int = int(os.getenv('CHROMECAST_DISCOVERY_COOLDOWN', '30'))
    DISCOVERY_TIMEOUT_SECONDS: int = int(os.getenv('CHROMECAST_DISCOVERY_TIMEOUT', '8'))
    DISCOVERY_MAX_ATTEMPTS: int = int(os.getenv('CHROMECAST_DISCOVERY_MAX_ATTEMPTS', '15'))
    DISCOVERY_CALLBACK_TIMEOUT: int = int(os.getenv('CHROMECAST_DISCOVERY_CALLBACK_TIMEOUT', '3'))

    # Connection Settings
    CONNECTION_TIMEOUT_SECONDS: int = int(os.getenv('CHROMECAST_CONNECTION_TIMEOUT', '10'))
    CONNECTION_MAX_RETRIES: int = int(os.getenv('CHROMECAST_CONNECTION_MAX_RETRIES', '3'))
    CONNECTION_RETRY_DELAY: float = float(os.getenv('CHROMECAST_CONNECTION_RETRY_DELAY', '2.0'))
    CONNECTION_PORT_DEFAULT: int = int(os.getenv('CHROMECAST_DEFAULT_PORT', '8009'))
    SOCKET_TIMEOUT_SECONDS: int = int(os.getenv('CHROMECAST_SOCKET_TIMEOUT', '3'))

    # Playback Settings
    PLAYBACK_MAX_RETRIES: int = int(os.getenv('CHROMECAST_PLAYBACK_MAX_RETRIES', '2'))
    MEDIA_LOAD_MAX_ATTEMPTS: int = int(os.getenv('CHROMECAST_MEDIA_LOAD_MAX_ATTEMPTS', '15'))
    MEDIA_LOAD_INITIAL_WAIT: float = float(os.getenv('CHROMECAST_MEDIA_LOAD_INITIAL_WAIT', '0.5'))
    MEDIA_LOAD_SHORT_WAIT: float = float(os.getenv('CHROMECAST_MEDIA_LOAD_SHORT_WAIT', '1.0'))
    MEDIA_LOAD_MEDIUM_WAIT: float = float(os.getenv('CHROMECAST_MEDIA_LOAD_MEDIUM_WAIT', '2.0'))
    MEDIA_LOAD_LONG_WAIT: float = float(os.getenv('CHROMECAST_MEDIA_LOAD_LONG_WAIT', '3.0'))
    MEDIA_STOP_WAIT: float = float(os.getenv('CHROMECAST_MEDIA_STOP_WAIT', '1.5'))
    MEDIA_RESTART_WAIT: float = float(os.getenv('CHROMECAST_MEDIA_RESTART_WAIT', '1.0'))

    # Athan Settings
    ATHAN_TIMEOUT_SECONDS: int = int(os.getenv('ATHAN_TIMEOUT_SECONDS', '480'))  # 8 minutes
    ATHAN_REGULAR_FILENAME: str = os.getenv('ATHAN_REGULAR_FILENAME', 'media_Athan.mp3')
    ATHAN_FAJR_FILENAME: str = os.getenv('ATHAN_FAJR_FILENAME', 'media_adhan_al_fajr.mp3')

    # Device Priority Settings
    PRIMARY_DEVICE_NAME: str = os.getenv('CHROMECAST_PRIMARY_DEVICE', 'Adahn')
    FALLBACK_DEVICE_MODELS: List[str] = field(default_factory=lambda: [
        "Google Nest Mini",
        "Google Nest Hub",
        "Google Home",
        "Google Home Mini"
    ])

    # Web Interface Settings
    WEB_INTERFACE_HOST: str = os.getenv('WEB_INTERFACE_HOST', '0.0.0.0')
    WEB_INTERFACE_PORT: int = int(os.getenv('WEB_INTERFACE_PORT', '5000'))

    # Stream URLs
    QURAN_RADIO_URL: str = os.getenv(
        'QURAN_RADIO_URL',
        'https://n03.radiojar.com/8s5u5tpdtwzuv?rj-ttl=5&rj-tok=AAABlVflrAAAJLe-IOoD4VTShA'
    )

    # Performance Settings
    CONSECUTIVE_PLAYING_THRESHOLD: int = int(os.getenv('CHROMECAST_CONSECUTIVE_PLAYING_THRESHOLD', '2'))
    IDLE_STATE_CONCERN_THRESHOLD: int = int(os.getenv('CHROMECAST_IDLE_STATE_CONCERN_THRESHOLD', '8'))

    # Logging Settings
    LOG_LEVEL: str = os.getenv('CHROMECAST_LOG_LEVEL', 'INFO')
    LOG_DEBUG_DISCOVERY: bool = os.getenv('CHROMECAST_LOG_DEBUG_DISCOVERY', 'false').lower() == 'true'

    # Circuit Breaker Settings
    CIRCUIT_BREAKER_FAILURE_THRESHOLD: int = int(os.getenv('CIRCUIT_BREAKER_FAILURE_THRESHOLD', '5'))
    CIRCUIT_BREAKER_RECOVERY_TIMEOUT: int = int(os.getenv('CIRCUIT_BREAKER_RECOVERY_TIMEOUT', '60'))
    CIRCUIT_BREAKER_EXPECTED_EXCEPTION_TYPE: str = os.getenv('CIRCUIT_BREAKER_EXCEPTION_TYPE', 'Exception')

    # Caching Settings
    DEVICE_CACHE_TTL_SECONDS: int = int(os.getenv('DEVICE_CACHE_TTL_SECONDS', '300'))  # 5 minutes
    DEVICE_VALIDATION_INTERVAL: int = int(os.getenv('DEVICE_VALIDATION_INTERVAL', '60'))  # 1 minute

    # Health Check Settings
    HEALTH_CHECK_INTERVAL_SECONDS: int = int(os.getenv('HEALTH_CHECK_INTERVAL_SECONDS', '30'))
    HEALTH_CHECK_TIMEOUT_SECONDS: int = int(os.getenv('HEALTH_CHECK_TIMEOUT_SECONDS', '5'))

    def to_dict(self) -> Dict[str, Any]:
        """Convert configuration to dictionary for JSON serialization"""
        return {
            'discovery': {
                'cooldown_seconds': self.DISCOVERY_COOLDOWN_SECONDS,
                'timeout_seconds': self.DISCOVERY_TIMEOUT_SECONDS,
                'max_attempts': self.DISCOVERY_MAX_ATTEMPTS,
                'callback_timeout': self.DISCOVERY_CALLBACK_TIMEOUT,
            },
            'connection': {
                'timeout_seconds': self.CONNECTION_TIMEOUT_SECONDS,
                'max_retries': self.CONNECTION_MAX_RETRIES,
                'retry_delay': self.CONNECTION_RETRY_DELAY,
                'default_port': self.CONNECTION_PORT_DEFAULT,
                'socket_timeout': self.SOCKET_TIMEOUT_SECONDS,
            },
            'playback': {
                'max_retries': self.PLAYBACK_MAX_RETRIES,
                'media_load_max_attempts': self.MEDIA_LOAD_MAX_ATTEMPTS,
                'wait_times': {
                    'initial': self.MEDIA_LOAD_INITIAL_WAIT,
                    'short': self.MEDIA_LOAD_SHORT_WAIT,
                    'medium': self.MEDIA_LOAD_MEDIUM_WAIT,
                    'long': self.MEDIA_LOAD_LONG_WAIT,
                    'stop': self.MEDIA_STOP_WAIT,
                    'restart': self.MEDIA_RESTART_WAIT,
                }
            },
            'athan': {
                'timeout_seconds': self.ATHAN_TIMEOUT_SECONDS,
                'regular_filename': self.ATHAN_REGULAR_FILENAME,
                'fajr_filename': self.ATHAN_FAJR_FILENAME,
            },
            'devices': {
                'primary_name': self.PRIMARY_DEVICE_NAME,
                'fallback_models': self.FALLBACK_DEVICE_MODELS,
            },
            'web_interface': {
                'host': self.WEB_INTERFACE_HOST,
                'port': self.WEB_INTERFACE_PORT,
            },
            'streams': {
                'quran_radio_url': self.QURAN_RADIO_URL,
            },
            'circuit_breaker': {
                'failure_threshold': self.CIRCUIT_BREAKER_FAILURE_THRESHOLD,
                'recovery_timeout': self.CIRCUIT_BREAKER_RECOVERY_TIMEOUT,
            },
            'caching': {
                'device_cache_ttl': self.DEVICE_CACHE_TTL_SECONDS,
                'validation_interval': self.DEVICE_VALIDATION_INTERVAL,
            },
            'health_check': {
                'interval_seconds': self.HEALTH_CHECK_INTERVAL_SECONDS,
                'timeout_seconds': self.HEALTH_CHECK_TIMEOUT_SECONDS,
            }
        }

    @classmethod
    def from_dict(cls, config_dict: Dict[str, Any]) -> 'ChromecastConfig':
        """Create configuration from dictionary"""
        config = cls()

        # Update discovery settings
        if 'discovery' in config_dict:
            discovery = config_dict['discovery']
            config.DISCOVERY_COOLDOWN_SECONDS = discovery.get('cooldown_seconds', config.DISCOVERY_COOLDOWN_SECONDS)
            config.DISCOVERY_TIMEOUT_SECONDS = discovery.get('timeout_seconds', config.DISCOVERY_TIMEOUT_SECONDS)
            config.DISCOVERY_MAX_ATTEMPTS = discovery.get('max_attempts', config.DISCOVERY_MAX_ATTEMPTS)

        # Update connection settings
        if 'connection' in config_dict:
            connection = config_dict['connection']
            config.CONNECTION_TIMEOUT_SECONDS = connection.get('timeout_seconds', config.CONNECTION_TIMEOUT_SECONDS)
            config.CONNECTION_MAX_RETRIES = connection.get('max_retries', config.CONNECTION_MAX_RETRIES)
            config.CONNECTION_RETRY_DELAY = connection.get('retry_delay', config.CONNECTION_RETRY_DELAY)

        # Add more sections as needed...

        return config


# Create a singleton instance
default_config = ChromecastConfig()


def get_config() -> ChromecastConfig:
    """Get the current configuration instance"""
    return default_config


def update_config(config_dict: Dict[str, Any]) -> ChromecastConfig:
    """Update configuration from dictionary"""
    global default_config
    default_config = ChromecastConfig.from_dict(config_dict)
    return default_config