#!/usr/bin/env python3
"""
Fix CastBrowser Discovery

This script tries different CastBrowser configurations to match get_chromecasts() results.
"""

import time
import threading
import logging
import socket
from typing import Dict, List

# Enable all debug logging
logging.basicConfig(level=logging.DEBUG)

def test_get_chromecasts_internals():
    """Examine what get_chromecasts actually does internally."""
    print("🔍 Analyzing get_chromecasts() internals...")
    
    try:
        import pychromecast
        
        print("Calling get_chromecasts()...")
        chromecasts, browser = pychromecast.get_chromecasts()
        
        print(f"Browser type: {type(browser)}")
        print(f"Browser attributes: {dir(browser)}")
        
        if hasattr(browser, 'zc'):
            print(f"Zeroconf instance: {browser.zc}")
        
        # Try to access the internal services
        if hasattr(browser, 'services'):
            print(f"Services found: {len(browser.services)}")
            for service_name, service_info in list(browser.services.items())[:5]:  # Show first 5
                print(f"  Service: {service_name}")
                print(f"    Info: {service_info}")
        
        print(f"Found {len(chromecasts)} devices via get_chromecasts()")
        
        # Cleanup
        if browser:
            browser.stop_discovery()
        
        return browser
        
    except Exception as e:
        print(f"❌ Error analyzing get_chromecasts(): {e}")
        return None

def test_manual_castbrowser_with_zeroconf():
    """Test CastBrowser with explicit zeroconf setup."""
    print("\n🔧 Testing CastBrowser with manual zeroconf setup...")
    
    try:
        from pychromecast.discovery import CastBrowser, SimpleCastListener
        from zeroconf import Zeroconf
        
        devices = {}
        discovery_events = []
        lock = threading.Lock()
        discovery_complete = threading.Event()
        
        def add_cast(uuid, service):
            with lock:
                try:
                    device_info = {
                        'uuid': uuid,
                        'name': service.friendly_name,
                        'host': service.host,
                        'port': service.port,
                        'model_name': service.model_name,
                        'manufacturer': service.manufacturer,
                        'timestamp': time.time()
                    }
                    devices[uuid] = device_info
                    discovery_events.append(('ADD', device_info))
                    
                    print(f"  ✅ FOUND: {service.friendly_name} ({service.model_name}) at {service.host}:{service.port}")
                    
                    # Signal completion after finding a few devices
                    if len(devices) >= 1:
                        discovery_complete.set()
                    
                except Exception as e:
                    print(f"  ❌ Error in add_cast: {e}")
        
        def remove_cast(uuid, service):
            with lock:
                if uuid in devices:
                    device_info = devices.pop(uuid)
                    discovery_events.append(('REMOVE', device_info))
                    print(f"  📤 REMOVED: {device_info['name']}")
        
        # Create explicit Zeroconf instance
        print("Creating explicit Zeroconf instance...")
        zc = Zeroconf()
        
        # Create listener and browser with explicit zeroconf
        listener = SimpleCastListener(add_cast, remove_cast)
        browser = CastBrowser(listener, zc, None)
        
        print("Starting CastBrowser discovery...")
        start_time = time.time()
        browser.start_discovery()
        
        # Wait for discovery - longer timeout
        timeout = 20
        print(f"Waiting {timeout} seconds for devices...")
        
        if discovery_complete.wait(timeout=timeout):
            elapsed = time.time() - start_time
            print(f"✅ Discovery completed in {elapsed:.1f}s")
        else:
            elapsed = time.time() - start_time
            print(f"⏰ Discovery timeout after {elapsed:.1f}s")
        
        print(f"Final count: {len(devices)} devices")
        
        if devices:
            print("📋 Devices found:")
            for uuid, info in devices.items():
                print(f"  - {info['name']} ({info['model_name']}) at {info['host']}:{info['port']}")
        else:
            print("❌ No devices found")
        
        # Cleanup
        browser.stop_discovery()
        zc.close()
        
        return len(devices), devices
        
    except Exception as e:
        print(f"❌ Manual CastBrowser test failed: {e}")
        import traceback
        traceback.print_exc()
        return 0, {}

def test_castbrowser_different_configs():
    """Test CastBrowser with different network configurations."""
    print("\n🌐 Testing CastBrowser with different network configs...")
    
    try:
        from pychromecast.discovery import CastBrowser, SimpleCastListener
        
        devices = {}
        lock = threading.Lock()
        discovery_complete = threading.Event()
        
        def add_cast(uuid, service):
            with lock:
                try:
                    device_info = {
                        'uuid': uuid,
                        'name': service.friendly_name,
                        'host': service.host,
                        'port': service.port,
                        'model_name': service.model_name,
                        'manufacturer': service.manufacturer
                    }
                    devices[uuid] = device_info
                    print(f"  ✅ DISCOVERED: {service.friendly_name}")
                    discovery_complete.set()
                    
                except Exception as e:
                    print(f"  ❌ Error in add_cast: {e}")
        
        def remove_cast(uuid, service):
            pass
        
        # Test different configurations
        configs = [
            {"host": None, "port": None, "description": "Default config"},
            {"host": "0.0.0.0", "port": None, "description": "Listen on all interfaces"},
        ]
        
        # Try to get local network interface
        try:
            s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            s.connect(("8.8.8.8", 80))
            local_ip = s.getsockname()[0]
            s.close()
            configs.append({"host": local_ip, "port": None, "description": f"Local IP: {local_ip}"})
        except:
            pass
        
        for config in configs:
            print(f"\n🔧 Testing: {config['description']}")
            print(f"   Host: {config['host']}, Port: {config['port']}")
            
            devices.clear()
            discovery_complete.clear()
            
            try:
                listener = SimpleCastListener(add_cast, remove_cast)
                browser = CastBrowser(listener, config['host'], config['port'])
                
                browser.start_discovery()
                
                # Wait for discovery
                if discovery_complete.wait(timeout=15):
                    print(f"   ✅ Found {len(devices)} devices")
                    if devices:
                        for uuid, info in list(devices.items())[:3]:  # Show first 3
                            print(f"     - {info['name']} ({info['model_name']})")
                        browser.stop_discovery()
                        return len(devices), devices  # Return on first success
                else:
                    print(f"   ❌ No devices found")
                
                browser.stop_discovery()
                
            except Exception as e:
                print(f"   ❌ Config failed: {e}")
                continue
        
        return 0, {}
        
    except Exception as e:
        print(f"❌ Network config test failed: {e}")
        return 0, {}

def test_raw_zeroconf_discovery():
    """Test raw zeroconf discovery to see what services are available."""
    print("\n🔍 Testing raw zeroconf service discovery...")
    
    try:
        from zeroconf import Zeroconf, ServiceBrowser, ServiceListener
        
        class TestListener(ServiceListener):
            def __init__(self):
                self.services = {}
                self.lock = threading.Lock()
            
            def add_service(self, zc, type_, name):
                with self.lock:
                    info = zc.get_service_info(type_, name)
                    if info:
                        self.services[name] = {
                            'name': name,
                            'type': type_, 
                            'addresses': [socket.inet_ntoa(addr) for addr in info.addresses],
                            'port': info.port,
                            'properties': info.properties
                        }
                        print(f"  📡 FOUND SERVICE: {name}")
                        if info.properties and b'fn' in info.properties:
                            friendly_name = info.properties[b'fn'].decode('utf-8')
                            print(f"      Friendly name: {friendly_name}")
            
            def remove_service(self, zc, type_, name):
                with self.lock:
                    if name in self.services:
                        del self.services[name]
                        print(f"  📤 REMOVED SERVICE: {name}")
            
            def update_service(self, zc, type_, name):
                pass
        
        zc = Zeroconf()
        listener = TestListener()
        
        print("Starting raw zeroconf browser for _googlecast._tcp.local.")
        browser = ServiceBrowser(zc, "_googlecast._tcp.local.", listener)
        
        # Wait for discovery
        print("Waiting 10 seconds for services...")
        time.sleep(10)
        
        print(f"\n📊 Found {len(listener.services)} raw services:")
        for name, info in listener.services.items():
            print(f"  - {name}")
            print(f"    Addresses: {info['addresses']}")
            print(f"    Port: {info['port']}")
            if b'fn' in info['properties']:
                print(f"    Friendly Name: {info['properties'][b'fn'].decode('utf-8')}")
        
        browser.cancel()
        zc.close()
        
        return len(listener.services)
        
    except Exception as e:
        print(f"❌ Raw zeroconf test failed: {e}")
        import traceback
        traceback.print_exc()
        return 0

def main():
    """Run comprehensive CastBrowser debugging."""
    print("🔧 CastBrowser Discovery Fix Tool")
    print("=" * 40)
    
    # First, see what get_chromecasts finds
    print("Step 1: Analyze get_chromecasts() behavior")
    old_browser = test_get_chromecasts_internals()
    
    # Test raw zeroconf
    print("\nStep 2: Test raw zeroconf discovery")
    raw_count = test_raw_zeroconf_discovery()
    
    # Test manual CastBrowser with explicit zeroconf
    print("\nStep 3: Test CastBrowser with manual setup")
    cb_count, cb_devices = test_manual_castbrowser_with_zeroconf()
    
    # Test different network configurations
    if cb_count == 0:
        print("\nStep 4: Test different network configurations")
        cb_count, cb_devices = test_castbrowser_different_configs()
    
    # Summary
    print("\n🎯 RESULTS SUMMARY")
    print("=" * 20)
    print(f"Raw zeroconf services:  {raw_count}")
    print(f"get_chromecasts():      11 devices")
    print(f"Fixed CastBrowser:      {cb_count} devices")
    
    if cb_count > 0:
        print("\n✅ SUCCESS! CastBrowser is now working")
        print("Key devices found:")
        for uuid, info in list(cb_devices.items())[:5]:
            print(f"  - {info['name']} ({info['model_name']})")
        
        # Check for Adahn
        adahn_found = any('adahn' in info['name'].lower() for info in cb_devices.values())
        if adahn_found:
            print("🎉 Your 'Adahn' group was found!")
        
    else:
        print("\n❌ CastBrowser still not working")
        print("This may be due to:")
        print("  • Network firewall blocking multicast")
        print("  • pychromecast version compatibility")
        print("  • System network configuration")
        print("\n💡 Recommendation: Use the fallback method in ChromecastManager")

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\n🛑 Test interrupted")
    except Exception as e:
        print(f"\n❌ Test failed: {e}")
        import traceback
        traceback.print_exc()
