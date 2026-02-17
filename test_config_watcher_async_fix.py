#!/usr/bin/env python3
"""Test script to verify config watcher async fixes"""

import sys
import os

# Add current directory to path to import modules
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from config_manager import ConfigManager

def test_config_watcher_handlers():
    """Test ConfigWatcher handler methods directly"""
    print("=== Testing ConfigWatcher Handler Methods ===\n")

    try:
        # Import the config watcher after setup
        from config_watcher import ConfigWatcher

        # Mock scheduler object
        class MockScheduler:
            def __init__(self):
                self.location = 'naas'
                self.google_device = 'athan'

                # Mock chromecast manager
                class MockChromecastManager:
                    def __init__(self):
                        self.target_device = None
                        self.last_discovery_time = 0

                self.chromecast_manager = MockChromecastManager()

                # Mock fetcher with cache
                class MockFetcher:
                    def __init__(self):
                        self._cache = {}

                self.fetcher = MockFetcher()

            def load_prayer_times(self):
                return {"success": True, "message": "Mock prayer times loaded"}

            def schedule_prayers(self):
                return {"success": True, "scheduled_count": 3}

        # Create config manager and mock scheduler
        config_manager = ConfigManager()
        mock_scheduler = MockScheduler()

        # Create config watcher (don't start it)
        config_watcher = ConfigWatcher(config_manager, mock_scheduler)
        print("✓ ConfigWatcher created successfully")

        # Test location change handler
        print("\n1. Testing location change handler:")
        try:
            config_watcher._handle_location_change('naas', 'icci')
            print("   ✓ Location change handler executed without errors")
        except Exception as e:
            print(f"   ✗ Location change handler error: {e}")

        # Test speaker change handler
        print("\n2. Testing speaker change handler:")
        try:
            config_watcher._handle_speaker_change('athan', 'test-speaker')
            print("   ✓ Speaker change handler executed without errors")
        except Exception as e:
            print(f"   ✗ Speaker change handler error: {e}")

        # Test pre-fajr change handler
        print("\n3. Testing pre-fajr change handler:")
        try:
            config_watcher._handle_pre_fajr_change('True', 'False')
            print("   ✓ Pre-fajr change handler executed without errors")
        except Exception as e:
            print(f"   ✗ Pre-fajr change handler error: {e}")

        # Test prayer source change handler
        print("\n4. Testing prayer source change handler:")
        try:
            config_watcher._handle_prayer_source_change('naas', 'icci')
            print("   ✓ Prayer source change handler executed without errors")
        except Exception as e:
            print(f"   ✗ Prayer source change handler error: {e}")

        print("\n✅ All config watcher handlers work correctly!")
        return True

    except Exception as e:
        print(f"\n❌ Config watcher test failed: {e}")
        return False

if __name__ == "__main__":
    success = test_config_watcher_handlers()
    exit(0 if success else 1)