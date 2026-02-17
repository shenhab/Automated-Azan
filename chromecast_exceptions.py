"""
Chromecast Manager Exception Classes

This module defines custom exceptions for better error handling and debugging
in the Chromecast Manager system.
"""

from typing import Optional, Dict, Any
from datetime import datetime


class ChromecastError(Exception):
    """Base exception for all Chromecast-related errors"""

    def __init__(
        self,
        message: str,
        error_code: Optional[str] = None,
        details: Optional[Dict[str, Any]] = None
    ):
        super().__init__(message)
        self.message = message
        self.error_code = error_code or self.__class__.__name__
        self.details = details or {}
        self.timestamp = datetime.now().isoformat()

    def to_dict(self) -> Dict[str, Any]:
        """Convert exception to dictionary for JSON response"""
        return {
            'success': False,
            'error': self.message,
            'error_code': self.error_code,
            'error_type': self.__class__.__name__,
            'details': self.details,
            'timestamp': self.timestamp
        }


# Discovery Exceptions
class DiscoveryError(ChromecastError):
    """Base exception for device discovery errors"""
    pass


class NoDevicesFoundError(DiscoveryError):
    """Raised when no Chromecast devices are discovered"""

    def __init__(self, message: str = "No Chromecast devices found on the network"):
        super().__init__(message, error_code="NO_DEVICES_FOUND")


class DiscoveryTimeoutError(DiscoveryError):
    """Raised when device discovery times out"""

    def __init__(self, timeout: int):
        super().__init__(
            f"Device discovery timed out after {timeout} seconds",
            error_code="DISCOVERY_TIMEOUT",
            details={'timeout_seconds': timeout}
        )


class DiscoveryCooldownError(DiscoveryError):
    """Raised when discovery is attempted within cooldown period"""

    def __init__(self, cooldown_remaining: float):
        super().__init__(
            f"Discovery cooldown active, {cooldown_remaining:.1f} seconds remaining",
            error_code="DISCOVERY_COOLDOWN",
            details={'cooldown_remaining': cooldown_remaining}
        )


# Connection Exceptions
class ConnectionError(ChromecastError):
    """Base exception for connection-related errors"""
    pass


class DeviceConnectionError(ConnectionError):
    """Raised when unable to connect to a specific device"""

    def __init__(self, device_name: str, host: str, reason: Optional[str] = None):
        message = f"Failed to connect to device '{device_name}' at {host}"
        if reason:
            message += f": {reason}"
        super().__init__(
            message,
            error_code="DEVICE_CONNECTION_FAILED",
            details={'device_name': device_name, 'host': host, 'reason': reason}
        )


class ConnectionTimeoutError(ConnectionError):
    """Raised when connection to device times out"""

    def __init__(self, device_name: str, timeout: int):
        super().__init__(
            f"Connection to '{device_name}' timed out after {timeout} seconds",
            error_code="CONNECTION_TIMEOUT",
            details={'device_name': device_name, 'timeout_seconds': timeout}
        )


class MaxRetriesExceededError(ConnectionError):
    """Raised when maximum connection retries are exceeded"""

    def __init__(self, device_name: str, max_retries: int):
        super().__init__(
            f"Maximum connection retries ({max_retries}) exceeded for '{device_name}'",
            error_code="MAX_RETRIES_EXCEEDED",
            details={'device_name': device_name, 'max_retries': max_retries}
        )


class ThreadingError(ConnectionError):
    """Raised when threading issues occur"""

    def __init__(self, message: str = "Threading error occurred"):
        super().__init__(
            message,
            error_code="THREADING_ERROR"
        )


# Playback Exceptions
class PlaybackError(ChromecastError):
    """Base exception for media playback errors"""
    pass


class MediaLoadError(PlaybackError):
    """Raised when media fails to load on device"""

    def __init__(self, url: str, device_name: str, reason: Optional[str] = None):
        message = f"Failed to load media '{url}' on '{device_name}'"
        if reason:
            message += f": {reason}"
        super().__init__(
            message,
            error_code="MEDIA_LOAD_FAILED",
            details={'url': url, 'device_name': device_name, 'reason': reason}
        )


class MediaTimeoutError(PlaybackError):
    """Raised when media loading times out"""

    def __init__(self, url: str, timeout: int):
        super().__init__(
            f"Media loading timed out after {timeout} attempts",
            error_code="MEDIA_TIMEOUT",
            details={'url': url, 'timeout_attempts': timeout}
        )


class PlaybackAlreadyActiveError(PlaybackError):
    """Raised when trying to play while another playback is active"""

    def __init__(self, current_media: str, elapsed_time: float):
        super().__init__(
            f"Playback already active: {current_media} (elapsed: {elapsed_time:.1f}s)",
            error_code="PLAYBACK_ACTIVE",
            details={'current_media': current_media, 'elapsed_time': elapsed_time}
        )


class InvalidMediaURLError(PlaybackError):
    """Raised when media URL is invalid"""

    def __init__(self, url: str):
        super().__init__(
            f"Invalid media URL: {url}",
            error_code="INVALID_MEDIA_URL",
            details={'url': url}
        )


# Device Exceptions
class DeviceError(ChromecastError):
    """Base exception for device-specific errors"""
    pass


class DeviceNotFoundError(DeviceError):
    """Raised when specified device is not found"""

    def __init__(self, device_name: str):
        super().__init__(
            f"Device '{device_name}' not found",
            error_code="DEVICE_NOT_FOUND",
            details={'device_name': device_name}
        )


class DeviceUnavailableError(DeviceError):
    """Raised when device is not available/responsive"""

    def __init__(self, device_name: str, host: str):
        super().__init__(
            f"Device '{device_name}' at {host} is not responding",
            error_code="DEVICE_UNAVAILABLE",
            details={'device_name': device_name, 'host': host}
        )


class NoSuitableDeviceError(DeviceError):
    """Raised when no suitable device is found for casting"""

    def __init__(self, devices_checked: int):
        super().__init__(
            f"No suitable Chromecast device found (checked {devices_checked} devices)",
            error_code="NO_SUITABLE_DEVICE",
            details={'devices_checked': devices_checked}
        )


# Athan Exceptions
class AthanError(ChromecastError):
    """Base exception for Athan-related errors"""
    pass


class AthanAlreadyPlayingError(AthanError):
    """Raised when Athan is already playing"""

    def __init__(self, prayer_type: str, elapsed_time: float):
        super().__init__(
            f"Athan is already playing ({prayer_type}, elapsed: {elapsed_time:.1f}s)",
            error_code="ATHAN_ALREADY_PLAYING",
            details={'prayer_type': prayer_type, 'elapsed_time': elapsed_time}
        )


class AthanPlaybackFailedError(AthanError):
    """Raised when Athan playback fails"""

    def __init__(self, prayer_type: str, reason: Optional[str] = None):
        message = f"Failed to play {prayer_type} Athan"
        if reason:
            message += f": {reason}"
        super().__init__(
            message,
            error_code="ATHAN_PLAYBACK_FAILED",
            details={'prayer_type': prayer_type, 'reason': reason}
        )


# Configuration Exceptions
class ConfigurationError(ChromecastError):
    """Base exception for configuration errors"""
    pass


class InvalidConfigurationError(ConfigurationError):
    """Raised when configuration is invalid"""

    def __init__(self, setting: str, value: Any, reason: str):
        super().__init__(
            f"Invalid configuration for '{setting}': {value} - {reason}",
            error_code="INVALID_CONFIGURATION",
            details={'setting': setting, 'value': value, 'reason': reason}
        )


# Circuit Breaker Exceptions
class CircuitBreakerError(ChromecastError):
    """Base exception for circuit breaker errors"""
    pass


class CircuitBreakerOpenError(CircuitBreakerError):
    """Raised when circuit breaker is open"""

    def __init__(self, device_name: str, reset_time: datetime):
        super().__init__(
            f"Circuit breaker open for device '{device_name}', will reset at {reset_time.isoformat()}",
            error_code="CIRCUIT_BREAKER_OPEN",
            details={'device_name': device_name, 'reset_time': reset_time.isoformat()}
        )


# Resource Management Exceptions
class ResourceError(ChromecastError):
    """Base exception for resource management errors"""
    pass


class CleanupError(ResourceError):
    """Raised when cleanup operations fail"""

    def __init__(self, resource: str, reason: Optional[str] = None):
        message = f"Failed to cleanup {resource}"
        if reason:
            message += f": {reason}"
        super().__init__(
            message,
            error_code="CLEANUP_FAILED",
            details={'resource': resource, 'reason': reason}
        )


class ResourceLockError(ResourceError):
    """Raised when unable to acquire resource lock"""

    def __init__(self, resource: str, timeout: float):
        super().__init__(
            f"Failed to acquire lock for {resource} within {timeout} seconds",
            error_code="RESOURCE_LOCK_TIMEOUT",
            details={'resource': resource, 'timeout': timeout}
        )