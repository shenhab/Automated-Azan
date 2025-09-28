#!/usr/bin/env python3
"""Test script to verify enhanced settings functionality"""

import sys
import os
import json

# Add current directory to path to import modules
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from config_manager import ConfigManager

def test_config_manager():
    """Test ConfigManager functionality with all settings"""
    print("=== Testing Enhanced Settings Configuration ===\n")

    # Initialize ConfigManager
    config_manager = ConfigManager()
    print("✓ ConfigManager initialized successfully")

    # Test getting all current settings
    print("\n1. Testing get methods:")

    speakers_result = config_manager.get_speakers_group_name()
    print(f"   Speakers: {speakers_result}")

    location_result = config_manager.get_location()
    print(f"   Location: {location_result}")

    pre_fajr_result = config_manager.is_pre_fajr_enabled()
    print(f"   Pre-Fajr: {pre_fajr_result}")

    # Test updating settings
    print("\n2. Testing update methods:")

    # Update speakers name
    update_result = config_manager.update_setting('Settings', 'speakers-group-name', 'test-enhanced-speakers')
    print(f"   Update speakers: {update_result.get('success')}")

    # Update location
    update_result = config_manager.update_setting('Settings', 'location', 'icci')
    print(f"   Update location: {update_result.get('success')}")

    # Update pre-Fajr setting
    update_result = config_manager.update_setting('Settings', 'pre_fajr_enabled', 'False')
    print(f"   Update pre-Fajr: {update_result.get('success')}")

    # Save configuration
    print("\n3. Testing save:")
    save_result = config_manager.save_config()
    print(f"   Save config: {save_result.get('success')}")

    # Verify changes
    print("\n4. Verifying changes:")

    speakers_result = config_manager.get_speakers_group_name()
    print(f"   New speakers: {speakers_result.get('speakers_group_name')}")

    location_result = config_manager.get_location()
    print(f"   New location: {location_result.get('location')}")

    pre_fajr_result = config_manager.is_pre_fajr_enabled()
    print(f"   New pre-Fajr: {pre_fajr_result.get('pre_fajr_enabled')}")

    print("\n=== Test Results ===")

    # Simulate the enhanced settings API payload
    enhanced_config = {
        'speakers_group_name': speakers_result.get('speakers_group_name', ''),
        'location': location_result.get('location', 'naas'),
        'pre_fajr_enabled': pre_fajr_result.get('pre_fajr_enabled', True)
    }

    print(f"Enhanced config API response format:")
    print(json.dumps(enhanced_config, indent=2))

    # Reset to original values
    print("\n5. Resetting to original values:")
    config_manager.update_setting('Settings', 'speakers-group-name', 'athan')
    config_manager.update_setting('Settings', 'location', 'naas')
    config_manager.update_setting('Settings', 'pre_fajr_enabled', 'True')
    save_result = config_manager.save_config()
    print(f"   Reset complete: {save_result.get('success')}")

    return True

if __name__ == "__main__":
    test_config_manager()