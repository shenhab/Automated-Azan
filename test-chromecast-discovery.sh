#!/bin/bash

# Automated Azan - Comprehensive Chromecast Discovery Test
# This script thoroughly tests both modern and deprecated Chromecast discovery methods

set -e

echo "🕌 Automated Azan - Chromecast Discovery Test Suite"
echo "=================================================="

# Check if pipenv is available
if ! command -v pipenv &> /dev/null; then
    echo "❌ pipenv is not installed. Please run ./setup-dev.sh first"
    exit 1
fi

echo "🔍 Testing Chromecast discovery methods..."
echo ""

# Test 1: Raw CastBrowser test with detailed logging
echo "1️⃣  Testing CastBrowser (Modern Approach)"
echo "========================================="
pipenv run python -c "
from pychromecast.discovery import CastBrowser, SimpleCastListener
import time
import threading
import logging

# Enable debug logging for pychromecast
logging.basicConfig(level=logging.DEBUG)
logger = logging.getLogger('pychromecast')

discovered_devices = {}
discovery_complete = threading.Event()

def add_cast(uuid, service):
    try:
        cast_info = {
            'uuid': uuid,
            'name': service.friendly_name,
            'host': service.host,
            'port': service.port,
            'model_name': service.model_name,
            'manufacturer': service.manufacturer,
            'cast_type': getattr(service, 'cast_type', 'Unknown')
        }
        discovered_devices[uuid] = cast_info
        print(f'📱 DISCOVERED: {service.friendly_name}')
        print(f'   Model: {service.model_name}')
        print(f'   Host: {service.host}:{service.port}')
        print(f'   UUID: {uuid}')
        print(f'   Type: {cast_info[\"cast_type\"]}')
        print()
        
        # Signal completion after first device (for faster testing)
        discovery_complete.set()
        
    except Exception as e:
        print(f'❌ Error processing discovered device: {e}')

def remove_cast(uuid, service):
    if uuid in discovered_devices:
        cast_info = discovered_devices.pop(uuid)
        print(f'📤 REMOVED: {cast_info[\"name\"]} ({uuid})')

print('🔄 Starting CastBrowser discovery...')
print('   Timeout: 15 seconds')
print()

try:
    # Disable excessive debug output for cleaner test results
    logging.getLogger('pychromecast.socket_client').setLevel(logging.WARNING)
    
    # Create listener and browser
    listener = SimpleCastListener(add_cast, remove_cast)
    browser = CastBrowser(listener, None, None)
    
    # Start discovery
    browser.start_discovery()
    
    # Wait for discovery with timeout
    start_time = time.time()
    if discovery_complete.wait(timeout=15):
        elapsed = time.time() - start_time
        print(f'✅ Discovery completed in {elapsed:.2f} seconds')
    else:
        print(f'⏰ Discovery timeout after 15 seconds')
    
    # Additional wait to catch more devices
    time.sleep(2)
    
    # Results summary
    print(f'📊 CASTBROWSER RESULTS: {len(discovered_devices)} devices found')
    if discovered_devices:
        for uuid, info in discovered_devices.items():
            print(f'   ✓ {info[\"name\"]} ({info[\"model_name\"]}) - {info[\"host\"]}')
    else:
        print('   No devices discovered')
    
    # Cleanup
    browser.stop_discovery()
    print('🧹 CastBrowser cleaned up')
    
except Exception as e:
    print(f'❌ CastBrowser test failed: {e}')
    import traceback
    traceback.print_exc()

print()
print('=' * 50)
print()
"

# Test 2: Deprecated get_chromecasts test
echo "2️⃣  Testing get_chromecasts() (Deprecated)"
echo "========================================"
pipenv run python -c "
import pychromecast
import time

print('🔄 Starting get_chromecasts() discovery...')
print('   This method is deprecated and will be removed')
print()

try:
    start_time = time.time()
    chromecasts, browser = pychromecast.get_chromecasts()
    elapsed = time.time() - start_time
    
    print(f'✅ get_chromecasts() completed in {elapsed:.2f} seconds')
    print(f'📊 GET_CHROMECASTS RESULTS: {len(chromecasts)} devices found')
    
    if chromecasts:
        for i, cast in enumerate(chromecasts, 1):
            print(f'   {i}. {cast.name}')
            print(f'      Model: {cast.model_name}')
            print(f'      Host: {cast.host}:{cast.port}')
            print(f'      UUID: {getattr(cast, \"uuid\", \"N/A\")}')
            print(f'      Status: {cast.status}')
            
            # Try to get additional info
            try:
                cast.wait(timeout=2)
                print(f'      Connected: Yes')
                print(f'      App: {cast.app_display_name}')
            except:
                print(f'      Connected: No (timeout)')
            print()
    else:
        print('   No devices discovered')
    
    # Cleanup
    if browser:
        browser.stop_discovery()
        print('🧹 get_chromecasts() browser cleaned up')
    
except Exception as e:
    print(f'❌ get_chromecasts() test failed: {e}')
    import traceback
    traceback.print_exc()

print()
print('=' * 50)
print()
"

# Test 3: ChromecastManager comprehensive test
echo "3️⃣  Testing ChromecastManager Integration"
echo "======================================"
pipenv run python -c "
from chromecast_manager import ChromecastManager
import time
import json

print('🔄 Testing ChromecastManager...')
print()

try:
    # Initialize manager
    print('Initializing ChromecastManager...')
    manager = ChromecastManager()
    
    # Wait for discovery
    print('Waiting for initial discovery (5 seconds)...')
    time.sleep(5)
    
    print(f'📊 CHROMECASTMANAGER RESULTS: {len(manager.chromecasts)} devices found')
    
    if manager.chromecasts:
        for uuid, info in manager.chromecasts.items():
            print(f'   📱 {info[\"name\"]}')
            print(f'      Model: {info[\"model_name\"]}')
            print(f'      Manufacturer: {info[\"manufacturer\"]}')
            print(f'      Host: {info[\"host\"]}:{info[\"port\"]}')
            print(f'      UUID: {uuid}')
            
            # Test availability
            if manager._is_device_available(info):
                print(f'      Network: ✅ Available')
            else:
                print(f'      Network: ❌ Unavailable')
            print()
        
        # Test candidate selection
        print('🎯 Testing device selection logic...')
        candidate = manager._find_casting_candidate(retry_discovery=False)
        if candidate:
            print(f'   ✅ Selected: {candidate.name} ({candidate.model_name})')
            
            # Test device status
            status = manager.get_device_status()
            print(f'   📊 Status: {json.dumps(status, indent=6)}')
            
            # Test connectivity
            if manager._is_device_available_by_cast(candidate):
                print(f'   🌐 Connectivity: Available')
            else:
                print(f'   🌐 Connectivity: Unavailable')
        else:
            print('   ⚠️  No suitable candidate found')
            print('   (Looking for \"Adahn\" or Google Home/Nest devices)')
        
        # Test forced rediscovery
        print()
        print('🔄 Testing forced rediscovery...')
        manager.discover_devices(force_rediscovery=True)
        time.sleep(3)
        print(f'   After rediscovery: {len(manager.chromecasts)} devices')
        
    else:
        print('   No devices found by ChromecastManager')
        print('   This is expected if no Chromecast devices are on the network')
    
    # Cleanup
    manager.cleanup()
    print('🧹 ChromecastManager cleaned up')
    
except Exception as e:
    print(f'❌ ChromecastManager test failed: {e}')
    import traceback
    traceback.print_exc()

print()
print('=' * 50)
print()
"

# Performance comparison
echo "4️⃣  Performance & Feature Comparison"
echo "=================================="
pipenv run python -c "
print('📊 METHOD COMPARISON SUMMARY:')
print()
print('🏆 CastBrowser (Recommended):')
print('   ✅ Modern, supported API')
print('   ✅ Event-driven discovery')
print('   ✅ Real-time device add/remove callbacks')
print('   ✅ Better resource management')
print('   ✅ Non-blocking discovery')
print('   ✅ Used by ChromecastManager')
print('   ⚠️  Slightly more complex setup')
print()
print('🔄 get_chromecasts() (Deprecated):')
print('   ❌ Will be removed in pychromecast 15.0+ (June 2024)')
print('   ❌ Potential resource leaks')
print('   ❌ Blocking discovery')
print('   ✅ Simple to use')
print('   ✅ Returns ready-to-use cast objects')
print('   ✅ Good for quick scripts')
print()
print('🎯 RECOMMENDATIONS:')
print('   • Use CastBrowser for production applications')
print('   • Migrate away from get_chromecasts() before June 2024')
print('   • ChromecastManager provides the best abstraction')
print('   • Always cleanup browser resources')
print()
print('🔧 TROUBLESHOOTING:')
print('   • No devices found: Check network connectivity')
print('   • Timeout issues: Increase discovery timeout')
print('   • Connection errors: Verify firewall settings')
print('   • Resource leaks: Always call cleanup/stop_discovery')
"

echo ""
echo "🎉 Comprehensive Chromecast Discovery Test Completed!"
echo "===================================================="
echo ""
echo "📋 Summary:"
echo "• Tested both modern CastBrowser and deprecated get_chromecasts()"
echo "• Verified ChromecastManager integration"
echo "• Compared performance and features"
echo ""
echo "💡 Next steps:"
echo "• If no devices found, check network configuration"
echo "• Consider using ChromecastManager for production"
echo "• Plan migration away from deprecated methods"
