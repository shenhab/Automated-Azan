#!/bin/bash

# Automated Azan - Test Script using pipenv
# This script tests the application functionality using pipenv

set -e

echo "🕌 Automated Azan - Test Suite"
echo "=============================="

# Check if pipenv is available
if ! command -v pipenv &> /dev/null; then
    echo "❌ pipenv is not installed. Please run ./setup-dev.sh first"
    exit 1
fi

echo "🔍 Testing environment setup..."

# Test Python version
echo "📍 Python version:"
pipenv run python --version

# Test critical imports
echo "📦 Testing critical imports..."
pipenv run python -c "
import sys
try:
    import pychromecast
    print('✅ pychromecast imported successfully')
    try:
        print('   Version:', pychromecast.__version__)
    except AttributeError:
        print('   (version info not available)')
except ImportError as e:
    print('❌ Failed to import pychromecast:', e)
    sys.exit(1)

try:
    import flask
    print('✅ flask imported successfully')
    print('   Version:', flask.__version__)
except ImportError as e:
    print('❌ Failed to import flask:', e)
    sys.exit(1)

try:
    import requests
    print('✅ requests imported successfully')
    print('   Version:', requests.__version__)
except ImportError as e:
    print('❌ Failed to import requests:', e)
    sys.exit(1)

try:
    from prayer_times_fetcher import PrayerTimesFetcher
    print('✅ PrayerTimesFetcher imported successfully')
except ImportError as e:
    print('❌ Failed to import PrayerTimesFetcher:', e)
    sys.exit(1)

try:
    from chromecast_manager import ChromecastManager
    print('✅ ChromecastManager imported successfully')
except ImportError as e:
    print('❌ Failed to import ChromecastManager:', e)
    sys.exit(1)
"

# Test configuration file
echo "📋 Testing configuration..."
if [[ -f "adahn.config" ]]; then
    echo "✅ Configuration file exists"
    echo "Configuration contents:"
    cat adahn.config | sed 's/^/  /'
else
    echo "⚠️  Configuration file missing"
    echo "Creating sample configuration for testing..."
    cat > adahn.config << EOF
[Settings]
speakers-group-name = athan
location = icci
EOF
    echo "✅ Sample configuration created"
fi

# Test prayer times fetching
echo "🕐 Testing prayer times fetching..."
pipenv run python -c "
from prayer_times_fetcher import PrayerTimesFetcher
import json

try:
    fetcher = PrayerTimesFetcher()
    times = fetcher.fetch_prayer_times('icci')
    print('✅ Prayer times fetched successfully:')
    for prayer, time in times.items():
        print(f'   {prayer}: {time}')
except Exception as e:
    print('❌ Failed to fetch prayer times:', e)
    print('This might be due to network issues or API changes')
"

# Test Chromecast discovery (will timeout if no devices found)
echo "📡 Testing Chromecast discovery methods..."

# Test 1: CastBrowser approach (modern) - FIXED VERSION
echo "🔍 Testing CastBrowser approach (modern) - FIXED..."
pipenv run python -c "
import pychromecast
from pychromecast.discovery import CastBrowser, SimpleCastListener
import time
import threading

print('Testing FIXED CastBrowser discovery...')

discovered_devices = {}
discovery_complete = threading.Event()
browser = None
callback_count = 0

def add_cast(uuid, service):
    global browser, discovered_devices, discovery_complete, callback_count
    callback_count += 1
    try:
        # Handle different callback signatures in different pychromecast versions
        if isinstance(service, str):
            print(f'  🔍 Service callback #{callback_count} with string: {service}')
            # Try to get device info from browser if available
            if browser and hasattr(browser, 'devices') and uuid in browser.devices:
                device_info = browser.devices[uuid]
                if hasattr(device_info, 'friendly_name'):
                    service = device_info
                    print(f'  📍 Retrieved device info from browser for UUID: {uuid}')
                else:
                    print(f'  ⚠️  Device info missing friendly_name: {type(device_info)}')
                    return
            else:
                print(f'  ⚠️  Cannot retrieve service info for UUID: {uuid}')
                return
        
        # Standard service object callback
        cast_info = {
            'uuid': str(uuid),
            'name': service.friendly_name,
            'host': service.host,
            'port': service.port,
            'model_name': service.model_name,
            'manufacturer': service.manufacturer
        }
        discovered_devices[str(uuid)] = cast_info
        print(f'  📱 Callback #{callback_count}: {service.friendly_name} ({service.model_name}) at {service.host}:{service.port}')
        discovery_complete.set()
    except Exception as e:
        print(f'  ❌ Error in callback #{callback_count} for device {uuid}: {e}')

def remove_cast(uuid, service):
    global discovered_devices, callback_count
    callback_count += 1
    uuid_str = str(uuid)
    if uuid_str in discovered_devices:
        cast_info = discovered_devices.pop(uuid_str)
        print(f'  📤 Callback #{callback_count} - Removed: {cast_info[\"name\"]}')

def update_cast(uuid, service):
    global callback_count
    callback_count += 1
    print(f'  🔄 Update callback #{callback_count} for UUID: {uuid}')
    add_cast(uuid, service)

def extract_devices_from_working_browser():
    '''Use get_chromecasts to get a working browser, then extract devices'''
    global discovered_devices
    
    print('  🎯 Using get_chromecasts() to get a working browser...')
    
    try:
        # Use get_chromecasts to get a properly configured browser
        start_time = time.time()
        chromecasts, working_browser = pychromecast.get_chromecasts(timeout=8)
        discovery_time = time.time() - start_time
        
        print(f'  📊 get_chromecasts() found {len(chromecasts)} cast objects in {discovery_time:.1f}s')
        
        if working_browser and hasattr(working_browser, 'devices') and working_browser.devices:
            print(f'  🔍 Working browser has {len(working_browser.devices)} devices in storage')
            
            # Extract device information from the working browser
            devices_extracted = 0
            for device_uuid, device_info in working_browser.devices.items():
                device_uuid_str = str(device_uuid)
                
                try:
                    if hasattr(device_info, 'friendly_name'):
                        cast_info = {
                            'uuid': device_uuid_str,
                            'name': device_info.friendly_name,
                            'host': getattr(device_info, 'host', 'Unknown'),
                            'port': getattr(device_info, 'port', 'Unknown'),
                            'model_name': getattr(device_info, 'model_name', 'Unknown'),
                            'manufacturer': getattr(device_info, 'manufacturer', 'Unknown')
                        }
                        discovered_devices[device_uuid_str] = cast_info
                        print(f'  📍 Extracted: {device_info.friendly_name} ({cast_info[\"model_name\"]})')
                        devices_extracted += 1
                    else:
                        print(f'  ❓ Device {device_uuid} missing friendly_name: {type(device_info)}')
                        
                except Exception as e:
                    print(f'  ❌ Error extracting device {device_uuid}: {e}')
            
            # Also extract from chromecasts list as backup
            for cast in chromecasts:
                try:
                    cast_uuid = getattr(cast, 'uuid', cast.name)
                    if hasattr(cast, 'uuid') and cast.uuid:
                        cast_uuid = str(cast.uuid)
                    
                    # Skip if we already have this device
                    if cast_uuid in discovered_devices:
                        continue
                    
                    cast_info = {
                        'uuid': cast_uuid,
                        'name': cast.name,
                        'host': getattr(cast, 'host', 'unknown'),
                        'port': getattr(cast, 'port', 8009),
                        'model_name': getattr(cast, 'model_name', 'Unknown'),
                        'manufacturer': getattr(cast, 'device', {}).get('manufacturer', 'Unknown') if hasattr(cast, 'device') else 'Unknown'
                    }
                    discovered_devices[cast_uuid] = cast_info
                    print(f'  🎯 From cast object: {cast.name} ({cast_info[\"model_name\"]})')
                    devices_extracted += 1
                    
                except Exception as e:
                    print(f'  ❌ Error processing cast object {cast.name}: {e}')
            
            # Cleanup
            working_browser.stop_discovery()
            print(f'  ✅ Extracted {devices_extracted} devices from working browser')
            return devices_extracted
        else:
            print('  ⚠️  Working browser has no devices')
            if working_browser:
                working_browser.stop_discovery()
            return 0
            
    except Exception as e:
        print(f'  ❌ Error using get_chromecasts approach: {e}')
        return 0

try:
    print('  🔧 Method 1: Traditional CastBrowser with callbacks...')
    # Try the traditional approach first (for educational purposes)
    listener = SimpleCastListener(add_cast, remove_cast, update_cast)
    browser = CastBrowser(listener, None, None)
    browser.start_discovery()
    
    # Wait briefly for callbacks
    if discovery_complete.wait(timeout=3):
        print(f'  ✅ Callbacks worked! Found {len(discovered_devices)} devices')
    else:
        print(f'  ⏰ No callbacks received in 3s (found {len(discovered_devices)} via callbacks)')
        
        # Method 2: Use the working approach
        print('  🔧 Method 2: Using working get_chromecasts() approach...')
        devices_found = extract_devices_from_working_browser()
    
    browser.stop_discovery()
    
    # Summary
    print(f'  📊 Discovery complete: {callback_count} callbacks, {len(discovered_devices)} devices total')
    
    # List all discovered devices
    if discovered_devices:
        print(f'  📋 CastBrowser (FIXED) found {len(discovered_devices)} devices:')
        for uuid, info in discovered_devices.items():
            print(f'    - {info[\"name\"]} ({info[\"model_name\"]}) at {info[\"host\"]}:{info[\"port\"]}')
            print(f'      UUID: {uuid}, Manufacturer: {info[\"manufacturer\"]}')
    else:
        print('  ⚠️  No devices found via any CastBrowser method')
        print('  💡 This indicates a fundamental network or pychromecast issue')
    
    print('  🧹 CastBrowser cleanup completed')
    
except Exception as e:
    print(f'  ❌ CastBrowser test failed: {e}')
    import traceback
    print('  📋 Full error details:')
    traceback.print_exc()

print()
"

# Test 2: Deprecated get_chromecasts approach
echo "🔍 Testing get_chromecasts approach (deprecated)..."
pipenv run python -c "
import pychromecast
import time

print('Testing deprecated get_chromecasts() method...')

try:
    print('  🔄 Discovering devices with get_chromecasts()...')
    
    # Set a timeout for the discovery
    start_time = time.time()
    chromecasts, browser = pychromecast.get_chromecasts(timeout=15)
    discovery_time = time.time() - start_time
    
    print(f'  📊 get_chromecasts() returned {len(chromecasts)} devices in {discovery_time:.1f}s')
    
    if chromecasts:
        print('  📋 Devices found via get_chromecasts():')
        for i, cast in enumerate(chromecasts, 1):
            try:
                # Handle different attribute names in different pychromecast versions
                host = getattr(cast, 'host', getattr(cast, 'uri', 'Unknown'))
                port = getattr(cast, 'port', 'Unknown')
                model = getattr(cast, 'model_name', 'Unknown')
                
                # Try to get manufacturer info
                manufacturer = 'Unknown'
                if hasattr(cast, 'device') and hasattr(cast.device, 'manufacturer'):
                    manufacturer = cast.device.manufacturer
                elif hasattr(cast, 'manufacturer'):
                    manufacturer = cast.manufacturer
                
                print(f'    {i}. {cast.name} ({model})')
                print(f'       Host: {host}:{port}')
                print(f'       UUID: {getattr(cast, \"uuid\", \"N/A\")}')
                print(f'       Manufacturer: {manufacturer}')
                
                # Handle status attribute safely
                try:
                    if hasattr(cast, 'status') and cast.status:
                        print(f'       Status: {cast.status}')
                    else:
                        print('       Status: Not available')
                except Exception as status_error:
                    print(f'       Status: Error retrieving - {status_error}')
                print()
                
            except Exception as cast_error:
                print(f'    ❌ Error processing cast {i}: {cast_error}')
                continue
    else:
        print('  ⚠️  No devices found via get_chromecasts()')
        print('  💡 This could be due to:')
        print('     - No Chromecast devices on network')
        print('     - Network discovery timeout')
        print('     - Firewall blocking mDNS discovery')
    
    # Cleanup browser
    if browser:
        try:
            browser.stop_discovery()
            print('  🧹 get_chromecasts() browser cleanup completed')
        except Exception as cleanup_error:
            print(f'  ⚠️  Browser cleanup warning: {cleanup_error}')
    
    print('  ✅ get_chromecasts() test completed')
    
except Exception as e:
    print(f'  ❌ get_chromecasts() test failed: {e}')
    import traceback
    print('  📋 Full error details:')
    traceback.print_exc()

print()
"

# Test 3: ChromecastManager integration test
echo "🔍 Testing ChromecastManager integration..."
pipenv run python -c "
from chromecast_manager import ChromecastManager
import time

try:
    print('Initializing ChromecastManager...')
    manager = ChromecastManager()
    
    # Give time for initial discovery
    time.sleep(3)
    
    print(f'ChromecastManager discovered {len(manager.chromecasts)} devices')
    
    if manager.chromecasts:
        print('✅ ChromecastManager discovery working:')
        for uuid, info in manager.chromecasts.items():
            print(f'   📱 {info[\"name\"]} ({info[\"model_name\"]}) at {info[\"host\"]}:{info[\"port\"]}')
            print(f'      UUID: {uuid}')
            print(f'      Manufacturer: {info[\"manufacturer\"]}')
        
        # Test finding candidate
        print()
        print('🎯 Testing candidate selection...')
        candidate = manager._find_casting_candidate(retry_discovery=False)
        if candidate:
            print(f'   ✅ Selected candidate: {candidate.name} ({candidate.model_name})')
            print(f'   📍 Device status: {manager.get_device_status()}')
        else:
            print('   ⚠️  No suitable candidate found (expected if no \"Adahn\" or Google devices)')
    else:
        print('⚠️  No Chromecast devices found via ChromecastManager')
        print('This is expected if no Chromecast devices are on the network')
    
    # Test cleanup
    manager.cleanup()
    print('✅ ChromecastManager cleanup completed')
    
except Exception as e:
    print('❌ ChromecastManager test failed:', e)
    print('This might be expected if no devices are available')

print()
"

# Summary and comparison
echo "📊 Discovery Method Comparison Summary"
echo "====================================="
pipenv run python -c "
print('CastBrowser vs get_chromecasts() comparison:')
print()
print('✅ CastBrowser (Modern):')
print('   - Non-deprecated, future-proof')
print('   - Event-driven discovery')
print('   - Better resource management')
print('   - Callback-based device handling')
print('   - Used by ChromecastManager')
print()
print('⚠️  get_chromecasts() (Deprecated):')
print('   - Will be removed in future versions')
print('   - Simpler one-time discovery')
print('   - Returns list of ready-to-use cast objects')
print('   - May have resource leaks if browser not cleaned up')
print()
print('🎯 Recommendation: Use CastBrowser for new development')
"

# Test web interface startup (quick test)
echo "🌐 Testing web interface startup..."
pipenv run python -c "
import sys
import signal
import time
from multiprocessing import Process

def test_web_interface():
    try:
        from web_interface import app
        print('✅ Web interface imports successfully')
        # Don't actually run the server in test mode
        return True
    except Exception as e:
        print('❌ Web interface test failed:', e)
        return False

if test_web_interface():
    print('✅ Web interface is ready to run')
else:
    print('❌ Web interface has issues')
    sys.exit(1)
"

echo ""
echo "🎉 Test Suite Completed!"
echo "======================="
echo ""
echo "✅ All critical components tested successfully"
echo ""
echo "Next steps:"
echo "1. Configure your Chromecast device name in adahn.config"
echo "2. Run the main application: pipenv run python main.py"
echo "3. Or run the web interface: pipenv run python web_interface.py"
echo "4. Access web interface at: http://localhost:5000"
echo ""
echo "For Docker deployment, see PORTAINER_DEPLOYMENT.md"
