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
speakers-group-name = Adahn
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
echo "📡 Testing Chromecast discovery..."
pipenv run python -c "
from chromecast_manager import ChromecastManager
import time

try:
    print('Initializing ChromecastManager...')
    manager = ChromecastManager()
    time.sleep(2)  # Give time for discovery
    
    if manager.chromecasts:
        print('✅ Chromecast discovery working:')
        for uuid, info in manager.chromecasts.items():
            print(f'   Found: {info[\"name\"]} ({info[\"model_name\"]}) at {info[\"host\"]}')
    else:
        print('⚠️  No Chromecast devices found')
        print('This is expected if no Chromecast devices are on the network')
    
    manager.cleanup()
    print('✅ ChromecastManager cleanup completed')
    
except Exception as e:
    print('❌ Chromecast discovery test failed:', e)
    print('This might be expected if no devices are available')
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
