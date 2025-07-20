#!/usr/bin/env python3
"""
Quick Chromecast Discovery Comparison Script

This script provides a quick comparison between CastBrowser and get_chromecasts()
methods for Chromecast device discovery.
"""

import time
import sys
from typing import Dict, Any

def test_castbrowser():
    """Test the modern CastBrowser approach."""
    print("🔄 Testing CastBrowser (modern approach)...")
    
    try:
        from pychromecast.discovery import CastBrowser, SimpleCastListener
        import threading
        
        devices = {}
        discovery_done = threading.Event()
        
        def add_cast(uuid, service):
            devices[uuid] = {
                'name': service.friendly_name,
                'model': service.model_name,
                'host': service.host,
                'port': service.port
            }
            print(f"  📱 Found: {service.friendly_name} ({service.model_name})")
            discovery_done.set()
            
        def remove_cast(uuid, service):
            if uuid in devices:
                device = devices.pop(uuid)
                print(f"  📤 Removed: {device['name']}")
        
        listener = SimpleCastListener(add_cast, remove_cast)
        browser = CastBrowser(listener, None, None)
        
        start_time = time.time()
        browser.start_discovery()
        
        # Wait for discovery
        discovery_done.wait(timeout=10)
        elapsed = time.time() - start_time
        
        browser.stop_discovery()
        
        print(f"  ✅ CastBrowser: Found {len(devices)} devices in {elapsed:.2f}s")
        return len(devices), elapsed, devices
        
    except Exception as e:
        print(f"  ❌ CastBrowser failed: {e}")
        return 0, 0, {}

def test_get_chromecasts():
    """Test the deprecated get_chromecasts approach."""
    print("🔄 Testing get_chromecasts() (deprecated)...")
    
    try:
        import pychromecast
        
        start_time = time.time()
        chromecasts, browser = pychromecast.get_chromecasts()
        elapsed = time.time() - start_time
        
        devices = {}
        for cast in chromecasts:
            devices[getattr(cast, 'uuid', cast.name)] = {
                'name': cast.name,
                'model': cast.model_name,
                'host': cast.host,
                'port': cast.port
            }
            print(f"  📱 Found: {cast.name} ({cast.model_name})")
        
        if browser:
            browser.stop_discovery()
            
        print(f"  ✅ get_chromecasts: Found {len(devices)} devices in {elapsed:.2f}s")
        return len(devices), elapsed, devices
        
    except Exception as e:
        print(f"  ❌ get_chromecasts failed: {e}")
        return 0, 0, {}

def main():
    """Run comparison tests."""
    print("🕌 Chromecast Discovery Method Comparison")
    print("=" * 45)
    print()
    
    # Test CastBrowser
    cb_count, cb_time, cb_devices = test_castbrowser()
    print()
    
    # Test get_chromecasts
    gc_count, gc_time, gc_devices = test_get_chromecasts()
    print()
    
    # Results comparison
    print("📊 COMPARISON RESULTS")
    print("=" * 21)
    print(f"CastBrowser:     {cb_count} devices in {cb_time:.2f}s")
    print(f"get_chromecasts: {gc_count} devices in {gc_time:.2f}s")
    print()
    
    # Check for consistency
    if cb_count == gc_count and cb_count > 0:
        print("✅ Both methods found the same number of devices")
        
        # Compare device names
        cb_names = {d['name'] for d in cb_devices.values()}
        gc_names = {d['name'] for d in gc_devices.values()}
        
        if cb_names == gc_names:
            print("✅ Both methods found the same devices")
        else:
            print("⚠️  Methods found different devices:")
            print(f"   CastBrowser only: {cb_names - gc_names}")
            print(f"   get_chromecasts only: {gc_names - cb_names}")
    
    elif cb_count != gc_count:
        print("⚠️  Methods found different numbers of devices")
        print("   This can happen due to timing or network conditions")
    
    elif cb_count == 0 and gc_count == 0:
        print("⚠️  No devices found by either method")
        print("   This is expected if no Chromecast devices are on the network")
    
    # Recommendations
    print()
    print("💡 RECOMMENDATIONS")
    print("=" * 18)
    if cb_count > 0 or gc_count > 0:
        print("✅ Chromecast discovery is working on your network")
    else:
        print("❌ No Chromecast devices discovered")
        print("   • Check that devices are powered on")
        print("   • Verify devices are on the same network")
        print("   • Check firewall/multicast settings")
    
    print()
    print("🎯 For production use:")
    print("   • Use CastBrowser (modern, supported)")
    print("   • Avoid get_chromecasts() (deprecated)")
    print("   • Use ChromecastManager for best abstraction")

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\n🛑 Test interrupted by user")
        sys.exit(1)
    except Exception as e:
        print(f"\n❌ Test failed with error: {e}")
        sys.exit(1)
