#!/usr/bin/env python3
"""
Test script for Chromecast discovery caching mechanism
"""

import requests
import json
import time
import sys

BASE_URL = "http://localhost:5000"

def test_cache():
    print("Testing Chromecast Discovery Cache Mechanism")
    print("=" * 50)

    # Test 1: Get devices from cache (should be empty initially)
    print("\n1. Getting devices from cache (initial)...")
    response = requests.get(f"{BASE_URL}/api/get-devices")
    data = response.json()
    print(f"   - Success: {data.get('success')}")
    print(f"   - Device count: {data.get('count')}")
    print(f"   - From cache: {data.get('from_cache')}")
    print(f"   - Cache valid: {data.get('cache_valid')}")

    # Test 2: Get cache status
    print("\n2. Getting cache status...")
    response = requests.get(f"{BASE_URL}/api/cache-status")
    status = response.json()
    print(f"   - Is valid: {status.get('is_valid')}")
    print(f"   - Device count: {status.get('device_count')}")
    print(f"   - Stats: {status.get('stats')}")

    # Test 3: Discover devices with cache (default behavior)
    print("\n3. Discovering devices (using cache by default)...")
    start_time = time.time()
    response = requests.post(f"{BASE_URL}/api/discover-chromecasts",
                           json={})  # Empty json uses defaults (use_cache=True)
    elapsed = time.time() - start_time
    data = response.json()
    print(f"   - Success: {data.get('success')}")
    print(f"   - Device count: {data.get('count')}")
    print(f"   - From cache: {data.get('from_cache')}")
    print(f"   - Response time: {elapsed:.3f} seconds")

    if data.get('from_cache'):
        print(f"   - Cache age: {data.get('cache_age', 0):.1f} seconds")

    # Test 4: Immediate second request (should hit cache)
    print("\n4. Requesting again (should hit cache)...")
    start_time = time.time()
    response = requests.post(f"{BASE_URL}/api/discover-chromecasts",
                           json={})
    elapsed = time.time() - start_time
    data = response.json()
    print(f"   - Success: {data.get('success')}")
    print(f"   - Device count: {data.get('count')}")
    print(f"   - From cache: {data.get('from_cache')}")
    print(f"   - Response time: {elapsed:.3f} seconds")

    if data.get('from_cache'):
        print(f"   - Cache age: {data.get('cache_age', 0):.1f} seconds")

    # Test 5: Force refresh (bypass cache)
    print("\n5. Force refreshing (bypass cache)...")
    start_time = time.time()
    response = requests.post(f"{BASE_URL}/api/discover-chromecasts",
                           json={'force_refresh': True})
    elapsed = time.time() - start_time
    data = response.json()
    print(f"   - Success: {data.get('success')}")
    print(f"   - Device count: {data.get('count')}")
    print(f"   - From cache: {data.get('from_cache')}")
    print(f"   - Response time: {elapsed:.3f} seconds")

    # Test 6: Disable cache for request
    print("\n6. Request without cache...")
    start_time = time.time()
    response = requests.post(f"{BASE_URL}/api/discover-chromecasts",
                           json={'use_cache': False})
    elapsed = time.time() - start_time
    data = response.json()
    print(f"   - Success: {data.get('success')}")
    print(f"   - Device count: {data.get('count')}")
    print(f"   - From cache: {data.get('from_cache')}")
    print(f"   - Response time: {elapsed:.3f} seconds")

    # Test 7: Final cache status
    print("\n7. Final cache status...")
    response = requests.get(f"{BASE_URL}/api/cache-status")
    status = response.json()
    print(f"   - Is valid: {status.get('is_valid')}")
    print(f"   - Device count: {status.get('device_count')}")
    print(f"   - Age: {status.get('age_seconds', 0):.1f} seconds")
    print(f"   - Stats:")
    stats = status.get('stats', {})
    print(f"     - Hits: {stats.get('hits')}")
    print(f"     - Misses: {stats.get('misses')}")
    print(f"     - Refreshes: {stats.get('refreshes')}")

    # Test 8: Clear cache
    print("\n8. Clearing cache...")
    response = requests.post(f"{BASE_URL}/api/clear-cache")
    data = response.json()
    print(f"   - Success: {data.get('success')}")
    print(f"   - Message: {data.get('message')}")

    # Verify cache is cleared
    response = requests.get(f"{BASE_URL}/api/cache-status")
    status = response.json()
    print(f"   - Device count after clear: {status.get('device_count')}")
    print(f"   - Is valid after clear: {status.get('is_valid')}")

    print("\n" + "=" * 50)
    print("Cache testing complete!")

if __name__ == "__main__":
    try:
        test_cache()
    except requests.exceptions.ConnectionError:
        print("Error: Could not connect to the web interface")
        print("Make sure the web interface is running on http://localhost:5000")
        sys.exit(1)
    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)