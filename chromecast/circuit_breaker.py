"""
Circuit Breaker Pattern Implementation

This module implements the circuit breaker pattern to prevent repeated
failed connection attempts to Chromecast devices.
"""

import time
import threading
from typing import Optional, Callable, Any, Dict
from datetime import datetime, timedelta
from enum import Enum
import logging

# Import from parent directory
import sys
import os
sys.path.append(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from chromecast_config import ChromecastConfig, get_config
from chromecast_exceptions import CircuitBreakerOpenError


class CircuitState(str, Enum):
    """Circuit breaker states"""
    CLOSED = "closed"  # Normal operation
    OPEN = "open"      # Circuit broken, rejecting calls
    HALF_OPEN = "half_open"  # Testing if service recovered


class CircuitBreaker:
    """
    Circuit breaker to prevent cascading failures.

    The circuit breaker has three states:
    - CLOSED: Normal operation, requests pass through
    - OPEN: Too many failures, requests are rejected
    - HALF_OPEN: Testing recovery, limited requests allowed
    """

    def __init__(
        self,
        name: str,
        failure_threshold: Optional[int] = None,
        recovery_timeout: Optional[int] = None,
        expected_exception: type = Exception,
        config: Optional[ChromecastConfig] = None
    ):
        """
        Initialize circuit breaker.

        Args:
            name: Identifier for this circuit breaker
            failure_threshold: Number of failures before opening circuit
            recovery_timeout: Seconds before attempting recovery
            expected_exception: Exception type to count as failure
            config: Configuration object
        """
        self.config = config or get_config()
        self.name = name
        self.failure_threshold = failure_threshold or self.config.CIRCUIT_BREAKER_FAILURE_THRESHOLD
        self.recovery_timeout = recovery_timeout or self.config.CIRCUIT_BREAKER_RECOVERY_TIMEOUT
        self.expected_exception = expected_exception

        self.state = CircuitState.CLOSED
        self.failure_count = 0
        self.last_failure_time: Optional[datetime] = None
        self.next_attempt_time: Optional[datetime] = None
        self.successful_calls = 0
        self.total_calls = 0
        self.lock = threading.Lock()

        # Statistics
        self.state_changes: list = []
        self.failure_reasons: Dict[str, int] = {}

    def __call__(self, func: Callable) -> Callable:
        """Decorator for wrapping functions with circuit breaker"""
        def wrapper(*args, **kwargs) -> Any:
            return self.call(func, *args, **kwargs)
        return wrapper

    def call(self, func: Callable, *args, **kwargs) -> Any:
        """
        Execute function through circuit breaker.

        Args:
            func: Function to execute
            *args: Function arguments
            **kwargs: Function keyword arguments

        Returns:
            Function result

        Raises:
            CircuitBreakerOpenError: If circuit is open
            Original exception: If function fails
        """
        with self.lock:
            self.total_calls += 1

            # Check circuit state
            if self.state == CircuitState.OPEN:
                if self._should_attempt_reset():
                    self._transition_to_half_open()
                else:
                    raise CircuitBreakerOpenError(
                        self.name,
                        self.next_attempt_time or datetime.now()
                    )

        # Execute the function
        try:
            result = func(*args, **kwargs)
            self._on_success()
            return result
        except self.expected_exception as e:
            self._on_failure(str(e))
            raise

    def _should_attempt_reset(self) -> bool:
        """Check if we should attempt to reset the circuit"""
        if self.next_attempt_time is None:
            return False
        return datetime.now() >= self.next_attempt_time

    def _on_success(self) -> None:
        """Handle successful call"""
        with self.lock:
            self.successful_calls += 1

            if self.state == CircuitState.HALF_OPEN:
                # Success in half-open state, close the circuit
                self._transition_to_closed()
            else:
                # Reset failure count on success
                self.failure_count = 0

    def _on_failure(self, reason: str) -> None:
        """Handle failed call"""
        with self.lock:
            self.failure_count += 1
            self.last_failure_time = datetime.now()

            # Track failure reason
            if reason not in self.failure_reasons:
                self.failure_reasons[reason] = 0
            self.failure_reasons[reason] += 1

            if self.state == CircuitState.HALF_OPEN:
                # Failure in half-open state, reopen the circuit
                self._transition_to_open()
            elif self.state == CircuitState.CLOSED:
                if self.failure_count >= self.failure_threshold:
                    # Too many failures, open the circuit
                    self._transition_to_open()

    def _transition_to_open(self) -> None:
        """Transition to OPEN state"""
        self.state = CircuitState.OPEN
        self.next_attempt_time = datetime.now() + timedelta(seconds=self.recovery_timeout)
        self.state_changes.append({
            'from': CircuitState.CLOSED,
            'to': CircuitState.OPEN,
            'timestamp': datetime.now().isoformat(),
            'failure_count': self.failure_count
        })
        logging.warning(
            f"Circuit breaker '{self.name}' opened due to {self.failure_count} failures. "
            f"Will retry at {self.next_attempt_time.isoformat()}"
        )

    def _transition_to_half_open(self) -> None:
        """Transition to HALF_OPEN state"""
        previous_state = self.state
        self.state = CircuitState.HALF_OPEN
        self.state_changes.append({
            'from': previous_state,
            'to': CircuitState.HALF_OPEN,
            'timestamp': datetime.now().isoformat()
        })
        logging.info(f"Circuit breaker '{self.name}' entering half-open state for testing")

    def _transition_to_closed(self) -> None:
        """Transition to CLOSED state"""
        previous_state = self.state
        self.state = CircuitState.CLOSED
        self.failure_count = 0
        self.next_attempt_time = None
        self.state_changes.append({
            'from': previous_state,
            'to': CircuitState.CLOSED,
            'timestamp': datetime.now().isoformat()
        })
        logging.info(f"Circuit breaker '{self.name}' closed, normal operation resumed")

    def reset(self) -> None:
        """Manually reset the circuit breaker"""
        with self.lock:
            self.state = CircuitState.CLOSED
            self.failure_count = 0
            self.next_attempt_time = None
            self.state_changes.append({
                'from': self.state,
                'to': CircuitState.CLOSED,
                'timestamp': datetime.now().isoformat(),
                'manual_reset': True
            })
            logging.info(f"Circuit breaker '{self.name}' manually reset")

    def get_status(self) -> Dict[str, Any]:
        """Get current circuit breaker status"""
        with self.lock:
            return {
                'name': self.name,
                'state': self.state,
                'failure_count': self.failure_count,
                'failure_threshold': self.failure_threshold,
                'successful_calls': self.successful_calls,
                'total_calls': self.total_calls,
                'success_rate': self.successful_calls / self.total_calls if self.total_calls > 0 else 0,
                'last_failure_time': self.last_failure_time.isoformat() if self.last_failure_time else None,
                'next_attempt_time': self.next_attempt_time.isoformat() if self.next_attempt_time else None,
                'recovery_timeout': self.recovery_timeout,
                'failure_reasons': dict(self.failure_reasons),
                'recent_state_changes': self.state_changes[-5:]  # Last 5 state changes
            }


class CircuitBreakerManager:
    """Manages multiple circuit breakers for different devices"""

    def __init__(self, config: Optional[ChromecastConfig] = None):
        self.config = config or get_config()
        self.breakers: Dict[str, CircuitBreaker] = {}
        self.lock = threading.Lock()

    def get_breaker(self, device_name: str) -> CircuitBreaker:
        """
        Get or create a circuit breaker for a device.

        Args:
            device_name: Name of the device

        Returns:
            CircuitBreaker for the device
        """
        with self.lock:
            if device_name not in self.breakers:
                self.breakers[device_name] = CircuitBreaker(
                    name=f"device_{device_name}",
                    config=self.config
                )
            return self.breakers[device_name]

    def reset_breaker(self, device_name: str) -> None:
        """Reset circuit breaker for a specific device"""
        if device_name in self.breakers:
            self.breakers[device_name].reset()

    def reset_all(self) -> None:
        """Reset all circuit breakers"""
        with self.lock:
            for breaker in self.breakers.values():
                breaker.reset()

    def get_all_status(self) -> Dict[str, Dict[str, Any]]:
        """Get status of all circuit breakers"""
        with self.lock:
            return {
                name: breaker.get_status()
                for name, breaker in self.breakers.items()
            }

    def cleanup(self) -> None:
        """Clean up all circuit breakers"""
        with self.lock:
            self.breakers.clear()