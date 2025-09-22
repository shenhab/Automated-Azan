#!/usr/bin/env python3
"""
Test script for Chromecast functionality
Target: Kitchen speaker (Google Nest Mini) at 192.168.86.172:8009
"""

import sys
import time
import logging
from chromecast_manager import ChromecastManager

# Set up logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s [%(levelname)s]: %(message)s')

def test_kitchen_speaker():
    """Test the Kitchen speaker specifically"""
    print("=" * 60)
    print("Testing Kitchen speaker (Google Nest Mini) at 192.168.86.172:8009")
    print("=" * 60)

    # Initialize ChromecastManager
    cm = ChromecastManager()

    # Discover devices first
    print("\n1. Discovering devices...")
    discovery_result = cm.discover_devices(force_rediscovery=True)
    print(f"Discovery result: {discovery_result.get('success', False)}")
    print(f"Devices found: {discovery_result.get('devices_found', 0)}")

    # Find the Kitchen speaker specifically
    kitchen_speaker = None
    for uuid, cast_info in cm.chromecasts.items():
        if "Kitchen speaker" in cast_info['name'] or cast_info['host'] == '192.168.86.172':
            kitchen_speaker = cast_info
            print(f"\n✓ Found Kitchen speaker: {cast_info['name']} at {cast_info['host']}:{cast_info['port']}")
            break

    if not kitchen_speaker:
        print("❌ Kitchen speaker not found!")
        return False

    # Test connection
    print("\n2. Testing connection...")
    cast_device = cm._create_cast_device(kitchen_speaker)
    if not cast_device:
        print("❌ Failed to create cast device!")
        return False

    connection_result = cm._connect_with_retry(cast_device, max_retries=2)
    if not connection_result.get('success', False):
        print(f"❌ Connection failed: {connection_result.get('error', 'Unknown error')}")
        return False

    print(f"✓ Connected successfully to {cast_device.name}")

    # Test media URL generation
    print("\n3. Testing media URL generation...")
    url_result = cm._get_media_url("media_Athan.mp3")
    if not url_result.get('success', False):
        print(f"❌ Media URL generation failed: {url_result.get('error', 'Unknown error')}")
        return False

    media_url = url_result['media_url']
    print(f"✓ Media URL: {media_url}")

    # Test media playback
    print("\n4. Testing media playback...")
    print(f"Attempting to play: {media_url}")

    # Set the target device manually for this test
    cm.target_device = cast_device

    playback_result = cm.play_url_on_cast(media_url, max_retries=1, preserve_target=True)

    if playback_result.get('success', False):
        print("✓ Media playback started successfully!")
        print(f"  Device: {playback_result.get('device', {}).get('name', 'Unknown')}")
        print(f"  Total time: {playback_result.get('total_time', 0)} seconds")
        print(f"  Load result: {playback_result.get('load_result', {}).get('success', False)}")

        # Wait a bit to see if it stays playing
        print("\nWaiting 10 seconds to monitor playback...")
        for i in range(10):
            try:
                cast_device.media_controller.update_status()
                state = cast_device.media_controller.status.player_state
                content_id = cast_device.media_controller.status.content_id
                print(f"  {i+1:2d}s: State={state}, Content={'✓' if content_id == media_url else '✗'}")
                time.sleep(1)
            except Exception as e:
                print(f"  {i+1:2d}s: Error checking status: {e}")
                time.sleep(1)

        return True
    else:
        print(f"❌ Media playback failed: {playback_result.get('error', 'Unknown error')}")
        print(f"Playback attempts: {len(playback_result.get('playback_attempts', []))}")
        return False

def main():
    """Main test function"""
    try:
        success = test_kitchen_speaker()
        print("\n" + "=" * 60)
        if success:
            print("🎉 TEST PASSED: Kitchen speaker Chromecast test successful!")
        else:
            print("❌ TEST FAILED: Kitchen speaker Chromecast test failed!")
        print("=" * 60)
        return 0 if success else 1
    except Exception as e:
        print(f"\n❌ TEST ERROR: {e}")
        import traceback
        traceback.print_exc()
        return 1

if __name__ == "__main__":
    sys.exit(main())