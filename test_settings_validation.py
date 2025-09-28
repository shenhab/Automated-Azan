#!/usr/bin/env python3
"""Test script to validate settings API validation logic"""

import requests
import json

API_BASE = "http://192.168.86.161:5000"

def test_validation():
    """Test the enhanced settings validation"""
    print("=== Testing Settings API Validation ===\n")

    test_cases = [
        # Valid configuration
        {
            "name": "Valid configuration",
            "data": {"speakers_name": "test-speaker", "location": "naas", "pre_fajr_enabled": True},
            "expect_success": True
        },
        # Missing speaker name
        {
            "name": "Missing speaker name",
            "data": {"location": "naas", "pre_fajr_enabled": True},
            "expect_success": False
        },
        # Empty speaker name
        {
            "name": "Empty speaker name",
            "data": {"speakers_name": "", "location": "naas", "pre_fajr_enabled": True},
            "expect_success": False
        },
        # Short speaker name
        {
            "name": "Too short speaker name",
            "data": {"speakers_name": "a", "location": "naas", "pre_fajr_enabled": True},
            "expect_success": False
        },
        # Long speaker name
        {
            "name": "Too long speaker name",
            "data": {"speakers_name": "a" * 101, "location": "naas", "pre_fajr_enabled": True},
            "expect_success": False
        },
        # Invalid location
        {
            "name": "Invalid location",
            "data": {"speakers_name": "test-speaker", "location": "invalid", "pre_fajr_enabled": True},
            "expect_success": False
        },
        # Only speakers name (should work)
        {
            "name": "Only speaker name (minimal)",
            "data": {"speakers_name": "minimal-test"},
            "expect_success": True
        },
        # Boolean validation
        {
            "name": "Boolean pre-fajr setting",
            "data": {"speakers_name": "test-speaker", "location": "icci", "pre_fajr_enabled": False},
            "expect_success": True
        }
    ]

    passed = 0
    total = len(test_cases)

    for i, test_case in enumerate(test_cases, 1):
        print(f"{i}. Testing: {test_case['name']}")

        try:
            response = requests.post(
                f"{API_BASE}/api/save-config",
                headers={"Content-Type": "application/json"},
                json=test_case["data"],
                timeout=5
            )

            success = response.status_code == 200 and response.json().get("success", False)

            if success == test_case["expect_success"]:
                print(f"   ✓ PASS - Expected: {test_case['expect_success']}, Got: {success}")
                passed += 1
            else:
                print(f"   ✗ FAIL - Expected: {test_case['expect_success']}, Got: {success}")
                print(f"   Response: {response.json()}")

        except requests.exceptions.RequestException as e:
            print(f"   ✗ NETWORK ERROR - {e}")
        except Exception as e:
            print(f"   ✗ ERROR - {e}")

        print()

    print(f"=== Results: {passed}/{total} tests passed ===")
    return passed == total

if __name__ == "__main__":
    test_validation()