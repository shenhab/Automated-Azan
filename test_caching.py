#!/usr/bin/env python3
"""Test script to verify Chromecast discovery caching"""

import time
import logging
from chromecast_manager import ChromecastManager

# Setup logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s [%(levelname)s]: %(message)s')

def test_caching():
    """Test that multiple ChromecastManager instances use shared cache"""

    print("\n=== Testing Chromecast Discovery Caching ===\n")

    # First instance - should trigger discovery
    print("1. Creating first ChromecastManager instance...")
    start = time.time()
    cm1 = ChromecastManager()
    time1 = time.time() - start
    print(f"   Time taken: {time1:.2f}s")
    print(f"   Devices found: {len(cm1.chromecasts)}")

    # Second instance - should use cache (fast)
    print("\n2. Creating second ChromecastManager instance (should use cache)...")
    start = time.time()
    cm2 = ChromecastManager()
    time2 = time.time() - start
    print(f"   Time taken: {time2:.2f}s")
    print(f"   Devices found: {len(cm2.chromecasts)}")

    # Third instance - should use cache (fast)
    print("\n3. Creating third ChromecastManager instance (should use cache)...")
    start = time.time()
    cm3 = ChromecastManager()
    time3 = time.time() - start
    print(f"   Time taken: {time3:.2f}s")
    print(f"   Devices found: {len(cm3.chromecasts)}")

    # Force rediscovery
    print("\n4. Force rediscovery on third instance...")
    start = time.time()
    result = cm3.discover_devices(force_rediscovery=True)
    time4 = time.time() - start
    print(f"   Time taken: {time4:.2f}s")
    print(f"   Success: {result.get('success')}")
    print(f"   Devices found: {result.get('devices_found', 0)}")

    # Fourth instance after forced rediscovery - should use new cache
    print("\n5. Creating fourth ChromecastManager instance (should use updated cache)...")
    start = time.time()
    cm4 = ChromecastManager()
    time5 = time.time() - start
    print(f"   Time taken: {time5:.2f}s")
    print(f"   Devices found: {len(cm4.chromecasts)}")

    # Summary
    print("\n=== Summary ===")
    print(f"First instance (discovery): {time1:.2f}s")
    print(f"Second instance (cached): {time2:.2f}s")
    print(f"Third instance (cached): {time3:.2f}s")
    print(f"Forced rediscovery: {time4:.2f}s")
    print(f"Fourth instance (new cache): {time5:.2f}s")

    # Check if caching worked
    if time2 < time1 / 2 and time3 < time1 / 2:
        print("\n✅ CACHING IS WORKING! Subsequent instances are much faster.")
    else:
        print("\n⚠️  Caching might not be working properly.")

    return True

if __name__ == "__main__":
    test_caching()