#!/usr/bin/env python3
"""Test script to verify media URL generation fix"""

import sys
import os

# Add current directory to path to import modules
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from chromecast_manager import ChromecastManager

def test_media_url_generation():
    """Test media URL generation with Docker-aware IP detection"""
    print("=== Testing Media URL Generation ===\n")

    # Test without HOST_IP environment variable
    print("1. Testing without HOST_IP environment variable:")
    cm = ChromecastManager()

    # Test regular Athan URL
    regular_result = cm._get_media_url("media_Athan.mp3")
    print(f"   Regular Athan URL: {regular_result.get('media_url')}")
    print(f"   Detected IP: {regular_result.get('local_ip')}")
    print(f"   Success: {regular_result.get('success')}")

    # Test Fajr Athan URL
    fajr_result = cm._get_media_url("media_adhan_al_fajr.mp3")
    print(f"\n   Fajr Athan URL: {fajr_result.get('media_url')}")
    print(f"   Success: {fajr_result.get('success')}")

    # Test with HOST_IP environment variable
    print("\n2. Testing with HOST_IP environment variable:")
    os.environ['HOST_IP'] = '192.168.86.161'

    cm2 = ChromecastManager()
    host_result = cm2._get_media_url("media_Athan.mp3")
    print(f"   Media URL: {host_result.get('media_url')}")
    print(f"   Detected IP: {host_result.get('local_ip')}")
    print(f"   Success: {host_result.get('success')}")

    # Clean up
    del os.environ['HOST_IP']

    # Check if the URLs are using proper IPs (not localhost or Docker internal)
    print("\n3. Validation:")
    urls_valid = True

    for result in [regular_result, fajr_result, host_result]:
        url = result.get('media_url', '')
        if '127.0.0.1' in url or '172.' in url[:10]:
            print(f"   ⚠️  Warning: URL might not be accessible from Chromecast: {url}")
            urls_valid = False
        else:
            print(f"   ✓ URL looks good: {url}")

    if urls_valid:
        print("\n✅ All media URLs are properly configured for Chromecast access!")
    else:
        print("\n⚠️  Some URLs might not be accessible from Chromecast devices.")
        print("   Consider setting HOST_IP environment variable in docker-compose.yml")

    return urls_valid

if __name__ == "__main__":
    test_media_url_generation()