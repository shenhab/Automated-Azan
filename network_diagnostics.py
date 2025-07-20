#!/usr/bin/env python3
"""
Network diagnostics script for Automated Azan Chromecast connectivity issues.
This script helps diagnose and monitor Chromecast network connectivity problems.
"""

import socket
import time
import logging
import threading
from datetime import datetime
import pychromecast
from pychromecast.discovery import CastBrowser, SimpleCastListener

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

def check_network_connectivity(host, port, timeout=5):
    """Check if a host:port is reachable."""
    try:
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(timeout)
        result = sock.connect_ex((host, port))
        sock.close()
        return result == 0
    except Exception as e:
        logging.error(f"Network check error for {host}:{port}: {e}")
        return False

def discover_chromecasts():
    """Discover all available Chromecast devices using CastBrowser."""
    devices = {}
    discovery_complete = threading.Event()
    
    def add_cast(uuid, service):
        """Callback when a new cast device is discovered."""
        try:
            device_info = {
                'name': service.friendly_name,
                'host': service.host,
                'port': service.port,
                'model': service.model_name,
                'manufacturer': service.manufacturer
            }
            devices[uuid] = device_info
            logging.info(f"Found: {device_info['name']} ({device_info['model']}) at {device_info['host']}:{device_info['port']}")
            
            # Signal discovery completion if we found some devices
            if len(devices) >= 1:
                discovery_complete.set()
                
        except Exception as e:
            logging.error(f"Error adding cast device {uuid}: {e}")
            
    def remove_cast(uuid, service):
        """Callback when a cast device is removed."""
        if uuid in devices:
            device_info = devices.pop(uuid)
            logging.debug(f"Removed cast device: {device_info['name']}")
    
    try:
        logging.info("Discovering Chromecast devices...")
        
        # Create a cast listener
        listener = SimpleCastListener(add_cast, remove_cast)
        
        # Create and start browser
        browser = CastBrowser(listener, None, None)
        browser.start_discovery()
        
        # Wait for discovery to find devices (timeout after 10 seconds)
        if discovery_complete.wait(timeout=10):
            logging.info(f"Discovery completed. Found {len(devices)} devices")
        else:
            logging.info(f"Discovery timeout reached. Found {len(devices)} devices so far")
        
        # Stop browser
        browser.stop_discovery()
        
        if not devices:
            logging.warning("No Chromecast devices found")
            return []
            
        return list(devices.values())
        
    except Exception as e:
        logging.error(f"Error discovering Chromecasts: {e}")
        return []

def monitor_device_connectivity(device_name="Adahn", duration_minutes=10, check_interval=30):
    """Monitor a specific device's connectivity over time."""
    logging.info(f"Starting connectivity monitoring for '{device_name}' for {duration_minutes} minutes")
    
    end_time = datetime.now().timestamp() + (duration_minutes * 60)
    connection_log = []
    
    while datetime.now().timestamp() < end_time:
        devices = discover_chromecasts()
        target_device = None
        
        for device in devices:
            if device['name'].lower() == device_name.lower():
                target_device = device
                break
                
        if target_device:
            connectivity = check_network_connectivity(target_device['host'], target_device['port'])
            status = "CONNECTED" if connectivity else "UNREACHABLE"
            timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            
            logging.info(f"[{timestamp}] {device_name} at {target_device['host']}:{target_device['port']} - {status}")
            connection_log.append({
                'timestamp': timestamp,
                'host': target_device['host'],
                'port': target_device['port'],
                'status': status
            })
        else:
            timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            logging.warning(f"[{timestamp}] {device_name} not found during discovery")
            connection_log.append({
                'timestamp': timestamp,
                'host': 'N/A',
                'port': 'N/A',
                'status': 'NOT_FOUND'
            })
            
        time.sleep(check_interval)
    
    # Generate summary
    logging.info("\n" + "="*60)
    logging.info("CONNECTIVITY MONITORING SUMMARY")
    logging.info("="*60)
    
    total_checks = len(connection_log)
    connected_count = sum(1 for entry in connection_log if entry['status'] == 'CONNECTED')
    unreachable_count = sum(1 for entry in connection_log if entry['status'] == 'UNREACHABLE')
    not_found_count = sum(1 for entry in connection_log if entry['status'] == 'NOT_FOUND')
    
    logging.info(f"Total checks: {total_checks}")
    logging.info(f"Connected: {connected_count} ({connected_count/total_checks*100:.1f}%)")
    logging.info(f"Unreachable: {unreachable_count} ({unreachable_count/total_checks*100:.1f}%)")
    logging.info(f"Not found: {not_found_count} ({not_found_count/total_checks*100:.1f}%)")
    
    # Check for IP address changes
    unique_hosts = set(entry['host'] for entry in connection_log if entry['host'] != 'N/A')
    if len(unique_hosts) > 1:
        logging.warning(f"IP ADDRESS CHANGES DETECTED: {unique_hosts}")
        logging.warning("This indicates DHCP lease renewal or network instability")
    else:
        logging.info(f"IP address stable: {list(unique_hosts)[0] if unique_hosts else 'N/A'}")

def main():
    print("Automated Azan - Chromecast Network Diagnostics")
    print("=" * 50)
    
    while True:
        print("\nChoose an option:")
        print("1. Discover all Chromecast devices")
        print("2. Check connectivity to a specific device")
        print("3. Monitor device connectivity over time")
        print("4. Exit")
        
        choice = input("\nEnter your choice (1-4): ").strip()
        
        if choice == "1":
            print("\nDiscovering Chromecast devices...")
            devices = discover_chromecasts()
            if devices:
                print(f"\nFound {len(devices)} devices:")
                for i, device in enumerate(devices, 1):
                    print(f"{i}. {device['name']} ({device['model']}) - {device['host']}:{device['port']}")
            else:
                print("No devices found")
                
        elif choice == "2":
            device_name = input("Enter device name (default: Adahn): ").strip() or "Adahn"
            devices = discover_chromecasts()
            target_device = None
            
            for device in devices:
                if device['name'].lower() == device_name.lower():
                    target_device = device
                    break
                    
            if target_device:
                connectivity = check_network_connectivity(target_device['host'], target_device['port'])
                status = "CONNECTED" if connectivity else "UNREACHABLE"
                print(f"\n{device_name} at {target_device['host']}:{target_device['port']} - {status}")
            else:
                print(f"\nDevice '{device_name}' not found")
                
        elif choice == "3":
            device_name = input("Enter device name (default: Adahn): ").strip() or "Adahn"
            duration = input("Monitor duration in minutes (default: 10): ").strip()
            try:
                duration = int(duration) if duration else 10
            except ValueError:
                duration = 10
                
            interval = input("Check interval in seconds (default: 30): ").strip()
            try:
                interval = int(interval) if interval else 30
            except ValueError:
                interval = 30
                
            monitor_device_connectivity(device_name, duration, interval)
            
        elif choice == "4":
            print("Goodbye!")
            break
            
        else:
            print("Invalid choice. Please try again.")

if __name__ == "__main__":
    main()
