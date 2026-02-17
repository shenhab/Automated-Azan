"""
Chromecast Connection Management Module

This module handles connection pooling, health checks, and device availability
monitoring for Chromecast devices.
"""

import socket
import time
import threading
import logging
from typing import Dict, Optional, Any, List
from datetime import datetime, timedelta
from contextlib import contextmanager
from dataclasses import dataclass, field
from uuid import UUID
import hashlib

import pychromecast
from pychromecast.models import CastInfo

# Import from parent directory
import sys
import os
sys.path.append(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from chromecast_config import ChromecastConfig, get_config
from chromecast_exceptions import (
    DeviceConnectionError, ConnectionTimeoutError, MaxRetriesExceededError,
    DeviceUnavailableError, ThreadingError, ResourceLockError
)
from chromecast_models import (
    CastDevice, ConnectionStats, ConnectionResponse, ConnectionAttempt,
    AvailabilityCheckResponse
)
from chromecast.circuit_breaker import CircuitBreakerManager


@dataclass
class DeviceConnection:
    """Represents a connection to a Chromecast device"""
    device: CastDevice
    cast_object: Optional[Any] = None
    last_used: datetime = field(default_factory=datetime.now)
    last_validated: Optional[datetime] = None
    is_connected: bool = False
    connection_count: int = 0
    lock: threading.Lock = field(default_factory=threading.Lock)


class DeviceConnectionPool:
    """
    Manages a pool of connections to Chromecast devices.

    Features:
    - Connection pooling and reuse
    - Automatic health checks
    - Connection validation
    - Circuit breaker integration
    - Thread-safe operations
    """

    def __init__(self, config: Optional[ChromecastConfig] = None):
        self.config = config or get_config()
        self.connections: Dict[str, DeviceConnection] = {}
        self.stats: Dict[str, ConnectionStats] = {}
        self.circuit_breaker_manager = CircuitBreakerManager(config)
        self.lock = threading.Lock()
        self.health_check_thread: Optional[threading.Thread] = None
        self.stop_health_checks = threading.Event()

        # Start health check thread
        self._start_health_checks()

    def get_connection(
        self,
        device: CastDevice,
        timeout: Optional[int] = None
    ) -> Optional[Any]:
        """
        Get a connection to a Chromecast device.

        Args:
            device: CastDevice to connect to
            timeout: Connection timeout in seconds

        Returns:
            Chromecast connection object or None

        Raises:
            DeviceConnectionError: If connection fails
            CircuitBreakerOpenError: If circuit breaker is open
        """
        timeout = timeout or self.config.CONNECTION_TIMEOUT_SECONDS
        device_key = device.uuid

        # Check circuit breaker
        breaker = self.circuit_breaker_manager.get_breaker(device.name)

        def connect():
            return self._connect_to_device(device, timeout)

        try:
            # Try to get existing connection
            with self.lock:
                if device_key in self.connections:
                    conn = self.connections[device_key]
                    if self._validate_connection(conn):
                        conn.last_used = datetime.now()
                        conn.connection_count += 1
                        logging.debug(f"Reusing connection to {device.name}")
                        return conn.cast_object

            # Create new connection through circuit breaker
            cast_object = breaker.call(connect)

            # Store connection
            with self.lock:
                self.connections[device_key] = DeviceConnection(
                    device=device,
                    cast_object=cast_object,
                    is_connected=True,
                    connection_count=1
                )
                self._update_stats(device.name, True, 0)

            return cast_object

        except Exception as e:
            self._update_stats(device.name, False, 0)
            raise

    def _connect_to_device(
        self,
        device: CastDevice,
        timeout: int
    ) -> Any:
        """
        Create a new connection to a device.

        Args:
            device: Device to connect to
            timeout: Connection timeout

        Returns:
            Chromecast object

        Raises:
            DeviceConnectionError: If connection fails
        """
        start_time = time.time()

        try:
            # First check if device is reachable
            if not self._is_device_reachable(device):
                raise DeviceUnavailableError(device.name, device.host)

            # Create Chromecast object
            cast_object = self._create_cast_object(device)
            if not cast_object:
                raise DeviceConnectionError(
                    device.name, device.host, "Failed to create cast object"
                )

            # Connect with retry logic
            for attempt in range(self.config.CONNECTION_MAX_RETRIES):
                try:
                    logging.info(f"Connecting to {device.name} (attempt {attempt + 1})")
                    cast_object.wait(timeout=timeout)

                    connection_time = time.time() - start_time
                    logging.info(f"Connected to {device.name} in {connection_time:.2f}s")
                    return cast_object

                except Exception as e:
                    if "threads can only be started once" in str(e):
                        # Threading issue, need new object
                        raise ThreadingError(f"Threading error for {device.name}: {e}")

                    if attempt < self.config.CONNECTION_MAX_RETRIES - 1:
                        time.sleep(self.config.CONNECTION_RETRY_DELAY)
                    else:
                        raise MaxRetriesExceededError(
                            device.name, self.config.CONNECTION_MAX_RETRIES
                        )

        except Exception as e:
            connection_time = time.time() - start_time
            logging.error(f"Failed to connect to {device.name} after {connection_time:.2f}s: {e}")
            raise

    def _create_cast_object(self, device: CastDevice) -> Optional[Any]:
        """Create a Chromecast object from CastDevice"""
        try:
            # If we already have a cast object, use it
            if device.cast_object:
                logging.debug(f"Using cached cast object for {device.name}")
                return device.cast_object

            # Create from device info
            logging.debug(f"Creating cast object for {device.name}")

            # Generate UUID if needed
            uuid_obj = device.uuid
            if isinstance(uuid_obj, str):
                try:
                    uuid_obj = UUID(uuid_obj)
                except Exception:
                    # Generate fallback UUID
                    uuid_str = hashlib.md5(
                        f"{device.host}:{device.port}".encode()
                    ).hexdigest()
                    uuid_obj = UUID(
                        f"{uuid_str[:8]}-{uuid_str[8:12]}-{uuid_str[12:16]}-"
                        f"{uuid_str[16:20]}-{uuid_str[20:32]}"
                    )

            # Create using host info
            host_tuple = (
                device.host,
                int(device.port),
                uuid_obj,
                device.name,
                device.model_name
            )

            return pychromecast.get_chromecast_from_host(
                host_tuple,
                timeout=self.config.CONNECTION_TIMEOUT_SECONDS
            )

        except Exception as e:
            logging.error(f"Error creating cast object for {device.name}: {e}")
            return None

    def _is_device_reachable(self, device: CastDevice) -> bool:
        """Check if device is reachable on the network"""
        try:
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            sock.settimeout(self.config.SOCKET_TIMEOUT_SECONDS)
            result = sock.connect_ex((device.host, device.port))
            sock.close()
            return result == 0
        except Exception as e:
            logging.debug(f"Device reachability check failed for {device.name}: {e}")
            return False

    def _validate_connection(self, conn: DeviceConnection) -> bool:
        """Validate if a connection is still active"""
        try:
            # Check if connection was used recently
            if (datetime.now() - conn.last_used).total_seconds() > self.config.DEVICE_CACHE_TTL_SECONDS:
                logging.debug(f"Connection to {conn.device.name} expired")
                return False

            # Quick network check
            if not self._is_device_reachable(conn.device):
                logging.debug(f"Device {conn.device.name} no longer reachable")
                return False

            # Try to get status
            if conn.cast_object:
                try:
                    conn.cast_object.media_controller.update_status()
                    return True
                except Exception as e:
                    logging.debug(f"Status check failed for {conn.device.name}: {e}")
                    return False

            return False

        except Exception as e:
            logging.debug(f"Connection validation failed for {conn.device.name}: {e}")
            return False

    def check_availability(self, device: CastDevice) -> AvailabilityCheckResponse:
        """
        Check if a device is available.

        Args:
            device: Device to check

        Returns:
            AvailabilityCheckResponse
        """
        try:
            available = self._is_device_reachable(device)
            device.update_availability(available)

            return {
                'success': True,
                'timestamp': datetime.now().isoformat(),
                'error': None,
                'available': available,
                'device_name': device.name,
                'host': device.host,
                'port': device.port,
                'response_code': 0 if available else 1
            }

        except Exception as e:
            return {
                'success': False,
                'timestamp': datetime.now().isoformat(),
                'error': str(e),
                'available': False,
                'device_name': device.name,
                'host': device.host,
                'port': device.port,
                'response_code': None
            }

    @contextmanager
    def device_connection(self, device: CastDevice):
        """
        Context manager for device connections.

        Usage:
            with connection_pool.device_connection(device) as cast:
                # Use cast object
                pass
        """
        cast_object = None
        try:
            cast_object = self.get_connection(device)
            yield cast_object
        finally:
            # Connection remains in pool for reuse
            if cast_object and device.uuid in self.connections:
                with self.lock:
                    conn = self.connections[device.uuid]
                    conn.last_used = datetime.now()

    def close_connection(self, device_uuid: str) -> None:
        """Close and remove a connection from the pool"""
        with self.lock:
            if device_uuid in self.connections:
                conn = self.connections.pop(device_uuid)
                logging.info(f"Closed connection to {conn.device.name}")

    def _start_health_checks(self) -> None:
        """Start background health check thread"""
        def health_check_loop():
            while not self.stop_health_checks.is_set():
                try:
                    self._perform_health_checks()
                except Exception as e:
                    logging.error(f"Health check error: {e}")

                self.stop_health_checks.wait(
                    self.config.HEALTH_CHECK_INTERVAL_SECONDS
                )

        self.health_check_thread = threading.Thread(
            target=health_check_loop,
            daemon=True,
            name="ChromecastHealthCheck"
        )
        self.health_check_thread.start()

    def _perform_health_checks(self) -> None:
        """Perform health checks on all connections"""
        with self.lock:
            connections_to_check = list(self.connections.items())

        for device_uuid, conn in connections_to_check:
            try:
                # Validate connection
                if not self._validate_connection(conn):
                    logging.info(f"Connection to {conn.device.name} failed validation")
                    self.close_connection(device_uuid)
                else:
                    conn.last_validated = datetime.now()

            except Exception as e:
                logging.warning(f"Health check failed for {conn.device.name}: {e}")

    def _update_stats(self, device_name: str, success: bool, connection_time: float) -> None:
        """Update connection statistics"""
        with self.lock:
            if device_name not in self.stats:
                self.stats[device_name] = ConnectionStats(device_name)

            self.stats[device_name].add_connection_result(success, connection_time)

    def get_stats(self, device_name: Optional[str] = None) -> Dict[str, Any]:
        """Get connection statistics"""
        with self.lock:
            if device_name:
                if device_name in self.stats:
                    stats = self.stats[device_name]
                    return {
                        'device_name': stats.device_name,
                        'total_attempts': stats.total_attempts,
                        'successful_connections': stats.successful_connections,
                        'failed_connections': stats.failed_connections,
                        'success_rate': stats.get_success_rate(),
                        'average_connection_time': stats.average_connection_time,
                        'last_success': stats.last_success.isoformat() if stats.last_success else None,
                        'last_failure': stats.last_failure.isoformat() if stats.last_failure else None
                    }
                return {'error': f"No stats for device {device_name}"}

            # Return all stats
            return {
                name: {
                    'device_name': stats.device_name,
                    'total_attempts': stats.total_attempts,
                    'success_rate': stats.get_success_rate(),
                    'average_connection_time': stats.average_connection_time
                }
                for name, stats in self.stats.items()
            }

    def get_pool_status(self) -> Dict[str, Any]:
        """Get current connection pool status"""
        with self.lock:
            active_connections = [
                {
                    'device_name': conn.device.name,
                    'device_uuid': conn.device.uuid,
                    'is_connected': conn.is_connected,
                    'connection_count': conn.connection_count,
                    'last_used': conn.last_used.isoformat(),
                    'last_validated': conn.last_validated.isoformat() if conn.last_validated else None
                }
                for conn in self.connections.values()
            ]

            return {
                'active_connections': len(self.connections),
                'connections': active_connections,
                'circuit_breakers': self.circuit_breaker_manager.get_all_status(),
                'health_check_active': self.health_check_thread and self.health_check_thread.is_alive()
            }

    def cleanup(self) -> None:
        """Clean up all resources"""
        logging.info("Cleaning up connection pool...")

        # Stop health checks
        self.stop_health_checks.set()
        if self.health_check_thread:
            self.health_check_thread.join(timeout=5)

        # Close all connections
        with self.lock:
            for conn in self.connections.values():
                try:
                    # Attempt graceful shutdown
                    if conn.cast_object:
                        conn.cast_object.disconnect()
                except Exception as e:
                    logging.debug(f"Error disconnecting {conn.device.name}: {e}")

            self.connections.clear()
            self.stats.clear()

        # Clean up circuit breakers
        self.circuit_breaker_manager.cleanup()

        logging.info("Connection pool cleanup complete")