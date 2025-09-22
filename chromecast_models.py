"""
Chromecast Manager Data Models and Type Definitions

This module defines TypedDict classes and data models for type safety
and consistent API responses throughout the Chromecast Manager system.
"""

from typing import TypedDict, Optional, List, Dict, Any, Union
from datetime import datetime
from enum import Enum
from dataclasses import dataclass, field


class PlayerState(str, Enum):
    """Media player states"""
    IDLE = "IDLE"
    PLAYING = "PLAYING"
    PAUSED = "PAUSED"
    BUFFERING = "BUFFERING"
    UNKNOWN = "UNKNOWN"


class DiscoveryMethod(str, Enum):
    """Device discovery methods"""
    CASTBROWSER = "castbrowser"
    GET_CHROMECASTS = "get_chromecasts"
    HYBRID = "hybrid"
    FALLBACK = "fallback"


class DeviceSource(str, Enum):
    """Device source types"""
    CACHED = "cached"
    DISCOVERED = "discovered"
    PRIMARY_TARGET = "primary_target"
    FALLBACK_CANDIDATE = "fallback_candidate"


class PrayerType(str, Enum):
    """Prayer types for Athan"""
    REGULAR = "regular"
    FAJR = "fajr"


# TypedDict definitions for API responses

class DeviceInfo(TypedDict):
    """Device information structure"""
    uuid: str
    name: str
    host: str
    port: int
    model_name: str
    manufacturer: str
    available: Optional[bool]


class SimpleDeviceInfo(TypedDict):
    """Simplified device information"""
    name: str
    model: str
    host: str
    port: int


class ConnectionAttempt(TypedDict):
    """Connection attempt details"""
    attempt: int
    error: Optional[str]
    connection_time: float


class StatusCheck(TypedDict):
    """Media status check details"""
    attempt: int
    player_state: Optional[str]
    content_id: Optional[str]
    expected_url: str
    content_matches: bool
    check_time: float
    error: Optional[str]


class PlaybackAttempt(TypedDict):
    """Playback attempt details"""
    attempt: int
    error: str
    attempt_time: float
    connection_result: Optional[Dict[str, Any]]
    load_result: Optional[Dict[str, Any]]


class BaseResponse(TypedDict):
    """Base response structure for all API calls"""
    success: bool
    timestamp: str
    error: Optional[str]


class DiscoveryResponse(BaseResponse):
    """Response for device discovery operations"""
    method: Optional[str]
    devices_found: int
    devices: Dict[str, DeviceInfo]
    skipped: Optional[bool]
    reason: Optional[str]
    discovery_result: Optional[Dict[str, Any]]


class DeviceListResponse(BaseResponse):
    """Response for getting device list"""
    devices_count: int
    devices: List[DeviceInfo]
    last_discovery_time: Optional[str]


class ConnectionResponse(BaseResponse):
    """Response for connection operations"""
    device_name: str
    attempts: int
    connection_time: float
    connection_attempts: List[ConnectionAttempt]


class MediaLoadResponse(BaseResponse):
    """Response for media loading operations"""
    player_state: Optional[str]
    content_id: Optional[str]
    attempts: int
    consecutive_good_states: Optional[int]
    status_checks: List[StatusCheck]
    final_state: Optional[str]


class PlaybackResponse(BaseResponse):
    """Response for playback operations"""
    url: str
    device: Optional[SimpleDeviceInfo]
    device_source: Optional[str]
    attempts: int
    total_time: Optional[float]
    connection_result: Optional[ConnectionResponse]
    load_result: Optional[MediaLoadResponse]
    playback_attempts: Optional[List[PlaybackAttempt]]


class AthanStatusResponse(BaseResponse):
    """Response for Athan status"""
    playing: bool
    elapsed_time: Optional[float]
    start_time: Optional[str]
    device_name: Optional[str]
    message: str


class AthanPlaybackResponse(BaseResponse):
    """Response for Athan playback operations"""
    prayer_type: str
    media_url: Optional[str]
    start_time: Optional[str]
    playback_result: Optional[PlaybackResponse]
    skipped: Optional[bool]
    reason: Optional[str]
    current_status: Optional[AthanStatusResponse]
    message: Optional[str]


class DeviceStatusResponse(BaseResponse):
    """Response for device status check"""
    status: str
    device: Optional[SimpleDeviceInfo]
    availability_check: Optional[Dict[str, Any]]
    message: Optional[str]


class SystemStatusResponse(BaseResponse):
    """Response for system status"""
    system_status: Dict[str, Any]


class CleanupAction(TypedDict):
    """Cleanup action details"""
    action: str
    success: bool
    error: Optional[str]
    devices_cleared: Optional[int]
    device_name: Optional[str]
    was_playing: Optional[bool]


class CleanupResponse(BaseResponse):
    """Response for cleanup operations"""
    cleanup_actions: List[CleanupAction]
    message: str


class MediaURLResponse(BaseResponse):
    """Response for media URL generation"""
    filename: str
    media_url: Optional[str]
    local_ip: Optional[str]
    fallback_url: Optional[str]


class AvailabilityCheckResponse(BaseResponse):
    """Response for device availability check"""
    available: bool
    device_name: str
    host: str
    port: int
    response_code: Optional[int]


# Dataclass models for internal state management

@dataclass
class CastDevice:
    """Internal representation of a Chromecast device"""
    uuid: str
    name: str
    host: str
    port: int
    model_name: str
    manufacturer: str
    service: Optional[Any] = None
    cast_object: Optional[Any] = None
    last_seen: datetime = field(default_factory=datetime.now)
    last_validated: Optional[datetime] = None
    is_available: bool = True
    connection_failures: int = 0

    def to_dict(self) -> DeviceInfo:
        """Convert to dictionary for API response"""
        return {
            'uuid': self.uuid,
            'name': self.name,
            'host': self.host,
            'port': self.port,
            'model_name': self.model_name,
            'manufacturer': self.manufacturer,
            'available': self.is_available
        }

    def update_availability(self, available: bool) -> None:
        """Update device availability status"""
        self.is_available = available
        self.last_validated = datetime.now()
        if not available:
            self.connection_failures += 1
        else:
            self.connection_failures = 0


@dataclass
class PlaybackState:
    """Internal representation of playback state"""
    is_playing: bool = False
    start_time: Optional[datetime] = None
    media_url: Optional[str] = None
    prayer_type: Optional[PrayerType] = None
    device_name: Optional[str] = None

    def get_elapsed_time(self) -> float:
        """Get elapsed playback time in seconds"""
        if not self.start_time:
            return 0.0
        return (datetime.now() - self.start_time).total_seconds()

    def clear(self) -> None:
        """Clear playback state"""
        self.is_playing = False
        self.start_time = None
        self.media_url = None
        self.prayer_type = None
        self.device_name = None


@dataclass
class ConnectionStats:
    """Connection statistics for a device"""
    device_name: str
    total_attempts: int = 0
    successful_connections: int = 0
    failed_connections: int = 0
    last_success: Optional[datetime] = None
    last_failure: Optional[datetime] = None
    average_connection_time: float = 0.0
    connection_times: List[float] = field(default_factory=list)

    def add_connection_result(self, success: bool, connection_time: float) -> None:
        """Record a connection attempt"""
        self.total_attempts += 1
        self.connection_times.append(connection_time)

        if success:
            self.successful_connections += 1
            self.last_success = datetime.now()
        else:
            self.failed_connections += 1
            self.last_failure = datetime.now()

        # Keep only last 100 connection times
        if len(self.connection_times) > 100:
            self.connection_times = self.connection_times[-100:]

        # Update average
        if self.connection_times:
            self.average_connection_time = sum(self.connection_times) / len(self.connection_times)

    def get_success_rate(self) -> float:
        """Get connection success rate"""
        if self.total_attempts == 0:
            return 0.0
        return self.successful_connections / self.total_attempts


@dataclass
class DiscoveryStats:
    """Discovery operation statistics"""
    total_discoveries: int = 0
    successful_discoveries: int = 0
    failed_discoveries: int = 0
    last_discovery: Optional[datetime] = None
    devices_found_history: List[int] = field(default_factory=list)
    discovery_methods_used: Dict[str, int] = field(default_factory=dict)

    def add_discovery_result(self, success: bool, devices_found: int, method: str) -> None:
        """Record a discovery attempt"""
        self.total_discoveries += 1
        self.last_discovery = datetime.now()

        if success:
            self.successful_discoveries += 1
            self.devices_found_history.append(devices_found)
        else:
            self.failed_discoveries += 1
            self.devices_found_history.append(0)

        # Keep only last 50 discovery results
        if len(self.devices_found_history) > 50:
            self.devices_found_history = self.devices_found_history[-50:]

        # Track method usage
        if method not in self.discovery_methods_used:
            self.discovery_methods_used[method] = 0
        self.discovery_methods_used[method] += 1

    def get_average_devices_found(self) -> float:
        """Get average number of devices found"""
        if not self.devices_found_history:
            return 0.0
        return sum(self.devices_found_history) / len(self.devices_found_history)