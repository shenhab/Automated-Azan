#!/usr/bin/env python3
"""
Time synchronization utilities for Automated Azan
Provides methods to check and sync system time without requiring sudo
"""

import time
import socket
import struct
import logging
import requests
from datetime import datetime, timezone

class TimeSynchronizer:
    """
    A class to handle time synchronization using various methods
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
        Get time from NTP server using SNTP protocol
        Returns: datetime object or None if failed
        """
        try:
            # SNTP packet format
            packet = b'\x1b' + 47 * b'\0'
            
            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            sock.settimeout(timeout)
            sock.sendto(packet, (server, 123))
            
            data = sock.recv(48)
            sock.close()
            
            # Extract timestamp from NTP response
            timestamp = struct.unpack('!12I', data)[10]
            timestamp -= 2208988800  # NTP epoch adjustment
            
            return datetime.fromtimestamp(timestamp, tz=timezone.utc)
            
        except Exception as e:
            logging.debug(f"Failed to get time from NTP server {server}: {e}")
            return None
    
    def get_http_time(self):
        """
        Get time from HTTP API as fallback
        Returns: datetime object or None if failed
        """
        for api_url in self.time_apis:
            try:
                response = requests.get(api_url, timeout=5)
                if response.status_code == 200:
                    data = response.json()
                    
                    # Handle different API formats
                    if 'datetime' in data:  # worldtimeapi.org
                        time_str = data['datetime']
                        return datetime.fromisoformat(time_str.replace('Z', '+00:00'))
                    elif 'dateTime' in data:  # timeapi.io
                        time_str = data['dateTime']
                        return datetime.fromisoformat(time_str)
                        
            except Exception as e:
                logging.debug(f"Failed to get time from HTTP API {api_url}: {e}")
                continue
                
        return None
    
    def get_accurate_time(self):
        """
        Get accurate time using best available method
        Returns: (datetime, source) tuple
        """
        # Try NTP servers first
        for server in self.ntp_servers:
            ntp_time = self.get_ntp_time(server)
            if ntp_time:
                return ntp_time, f"NTP ({server})"
        
        # Fallback to HTTP APIs
        http_time = self.get_http_time()
        if http_time:
            return http_time, "HTTP API"
        
        # Last resort: system time
        return datetime.now(timezone.utc), "System"
    
    def check_time_drift(self, threshold_seconds=60):
        """
        Check if system time differs from accurate time by more than threshold
        Returns: (drift_seconds, is_synchronized, accurate_time, source)
        """
        try:
            accurate_time, source = self.get_accurate_time()
            system_time = datetime.now(timezone.utc)
            
            # Calculate drift
            drift = (system_time - accurate_time).total_seconds()
            is_synchronized = abs(drift) <= threshold_seconds
            
            logging.info(f"Time check - System: {system_time.strftime('%Y-%m-%d %H:%M:%S UTC')}")
            logging.info(f"Time check - Accurate ({source}): {accurate_time.strftime('%Y-%m-%d %H:%M:%S UTC')}")
            logging.info(f"Time drift: {drift:.2f} seconds")
            
            if is_synchronized:
                logging.info("✅ System time is properly synchronized")
            else:
                logging.warning(f"⚠️ System time drift detected: {drift:.2f}s (threshold: {threshold_seconds}s)")
            
            return drift, is_synchronized, accurate_time, source
            
        except Exception as e:
            logging.error(f"Error checking time drift: {e}")
            return 0, True, datetime.now(timezone.utc), "Error"
    
    def get_system_time_info(self):
        """
        Get system time information (equivalent to timedatectl status)
        Returns: dict with time info
        """
        try:
            import subprocess
            result = subprocess.run(['timedatectl', 'status'], 
                                  capture_output=True, text=True, timeout=10)
            
            if result.returncode == 0:
                return {'status': 'success', 'output': result.stdout}
            else:
                return {'status': 'error', 'output': result.stderr}
                
        except FileNotFoundError:
            # timedatectl not available, try other methods
            return {'status': 'not_available', 'output': 'timedatectl not found'}
        except Exception as e:
            return {'status': 'error', 'output': str(e)}

def update_ntp_time():
    """
    Improved time synchronization function for main.py
    """
    synchronizer = TimeSynchronizer()
    
    # Get system time info if available
    time_info = synchronizer.get_system_time_info()
    if time_info['status'] == 'success':
        logging.info("System time status:\n" + time_info['output'].strip())
    
    # Check time drift
    drift, is_synchronized, accurate_time, source = synchronizer.check_time_drift()
    
    if not is_synchronized:
        logging.warning(f"Consider synchronizing system time. Accurate time from {source}: {accurate_time}")
        
    return is_synchronized


if __name__ == "__main__":
    # Test the time synchronizer
    logging.basicConfig(level=logging.INFO, format='%(asctime)s [%(levelname)s]: %(message)s')
    
    synchronizer = TimeSynchronizer()
    drift, is_synchronized, accurate_time, source = synchronizer.check_time_drift()
    
    print(f"System time synchronized: {is_synchronized}")
    print(f"Drift: {drift:.2f} seconds")
    print(f"Accurate time from {source}: {accurate_time}")
