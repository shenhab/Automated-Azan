#!/usr/bin/env python3
"""Test script to verify Docker-compatible config handling"""

import sys
import os
import json

# Add current directory to path to import modules
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from config_manager import ConfigManager

def test_docker_config_handling():
    """Test ConfigManager with Docker volume paths"""
    print("=== Testing Docker-Compatible Configuration Handling ===\n")

    # Initialize ConfigManager
    config_manager = ConfigManager()
    print("✓ ConfigManager initialized")

    # Test loading configuration
    print("\n1. Testing configuration loading:")
    load_result = config_manager.load_config()
    print(f"   Load result: {load_result.get('success')}")
    if load_result.get('success'):
        print(f"   Loaded from: {load_result.get('config_file')}")

    # Test getting current settings
    print("\n2. Testing get methods:")
    speakers_result = config_manager.get_speakers_group_name()
    location_result = config_manager.get_location()
    pre_fajr_result = config_manager.is_pre_fajr_enabled()

    print(f"   Speakers: {speakers_result.get('speakers_group_name')}")
    print(f"   Location: {location_result.get('location')}")
    print(f"   Pre-Fajr: {pre_fajr_result.get('pre_fajr_enabled')}")

    # Test updating and saving configuration
    print("\n3. Testing configuration updates:")

    # Update settings
    update1 = config_manager.update_setting('Settings', 'speakers-group-name', 'docker-test-speaker')
    update2 = config_manager.update_setting('Settings', 'location', 'icci')
    update3 = config_manager.update_setting('Settings', 'pre_fajr_enabled', 'False')

    print(f"   Update speakers: {update1.get('success')}")
    print(f"   Update location: {update2.get('success')}")
    print(f"   Update pre-Fajr: {update3.get('success')}")

    # Test save with Docker volume support
    print("\n4. Testing Docker-compatible save:")
    save_result = config_manager.save_config()
    print(f"   Save result: {save_result.get('success')}")
    if save_result.get('success'):
        print(f"   Saved to: {save_result.get('config_file')}")
    else:
        print(f"   Save error: {save_result.get('error')}")

    # Verify changes were saved
    print("\n5. Verifying saved changes:")
    new_config_manager = ConfigManager()
    new_config_manager.load_config()

    new_speakers = new_config_manager.get_speakers_group_name()
    new_location = new_config_manager.get_location()
    new_pre_fajr = new_config_manager.is_pre_fajr_enabled()

    print(f"   New speakers: {new_speakers.get('speakers_group_name')}")
    print(f"   New location: {new_location.get('location')}")
    print(f"   New pre-Fajr: {new_pre_fajr.get('pre_fajr_enabled')}")

    # Reset to original values
    print("\n6. Resetting to original values:")
    config_manager.update_setting('Settings', 'speakers-group-name', 'athan')
    config_manager.update_setting('Settings', 'location', 'naas')
    config_manager.update_setting('Settings', 'pre_fajr_enabled', 'True')
    reset_result = config_manager.save_config()
    print(f"   Reset complete: {reset_result.get('success')}")

    return save_result.get('success', False)

if __name__ == "__main__":
    success = test_docker_config_handling()
    if success:
        print("\n✅ Docker configuration handling works correctly!")
    else:
        print("\n❌ Docker configuration handling has issues!")
    exit(0 if success else 1)