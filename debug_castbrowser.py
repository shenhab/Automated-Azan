#!/usr/bin/env python3
"""
Debug CastBrowser Discovery Issues

This script helps debug why CastBrowser might not be discovering devices
that get_chromecasts() can find.
"""

import time
import socket
import threading
from typing import Dict, Any
import logging

# Setup detailed logging
logging.basicConfig(
    level=logging.DEBUG,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)

def test_network_interfaces():
    """Check available network interfaces."""
    print("🌐 Network Interface Analysis")
    print("=" * 30)
    
    try:
        hostname = socket.gethostname()
        local_ip = socket.gethostbyname(hostname)
        print(f"Hostname: {hostname}")
        print(f"Local IP: {local_ip}")
        
        # Get all network interfaces
        interfaces = socket.getaddrinfo(socket.gethostname(), None)
        unique_ips = set()
        for interface in interfaces:
            ip = interface[4][0]
            if not ip.startswith('127.'):  # Skip localhost
                unique_ips.add(ip)
        
        print("Available network interfaces:")
        for ip in sorted(unique_ips):
            print(f"  - {ip}")
            
    except Exception as e:
        print(f"❌ Network interface check failed: {e}")
    
    print()

def test_enhanced_castbrowser():
    """Test CastBrowser with enhanced configuration."""
    print("🔍 Enhanced CastBrowser Discovery")
    print("=" * 35)
    
    try:
        from pychromecast.discovery import CastBrowser, SimpleCastListener
        import pychromecast.discovery as discovery
        
        devices = {}
        discovery_events = []
        lock = threading.Lock()
        
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
                    
                    print(f"  ✅ DISCOVERED: {service.friendly_name}")
                    print(f"     Model: {service.model_name}")
                    print(f"     Host: {service.host}:{service.port}")
                    print(f"     UUID: {uuid}")
                    print(f"     Manufacturer: {service.manufacturer}")
                    print()
                    
                except Exception as e:
                    print(f"  ❌ Error processing discovered device: {e}")
        
        def remove_cast(uuid, service):
            with lock:
                if uuid in devices:
                    device_info = devices.pop(uuid)
                    discovery_events.append(('REMOVE', device_info))
                    print(f"  📤 REMOVED: {device_info['name']}")
        
        # Create listener and browser
        print("Creating CastBrowser with enhanced logging...")
        listener = SimpleCastListener(add_cast, remove_cast)
        
        # Test different network interface configurations
        network_configs = [
            (None, None),  # Default
        ]
        
        # Try to get local network interfaces
        try:
            hostname = socket.gethostname()
            local_ip = socket.gethostbyname(hostname)
            network_configs.append((local_ip, None))
        except:
            pass
        
        for host, port in network_configs:
            print(f"🔄 Testing CastBrowser with host={host}, port={port}")
            
            try:
                browser = CastBrowser(listener, host, port)
                
                print("  Starting discovery...")
                start_time = time.time()
                browser.start_discovery()
                
                # Wait longer for discovery - up to 30 seconds
                discovery_duration = 30
                print(f"  Waiting {discovery_duration} seconds for device discovery...")
                
                for i in range(discovery_duration):
                    time.sleep(1)
                    with lock:
                        if len(devices) > 0:
                            print(f"    Progress: Found {len(devices)} devices after {i+1}s")
                
                elapsed = time.time() - start_time
                print(f"  🏁 Discovery completed after {elapsed:.1f}s")
                print(f"  📊 Total devices found: {len(devices)}")
                
                # Stop discovery
                browser.stop_discovery()
                
                if devices:
                    print("  📋 Final device list:")
                    for uuid, info in devices.items():
                        print(f"    - {info['name']} ({info['model_name']})")
                    break  # Success, no need to try other configs
                else:
                    print("  ⚠️  No devices found with this configuration")
                
            except Exception as e:
                print(f"  ❌ CastBrowser test failed: {e}")
                continue
        
        print()
        print("📈 Discovery Statistics:")
        print(f"  Total events: {len(discovery_events)}")
        print(f"  Final device count: {len(devices)}")
        
        return len(devices), devices
        
    except ImportError as e:
        print(f"❌ Import error: {e}")
        return 0, {}
    except Exception as e:
        print(f"❌ Enhanced CastBrowser test failed: {e}")
        return 0, {}

def compare_with_get_chromecasts():
    """Compare results with get_chromecasts."""
    print("🔍 get_chromecasts() Reference Test")
    print("=" * 35)
    
    try:
        import pychromecast
        
        print("Running get_chromecasts() for comparison...")
        start_time = time.time()
        chromecasts, browser = pychromecast.get_chromecasts()
        elapsed = time.time() - start_time
        
        print(f"📊 get_chromecasts() found {len(chromecasts)} devices in {elapsed:.1f}s")
        
        device_names = []
        for cast in chromecasts:
            name = cast.name
            model = getattr(cast, 'model_name', 'Unknown')
            device_names.append(name)
            print(f"  - {name} ({model})")
        
        # Cleanup
        if browser:
            browser.stop_discovery()
        
        print()
        return len(chromecasts), device_names
        
    except Exception as e:
        print(f"❌ get_chromecasts() test failed: {e}")
        return 0, []

def main():
    """Run comprehensive CastBrowser debugging."""
    print("🔧 CastBrowser Discovery Debug Tool")
    print("=" * 40)
    print()
    
    # Test network setup
    test_network_interfaces()
    
    # Test get_chromecasts first (reference)
    gc_count, gc_devices = compare_with_get_chromecasts()
    
    # Test enhanced CastBrowser
    cb_count, cb_devices = test_enhanced_castbrowser()
    
    # Final analysis
    print("🎯 ANALYSIS & RECOMMENDATIONS")
    print("=" * 30)
    print(f"get_chromecasts(): {gc_count} devices")
    print(f"CastBrowser:       {cb_count} devices")
    print()
    
    if cb_count == 0 and gc_count > 0:
        print("❌ CastBrowser discovery is not working")
        print("🔧 Possible solutions:")
        print("   1. Network firewall blocking mDNS/multicast traffic")
        print("   2. CastBrowser timing issues - needs longer discovery time")
        print("   3. Network interface binding problems")
        print("   4. pychromecast version compatibility issues")
        print()
        print("💡 Try these fixes:")
        print("   • Increase discovery timeout in ChromecastManager")
        print("   • Check firewall settings for mDNS (port 5353)")
        print("   • Verify multicast is enabled on network interface")
        
    elif cb_count == gc_count:
        print("✅ CastBrowser discovery is working correctly!")
        
    elif cb_count < gc_count:
        print("⚠️  CastBrowser found fewer devices than get_chromecasts()")
        print("   This might be due to timing or different discovery methods")
        
    else:
        print("🤔 CastBrowser found more devices than get_chromecasts()")
        print("   This is unusual but not necessarily problematic")
    
    print()
    print("🎯 For your Islamic prayer automation:")
    if cb_count > 0 or gc_count > 0:
        print("✅ Chromecast discovery is functional")
        if 'Adahn' in str(gc_devices) or any('Adahn' in str(d) for d in cb_devices.values()):
            print("✅ Your 'Adahn' group was found - perfect for prayer calls!")
        else:
            print("⚠️  'Adahn' group not found, but other devices are available")
    else:
        print("❌ No Chromecast devices discovered by either method")
        print("   Check network connectivity and device power status")

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\n🛑 Debug interrupted by user")
    except Exception as e:
        print(f"\n❌ Debug failed: {e}")
        import traceback
        traceback.print_exc()
