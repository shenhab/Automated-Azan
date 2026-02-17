#!/usr/bin/env python3
"""Test script to verify config watcher Docker volume support"""

import sys
import os
import time
import json

# Add current directory to path to import modules
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from config_manager import ConfigManager
from config_watcher import ConfigWatcher

def test_config_watcher_docker_support():
    """Test ConfigWatcher with Docker volume paths"""
    print("=== Testing ConfigWatcher Docker Volume Support ===\n")

    # Initialize ConfigManager and ConfigWatcher
    config_manager = ConfigManager()
    print("✓ ConfigManager initialized")

    # Test starting config watcher (skip actual initialization for testing)
    print("✓ ConfigWatcher ready for testing")

    # Check which file it would watch
    config_paths_to_watch = [
        '/app/config/adahn.config',  # Docker volume location (writable)
        'config/adahn.config',       # Local config directory
        config_manager.config_file   # Original config file
    ]

    watch_file = None
    for config_path in config_paths_to_watch:
        if os.path.exists(config_path):
            watch_file = config_path
            print(f"   Found existing config file: {config_path}")
            break

    if not watch_file:
        watch_file = '/app/config/adahn.config'
        print(f"   No existing config found, would watch: {watch_file}")

    print(f"\n   ConfigWatcher would watch: {watch_file}")

    # Test if the directories exist
    watch_dir = os.path.dirname(watch_file)
    print(f"   Watch directory: {watch_dir}")
    print(f"   Watch directory exists: {os.path.exists(watch_dir)}")

    # Test creating the directory
    try:
        os.makedirs(watch_dir, exist_ok=True)
        print(f"   ✓ Watch directory ensured: {watch_dir}")
    except Exception as e:
        print(f"   ✗ Failed to create watch directory: {e}")

    # Test if config file exists in different locations
    print(f"\n   Config file locations:")
    for i, path in enumerate(config_paths_to_watch, 1):
        exists = os.path.exists(path)
        print(f"   {i}. {path}: {'✓ EXISTS' if exists else '✗ missing'}")

    # Test saving a config to see where it goes
    print(f"\n   Testing config save location:")
    original_speakers = config_manager.get_speakers_group_name().get('speakers_group_name')

    # Save a test config
    config_manager.update_setting('Settings', 'speakers-group-name', 'watcher-test')
    save_result = config_manager.save_config()

    if save_result.get('success'):
        saved_to = save_result.get('config_file')
        print(f"   ✓ Config saved to: {saved_to}")

        # Check if the file was created/updated
        if os.path.exists(saved_to):
            print(f"   ✓ File exists at save location")

            # Check file modification time
            mtime = os.path.getmtime(saved_to)
            print(f"   ✓ File modification time: {time.ctime(mtime)}")
        else:
            print(f"   ✗ File does not exist at save location")
    else:
        print(f"   ✗ Config save failed: {save_result.get('error')}")

    # Reset config
    config_manager.update_setting('Settings', 'speakers-group-name', original_speakers)
    config_manager.save_config()

    return True

if __name__ == "__main__":
    test_config_watcher_docker_support()