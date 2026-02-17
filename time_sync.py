#!/usr/bin/env python3
"""
Time synchronization utilities for Automated Azan
Provides methods to check and sync system time without requiring sudo
All methods return JSON responses for API compatibility.
"""

import time
import socket
import struct
import logging
import requests
from datetime import datetime, timezone


class TimeSynchronizer:
    """
    A class to handle time synchronization using various methods.
    All methods return JSON responses for API compatibility.
    """

    def __init__(self):
        self.ntp_servers = [
            'pool.ntp.org',
            '0.pool.ntp.org',
            '1.pool.ntp.org',
            '2.pool.ntp.org',
            'time.google.com',
            'time.cloudflare.com'
        ]

        # HTTP time APIs as fallback
        self.time_apis = [
            'http://worldtimeapi.org/api/timezone/Europe/Dublin',
            'https://timeapi.io/api/Time/current/zone?timeZone=Europe/Dublin'
        ]

    def get_ntp_time(self, server='pool.ntp.org', timeout=10):
        """
        Get time from NTP server using SNTP protocol.

        Args:
            server (str): NTP server hostname
            timeout (int): Connection timeout in seconds

        Returns:
            dict: JSON response with NTP time or error
        """
        try:
            # SNTP packet format
            packet = b'\x1b' + 47 * b'\0'

            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            sock.settimeout(timeout)

            start_time = time.time()
            sock.sendto(packet, (server, 123))

            data = sock.recv(48)
            response_time = time.time() - start_time
            sock.close()

            # Extract timestamp from NTP response
            timestamp = struct.unpack('!12I', data)[10]
            timestamp -= 2208988800  # NTP epoch adjustment

            ntp_datetime = datetime.fromtimestamp(timestamp, tz=timezone.utc)

            return {
                "success": True,
                "server": server,
                "ntp_time": ntp_datetime.isoformat(),
                "response_time_ms": round(response_time * 1000, 2),
                "timestamp": timestamp,
                "source": "ntp",
                "message": f"Successfully retrieved time from NTP server {server}",
                "query_timestamp": datetime.now(timezone.utc).isoformat()
            }

        except Exception as e:
            logging.debug(f"Failed to get time from NTP server {server}: {e}")
            return {
                "success": False,
                "server": server,
                "error": str(e),
                "error_type": "ntp_query_failed",
                "source": "ntp",
                "query_timestamp": datetime.now(timezone.utc).isoformat()
            }

    def get_http_time(self):
        """
        Get time from HTTP API as fallback.

        Returns:
            dict: JSON response with HTTP time or error
        """
        api_attempts = []

        for api_url in self.time_apis:
            start_time = time.time()
            try:
                response = requests.get(api_url, timeout=5)
                response_time = time.time() - start_time

                if response.status_code == 200:
                    data = response.json()

                    # Handle different API formats
                    if 'datetime' in data:  # worldtimeapi.org
                        time_str = data['datetime']
                        parsed_time = datetime.fromisoformat(time_str.replace('Z', '+00:00'))
                    elif 'dateTime' in data:  # timeapi.io
                        time_str = data['dateTime']
                        parsed_time = datetime.fromisoformat(time_str)
                    else:
                        api_attempts.append({
                            "api_url": api_url,
                            "success": False,
                            "error": "Unsupported API response format",
                            "response_time_ms": round(response_time * 1000, 2)
                        })
                        continue

                    return {
                        "success": True,
                        "api_url": api_url,
                        "http_time": parsed_time.isoformat(),
                        "response_time_ms": round(response_time * 1000, 2),
                        "raw_response": data,
                        "source": "http_api",
                        "message": f"Successfully retrieved time from HTTP API {api_url}",
                        "query_timestamp": datetime.now(timezone.utc).isoformat()
                    }
                else:
                    api_attempts.append({
                        "api_url": api_url,
                        "success": False,
                        "error": f"HTTP {response.status_code}: {response.text}",
                        "response_time_ms": round(response_time * 1000, 2)
                    })

            except Exception as e:
                response_time = time.time() - start_time
                api_attempts.append({
                    "api_url": api_url,
                    "success": False,
                    "error": str(e),
                    "response_time_ms": round(response_time * 1000, 2)
                })
                logging.debug(f"Failed to get time from HTTP API {api_url}: {e}")

        return {
            "success": False,
            "error": "All HTTP time APIs failed",
            "api_attempts": api_attempts,
            "source": "http_api",
            "query_timestamp": datetime.now(timezone.utc).isoformat()
        }

    def get_accurate_time(self):
        """
        Get accurate time using best available method.

        Returns:
            dict: JSON response with accurate time and source
        """
        methods_attempted = []

        # Try NTP servers first
        for server in self.ntp_servers:
            ntp_result = self.get_ntp_time(server)
            methods_attempted.append({
                "method": "ntp",
                "server": server,
                "success": ntp_result.get("success", False),
                "response_time_ms": ntp_result.get("response_time_ms"),
                "error": ntp_result.get("error") if not ntp_result.get("success") else None
            })

            if ntp_result.get("success", False):
                return {
                    "success": True,
                    "accurate_time": ntp_result["ntp_time"],
                    "source": f"NTP ({server})",
                    "source_type": "ntp",
                    "response_time_ms": ntp_result.get("response_time_ms"),
                    "methods_attempted": methods_attempted,
                    "message": f"Accurate time retrieved from NTP server {server}",
                    "timestamp": datetime.now(timezone.utc).isoformat()
                }

        # Fallback to HTTP APIs
        http_result = self.get_http_time()
        methods_attempted.append({
            "method": "http_api",
            "success": http_result.get("success", False),
            "api_attempts": http_result.get("api_attempts", []),
            "error": http_result.get("error") if not http_result.get("success") else None
        })

        if http_result.get("success", False):
            return {
                "success": True,
                "accurate_time": http_result["http_time"],
                "source": f"HTTP API ({http_result['api_url']})",
                "source_type": "http_api",
                "response_time_ms": http_result.get("response_time_ms"),
                "methods_attempted": methods_attempted,
                "message": f"Accurate time retrieved from HTTP API",
                "timestamp": datetime.now(timezone.utc).isoformat()
            }

        # Last resort: system time
        system_time = datetime.now(timezone.utc)
        methods_attempted.append({
            "method": "system",
            "success": True,
            "note": "Fallback to system time"
        })

        return {
            "success": True,
            "accurate_time": system_time.isoformat(),
            "source": "System (fallback)",
            "source_type": "system",
            "methods_attempted": methods_attempted,
            "message": "Using system time as fallback - accuracy not guaranteed",
            "warning": "Could not contact external time sources",
            "timestamp": system_time.isoformat()
        }

    def check_time_drift(self, threshold_seconds=60):
        """
        Check if system time differs from accurate time by more than threshold.

        Args:
            threshold_seconds (int): Maximum allowed drift in seconds

        Returns:
            dict: JSON response with drift analysis
        """
        try:
            # Get accurate time
            accurate_time_result = self.get_accurate_time()
            if not accurate_time_result.get("success", False):
                return {
                    "success": False,
                    "error": "Failed to get accurate time for comparison",
                    "accurate_time_result": accurate_time_result,
                    "timestamp": datetime.now(timezone.utc).isoformat()
                }

            accurate_time = datetime.fromisoformat(accurate_time_result["accurate_time"])
            system_time = datetime.now(timezone.utc)

            # Calculate drift
            drift = (system_time - accurate_time).total_seconds()
            is_synchronized = abs(drift) <= threshold_seconds

            # Determine drift status
            if abs(drift) <= 1:
                drift_status = "excellent"
            elif abs(drift) <= 10:
                drift_status = "good"
            elif abs(drift) <= threshold_seconds:
                drift_status = "acceptable"
            else:
                drift_status = "poor"

            logging.info(f"Time check - System: {system_time.strftime('%Y-%m-%d %H:%M:%S UTC')}")
            logging.info(f"Time check - Accurate ({accurate_time_result['source']}): {accurate_time.strftime('%Y-%m-%d %H:%M:%S UTC')}")
            logging.info(f"Time drift: {drift:.2f} seconds")

            if is_synchronized:
                logging.info("✅ System time is properly synchronized")
            else:
                logging.warning(f"⚠️ System time drift detected: {drift:.2f}s (threshold: {threshold_seconds}s)")

            return {
                "success": True,
                "drift_seconds": round(drift, 2),
                "drift_abs_seconds": round(abs(drift), 2),
                "is_synchronized": is_synchronized,
                "drift_status": drift_status,
                "threshold_seconds": threshold_seconds,
                "system_time": system_time.isoformat(),
                "accurate_time": accurate_time.isoformat(),
                "time_source": accurate_time_result["source"],
                "source_type": accurate_time_result["source_type"],
                "accurate_time_result": accurate_time_result,
                "message": f"System time is {'synchronized' if is_synchronized else 'not synchronized'}",
                "recommendation": "System time is acceptable" if is_synchronized else f"Consider synchronizing system time (drift: {drift:.2f}s)",
                "timestamp": datetime.now(timezone.utc).isoformat()
            }

        except Exception as e:
            logging.error(f"Error checking time drift: {e}")
            return {
                "success": False,
                "error": str(e),
                "drift_seconds": 0,
                "is_synchronized": True,  # Default to synchronized on error to avoid false alarms
                "fallback_used": True,
                "system_time": datetime.now(timezone.utc).isoformat(),
                "message": "Error during time drift check, assuming synchronized",
                "timestamp": datetime.now(timezone.utc).isoformat()
            }

    def get_system_time_info(self):
        """
        Get system time information (equivalent to timedatectl status).

        Returns:
            dict: JSON response with system time info
        """
        try:
            import subprocess
            result = subprocess.run(['timedatectl', 'status'],
                                  capture_output=True, text=True, timeout=10)

            if result.returncode == 0:
                # Parse timedatectl output for key information
                output_lines = result.stdout.strip().split('\n')
                parsed_info = {}

                for line in output_lines:
                    if ':' in line:
                        key, value = line.split(':', 1)
                        parsed_info[key.strip()] = value.strip()

                return {
                    "success": True,
                    "status": "available",
                    "raw_output": result.stdout.strip(),
                    "parsed_info": parsed_info,
                    "command": "timedatectl status",
                    "message": "System time information retrieved successfully",
                    "timestamp": datetime.now(timezone.utc).isoformat()
                }
            else:
                return {
                    "success": False,
                    "status": "command_failed",
                    "error": result.stderr.strip(),
                    "return_code": result.returncode,
                    "command": "timedatectl status",
                    "timestamp": datetime.now(timezone.utc).isoformat()
                }

        except FileNotFoundError:
            # timedatectl not available, try other methods
            return {
                "success": False,
                "status": "not_available",
                "error": "timedatectl command not found",
                "fallback_info": {
                    "system_time": datetime.now().isoformat(),
                    "utc_time": datetime.now(timezone.utc).isoformat(),
                    "timezone": str(datetime.now().astimezone().tzinfo)
                },
                "message": "timedatectl not available, using basic time info",
                "timestamp": datetime.now(timezone.utc).isoformat()
            }
        except subprocess.TimeoutExpired:
            return {
                "success": False,
                "status": "timeout",
                "error": "timedatectl command timed out",
                "timeout_seconds": 10,
                "timestamp": datetime.now(timezone.utc).isoformat()
            }
        except Exception as e:
            return {
                "success": False,
                "status": "error",
                "error": str(e),
                "timestamp": datetime.now(timezone.utc).isoformat()
            }

    def get_all_ntp_servers_status(self):
        """
        Get status of all configured NTP servers.

        Returns:
            dict: JSON response with all server statuses
        """
        server_results = []
        successful_servers = []
        failed_servers = []

        for server in self.ntp_servers:
            result = self.get_ntp_time(server, timeout=5)  # Shorter timeout for bulk check
            server_results.append({
                "server": server,
                "success": result.get("success", False),
                "response_time_ms": result.get("response_time_ms"),
                "ntp_time": result.get("ntp_time"),
                "error": result.get("error") if not result.get("success") else None
            })

            if result.get("success", False):
                successful_servers.append(server)
            else:
                failed_servers.append(server)

        return {
            "success": True,
            "servers_tested": len(self.ntp_servers),
            "successful_servers": len(successful_servers),
            "failed_servers": len(failed_servers),
            "server_results": server_results,
            "successful_server_list": successful_servers,
            "failed_server_list": failed_servers,
            "message": f"Tested {len(self.ntp_servers)} NTP servers, {len(successful_servers)} successful",
            "timestamp": datetime.now(timezone.utc).isoformat()
        }

    def sync_status_summary(self):
        """
        Get a comprehensive synchronization status summary.

        Returns:
            dict: JSON response with complete sync status
        """
        try:
            # Get all information
            drift_result = self.check_time_drift()
            system_info = self.get_system_time_info()
            ntp_status = self.get_all_ntp_servers_status()

            # Determine overall status
            if drift_result.get("is_synchronized", False):
                if drift_result.get("drift_abs_seconds", 0) <= 1:
                    overall_status = "excellent"
                elif drift_result.get("drift_abs_seconds", 0) <= 10:
                    overall_status = "good"
                else:
                    overall_status = "acceptable"
            else:
                overall_status = "poor"

            return {
                "success": True,
                "overall_status": overall_status,
                "time_drift": drift_result,
                "system_info": system_info,
                "ntp_servers": ntp_status,
                "summary": {
                    "synchronized": drift_result.get("is_synchronized", False),
                    "drift_seconds": drift_result.get("drift_seconds", 0),
                    "time_source": drift_result.get("time_source", "unknown"),
                    "ntp_servers_available": ntp_status.get("successful_servers", 0),
                    "system_info_available": system_info.get("success", False)
                },
                "timestamp": datetime.now(timezone.utc).isoformat()
            }

        except Exception as e:
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now(timezone.utc).isoformat()
            }


def update_ntp_time():
    """
    Improved time synchronization function for main.py.

    Returns:
        dict: JSON response with synchronization status
    """
    try:
        synchronizer = TimeSynchronizer()

        # Get system time info if available
        time_info = synchronizer.get_system_time_info()
        if time_info.get('success', False):
            logging.info("System time status:\n" + time_info.get('raw_output', '').strip())

        # Check time drift
        drift_result = synchronizer.check_time_drift()

        if not drift_result.get('is_synchronized', True):
            accurate_time = drift_result.get('accurate_time', 'unknown')
            time_source = drift_result.get('time_source', 'unknown')
            logging.warning(f"Consider synchronizing system time. Accurate time from {time_source}: {accurate_time}")

        return {
            "success": True,
            "synchronized": drift_result.get('is_synchronized', True),
            "drift_seconds": drift_result.get('drift_seconds', 0),
            "time_source": drift_result.get('time_source', 'unknown'),
            "system_info": time_info,
            "drift_check": drift_result,
            "message": f"Time sync check completed - {'synchronized' if drift_result.get('is_synchronized', True) else 'not synchronized'}",
            "timestamp": datetime.now(timezone.utc).isoformat()
        }

    except Exception as e:
        logging.error(f"Error during time synchronization check: {e}")
        return {
            "success": False,
            "error": str(e),
            "synchronized": True,  # Default to true to avoid blocking operation
            "fallback_used": True,
            "message": "Error during time sync check, assuming synchronized",
            "timestamp": datetime.now(timezone.utc).isoformat()
        }


# Example usage and testing
if __name__ == "__main__":
    import json

    print("=== Time Synchronizer JSON API Demo ===\n")

    # Setup logging
    logging.basicConfig(level=logging.INFO, format='%(asctime)s [%(levelname)s]: %(message)s')

    synchronizer = TimeSynchronizer()

    # Test individual methods
    print("1. Checking time drift:")
    drift_result = synchronizer.check_time_drift()
    print(json.dumps(drift_result, indent=2))

    print("\n2. Getting system time info:")
    time_info = synchronizer.get_system_time_info()
    print(json.dumps(time_info, indent=2))

    print("\n3. Testing NTP servers:")
    ntp_status = synchronizer.get_all_ntp_servers_status()
    print(json.dumps(ntp_status, indent=2))

    print("\n4. Complete sync status summary:")
    summary = synchronizer.sync_status_summary()
    print(json.dumps(summary, indent=2))

    print("\n5. Main update function:")
    update_result = update_ntp_time()
    print(json.dumps(update_result, indent=2))

    print("\n=== Time Synchronizer JSON API Demo Complete ===")