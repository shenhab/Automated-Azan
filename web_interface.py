#!/usr/bin/env python3
"""
Web Interface for Automated Azan
Provides a web-based configuration and management interface
"""

import os
import json
import time
import logging
import configparser

import shutil
from datetime import datetime
from flask import Flask, render_template, request, jsonify, redirect, url_for, flash, send_from_directory
from flask_socketio import SocketIO, emit
import pychromecast
from prayer_times_fetcher import PrayerTimesFetcher
from chromecast_manager import ChromecastManager
from config_manager import ConfigManager
import threading

app = Flask(__name__)
app.secret_key = 'your-secret-key-change-this'  # Change this in production
socketio = SocketIO(app, cors_allowed_origins="*", allow_unsafe_werkzeug=True)

# Global variables for real-time updates
current_config = {}
prayer_times = {}
prayer_times_last_updated = None
scheduler_status = {"running": False, "next_prayer": None, "last_update": None}

# Global references to shared instances
web_scheduler = None
web_config_watcher = None

# Cache infrastructure for discovered devices
discovered_devices_cache = {
    'timestamp': None,
    'ttl': 300,  # 5 minutes cache TTL
    'is_discovering': False,
    'stats': {'hits': 0, 'misses': 0, 'refreshes': 0}
}
cache_lock = threading.Lock()

def get_discovered_devices():
    """Get devices directly from ChromecastManager (single source of truth)"""
    if not web_cast_manager or not web_cast_manager.chromecasts:
        return []

    devices = []
    for uuid, cast_info in web_cast_manager.chromecasts.items():
        devices.append({
            'name': cast_info['name'],
            'uuid': uuid,
            'model': cast_info['model_name'],
            'model_name': cast_info['model_name'],  # Include both for compatibility
            'host': cast_info['host'],
            'port': cast_info['port'],
            'cast_type': cast_info.get('manufacturer', 'Unknown'),
            'status': 'Available'
        })
    return devices

def get_speaker_status():
    """Get the status of the configured speaker"""
    try:
        if not current_config.get('speakers_group_name'):
            return {"status": "Not Configured", "message": "No speaker configured"}

        configured_speaker = current_config['speakers_group_name']
        devices = get_discovered_devices()

        # Check if configured speaker is found in discovered devices
        for device in devices:
            if device['name'] == configured_speaker:
                return {
                    "status": "Available",
                    "message": f"Found: {device['name']} ({device['model']})",
                    "device": device
                }

        # Speaker not found
        return {
            "status": "Not Found",
            "message": f"'{configured_speaker}' not discovered",
            "configured_name": configured_speaker
        }

    except Exception as e:
        logging.error(f"Error getting speaker status: {e}")
        return {"status": "Error", "message": f"Error checking speaker: {str(e)}"}


# Context processor to make config and prayer times available to all templates
@app.context_processor
def inject_global_vars():
    """Make config and prayer times available to all templates"""
    # Ensure config is loaded
    if not current_config:
        load_config()
    
    # Get current prayer times
    current_prayer_times = prayer_times if prayer_times else {}
    
    # Calculate next prayer for sidebar
    next_prayer_info = get_next_prayer_info()
    
    return {
        'config': current_config,
        'prayer_times': current_prayer_times,
        'next_prayer_info': next_prayer_info
    }

def get_next_prayer_info():
    """Get information about the next prayer"""
    if not prayer_times or 'error' in prayer_times:
        return {'name': 'Unknown', 'time': 'Loading...'}
    
    try:
        from datetime import datetime
        now = datetime.now()
        current_time = now.strftime("%H:%M")
        
        # Prayer order for comparison
        prayer_order = ['Fajr', 'Dhuhr', 'Asr', 'Maghrib', 'Isha']
        
        for prayer in prayer_order:
            if prayer in prayer_times:
                prayer_time = prayer_times[prayer]
                if current_time < prayer_time:
                    return {'name': prayer, 'time': prayer_time}
        
        # If no prayer found for today, next is Fajr tomorrow
        return {'name': 'Fajr', 'time': prayer_times.get('Fajr', 'Unknown') + ' (Tomorrow)'}
        
    except Exception as e:
        logging.debug(f"Error calculating next prayer: {e}")
        return {'name': 'Unknown', 'time': 'Error'}

# Chromecast manager will be set by start_web_interface()
web_cast_manager = None

def load_config():
    """Load current configuration using ConfigManager"""
    global current_config

    try:
        config_manager = ConfigManager()

        # Get all settings using ConfigManager
        speakers_result = config_manager.get_speakers_group_name()
        location_result = config_manager.get_location()
        pre_fajr_result = config_manager.is_pre_fajr_enabled()

        current_config = {
            'speakers_group_name': speakers_result.get('speakers_group_name', 'athan'),
            'location': location_result.get('location', 'naas'),
            'pre_fajr_enabled': pre_fajr_result.get('pre_fajr_enabled', True)
        }

        logging.debug("Configuration loaded successfully via ConfigManager")

    except Exception as e:
        logging.error(f"Error loading config via ConfigManager: {e}")
        # Fallback to default configuration
        current_config = {
            'speakers_group_name': 'athan',
            'location': 'naas',
            'pre_fajr_enabled': True
        }
        logging.info("Using default configuration")
    
    return current_config

def save_config(speakers_name, location):
    """Save configuration to file with Docker volume support"""
    config = configparser.ConfigParser()
    config.add_section('Settings')
    config.set('Settings', 'speakers-group-name', speakers_name)
    config.set('Settings', 'location', location)
    
    # Try to save to different possible locations
    config_paths = [
        '/app/config/adahn.config',  # Docker volume location
        'config/adahn.config',       # Local config directory
        'adahn.config'               # Default location
    ]
    
    saved = False
    for config_path in config_paths:
        try:
            # Create directory if it doesn't exist
            config_dir = os.path.dirname(config_path)
            if config_dir and not os.path.exists(config_dir):
                os.makedirs(config_dir, exist_ok=True)
                
            with open(config_path, 'w') as config_file:
                config.write(config_file)
            logging.info(f"Configuration saved to {config_path}")
            saved = True
            break
        except (PermissionError, OSError) as e:
            logging.debug(f"Cannot write to {config_path}: {e}")
            continue
    
    if not saved:
        raise PermissionError("Cannot write configuration file - all locations read-only")
    
    load_config()  # Reload to update global variable

def load_prayer_times(force_reload=False):
    """Load current prayer times with caching"""
    global prayer_times, prayer_times_last_updated
    
    # Check if we need to reload prayer times
    # Only reload if forced, if we haven't loaded them yet, or if it's been more than 1 hour
    now = datetime.now()
    need_reload = (force_reload or 
                   prayer_times_last_updated is None or 
                   not prayer_times or 
                   'error' in prayer_times or
                   (now - prayer_times_last_updated).total_seconds() > 3600)  # 1 hour cache
    
    if not need_reload:
        logging.debug("Using cached prayer times")
        return prayer_times
    
    try:
        logging.debug("Loading fresh prayer times from source")
        fetcher = PrayerTimesFetcher()
        location = current_config.get('location', 'naas')
        times = fetcher.fetch_prayer_times(location)
        
        if isinstance(times, dict) and 'error' not in times:
            # Extract just the prayer times from the response
            if 'prayer_times' in times:
                prayer_times = times['prayer_times']
            else:
                prayer_times = times
            prayer_times_last_updated = now
            logging.info(f"Prayer times successfully loaded and cached for {location}")
        else:
            prayer_times = {'error': 'Failed to load prayer times'}
            logging.warning(f"Failed to load prayer times: {times}")
            
    except Exception as e:
        logging.error(f"Error loading prayer times: {e}")
        prayer_times = {'error': str(e)}
    
    return prayer_times

# Routes
@app.route('/')
def index():
    """Main dashboard"""
    load_config()

    # Get prayer times from scheduler if available, otherwise fallback
    dashboard_prayer_times = {}
    if web_scheduler:
        try:
            scheduler_result = web_scheduler.get_prayer_times()
            if scheduler_result.get('success'):
                dashboard_prayer_times = scheduler_result['prayer_times']
                logging.info(f"[DEBUG] Dashboard using scheduler prayer times: {dashboard_prayer_times}")
            else:
                logging.warning(f"Scheduler prayer times failed, using fallback: {scheduler_result.get('error')}")
                load_prayer_times()
                dashboard_prayer_times = prayer_times
        except Exception as e:
            logging.error(f"Error getting scheduler prayer times: {e}")
            load_prayer_times()
            dashboard_prayer_times = prayer_times
    else:
        logging.warning("[DEBUG] web_scheduler not available, using fallback")
        load_prayer_times()
        dashboard_prayer_times = prayer_times

    # Debug logging for scheduler status
    logging.info(f"[DEBUG] Dashboard loading - scheduler_status: {scheduler_status}")

    # Try to get actual scheduler status from the scheduler if available
    actual_status = {"running": False, "next_prayer": None, "last_update": None}
    if web_scheduler:
        try:
            status_data = web_scheduler.get_scheduler_status()
            logging.info(f"[DEBUG] Raw scheduler status data: {status_data}")

            # Check if scheduler has jobs and is successful
            has_jobs = status_data.get("total_jobs", 0) > 0
            is_successful = status_data.get("success", False)
            actual_status["running"] = is_successful and has_jobs
            actual_status["next_prayer"] = status_data.get("next_run")
            actual_status["last_update"] = datetime.now().isoformat()

            logging.info(f"[DEBUG] Processed scheduler status - running: {actual_status['running']}, jobs: {status_data.get('total_jobs', 0)}, success: {is_successful}")
        except Exception as e:
            logging.error(f"[DEBUG] Error getting scheduler status: {e}")
    else:
        logging.warning("[DEBUG] web_scheduler is not available")

    # Get speaker status
    speaker_status = get_speaker_status()
    logging.info(f"[DEBUG] Speaker status: {speaker_status}")

    return render_template('dashboard.html',
                         config=current_config,
                         prayer_times=dashboard_prayer_times,
                         scheduler_status=actual_status,
                         speaker_status=speaker_status)

@app.route('/chromecasts')
def chromecasts():
    """Chromecast management page"""
    load_config()
    return render_template('chromecasts.html',
                         config=current_config,
                         devices=get_discovered_devices())

@app.route('/settings')
def settings():
    """Settings page"""
    load_config()
    return render_template('settings.html', config=current_config)

@app.route('/logs')
def logs():
    """Logs viewer page"""

    load_config()
    return render_template('logs.html', config=current_config)

@app.route('/test')
def test():
    """Test Adhan page"""
    load_config()
    return render_template('test.html', config=current_config)

@app.route('/api/prayer-times')
def api_prayer_times():
    """API endpoint to get prayer times from scheduler"""
    try:
        # Get prayer times from the scheduler if available
        if web_scheduler:
            # Use scheduler's prayer times (the authoritative source)
            scheduler_result = web_scheduler.get_prayer_times()
            if scheduler_result.get('success'):
                return jsonify({
                    'success': True,
                    'prayer_times': scheduler_result['prayer_times'],
                    'location': scheduler_result['location'],
                    'source': 'scheduler',
                    'timestamp': scheduler_result.get('timestamp')
                })
            else:
                logging.warning(f"Scheduler prayer times failed: {scheduler_result.get('error')}")

        # Fallback to independent calculation if scheduler is unavailable
        config = load_config()
        location = config.get('location', 'naas')
        times = load_prayer_times()

        return jsonify({
            'success': True,
            'prayer_times': times,
            'location': location,
            'source': 'fallback'
        })
    except Exception as e:
        logging.error(f"Error fetching prayer times: {e}")
        return jsonify({'success': False, 'error': str(e)}), 500

@app.route('/media/<filename>')
def serve_media(filename):
    """Serve media files (Adhan audio files)"""
    try:
        # Security: only allow specific media files
        allowed_files = ['media_Athan.mp3', 'media_adhan_al_fajr.mp3']
        
        if filename not in allowed_files:
            logging.warning(f"Unauthorized media file request: {filename}")
            return "File not found", 404
            
        media_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'Media')
        
        if not os.path.exists(os.path.join(media_dir, filename)):
            logging.error(f"Media file not found: {filename}")
            return "File not found", 404
            
        logging.info(f"Serving media file: {filename}")
        response = send_from_directory(media_dir, filename, mimetype='audio/mpeg')
        # Add headers for Chromecast compatibility - match working external server
        response.headers['Accept-Ranges'] = 'bytes'
        response.headers['Access-Control-Allow-Origin'] = '*'
        response.headers['Access-Control-Allow-Methods'] = 'GET, HEAD, OPTIONS'
        response.headers['Access-Control-Allow-Headers'] = 'Range'
        response.headers['Cache-Control'] = 'max-age=172800'
        # Remove the problematic cache-control from Flask
        response.headers.pop('Cache-Control', None)
        response.headers['Cache-Control'] = 'max-age=172800'
        return response
        
    except Exception as e:
        logging.error(f"Error serving media file {filename}: {e}")
        return "Internal server error", 500

# Cache management functions
def cache_is_valid():
    """Check if the discovered devices cache is still valid"""
    with cache_lock:
        devices = get_discovered_devices()
        if not devices or discovered_devices_cache['timestamp'] is None:
            return False

        age = time.time() - discovered_devices_cache['timestamp']
        return age < discovered_devices_cache['ttl']

def update_discovered_devices_cache():
    """Update the cache timestamp and emit updates to clients"""
    with cache_lock:
        devices = get_discovered_devices()
        discovered_devices_cache['timestamp'] = time.time()
        discovered_devices_cache['stats']['refreshes'] += 1

        # Emit update to connected clients
        socketio.emit('chromecasts_discovered', {'devices': devices})
        logging.info(f"Cache updated with {len(devices)} devices")
        return len(devices) > 0

def get_cache_status():
    """Get current cache status and statistics"""
    with cache_lock:
        age = None
        if discovered_devices_cache['timestamp']:
            age = time.time() - discovered_devices_cache['timestamp']

        devices = get_discovered_devices()
        return {
            'is_valid': cache_is_valid(),
            'device_count': len(devices),
            'age_seconds': age,
            'ttl': discovered_devices_cache['ttl'],
            'stats': discovered_devices_cache['stats'].copy(),
            'is_discovering': discovered_devices_cache['is_discovering']
        }

# API Routes
@app.route('/api/discover-chromecasts', methods=['POST'])
def api_discover_chromecasts():
    """API endpoint to get discovered Chromecast devices (read-only - no discovery)"""
    try:
        # Simply return the devices already discovered by the scheduler
        devices = get_discovered_devices()

        logging.debug(f"Returning {len(devices)} devices from ChromecastManager")

        return jsonify({
            'success': True,
            'devices': devices,
            'count': len(devices),
            'from_scheduler': True,
            'message': 'Devices discovered by scheduler' if devices else 'No devices found by scheduler yet'
        })

    except Exception as e:
        logging.error(f"Error getting devices: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500

@app.route('/api/cache-status', methods=['GET'])
def api_cache_status():
    """API endpoint to get cache status and statistics"""
    try:
        status = get_cache_status()
        return jsonify({
            'success': True,
            **status
        })
    except Exception as e:
        logging.error(f"Error getting cache status: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500

@app.route('/api/clear-cache', methods=['POST'])
def api_clear_cache():
    """API endpoint to clear the device cache"""
    try:
        with cache_lock:
            # Clear ChromecastManager's cache as the source of truth
            if web_cast_manager:
                web_cast_manager.chromecasts.clear()
            discovered_devices_cache['timestamp'] = None
            discovered_devices_cache['stats']['refreshes'] = 0
            logging.info("Cache cleared manually")

        return jsonify({
            'success': True,
            'message': 'Cache cleared successfully'
        })
    except Exception as e:
        logging.error(f"Error clearing cache: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500

@app.route('/api/get-devices', methods=['GET'])
def api_get_devices():
    """API endpoint to get devices from cache (always immediate response)"""
    try:
        # Always return from cache for immediate response
        with cache_lock:
            discovered_devices_cache['stats']['hits'] += 1
            cache_age = None
            if discovered_devices_cache['timestamp']:
                cache_age = time.time() - discovered_devices_cache['timestamp']

        devices = get_discovered_devices()
        return jsonify({
            'success': True,
            'devices': devices,
            'count': len(devices),
            'from_cache': True,
            'cache_valid': cache_is_valid(),
            'cache_age': cache_age
        })
    except Exception as e:
        logging.error(f"Error getting devices: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500

@app.route('/api/test-chromecast', methods=['POST'])
def api_test_chromecast():
    """API endpoint to test Chromecast connection"""
    data = request.json
    device_name = data.get('device_name', '')
    
    if not device_name:
        return jsonify({'success': False, 'error': 'Device name required'}), 400
    
    try:
        # Use ChromecastManager to test connection
        web_cast_manager.discover_devices(force_rediscovery=True)
        
        if web_cast_manager.chromecasts:
            # Look for device by name
            target_device = None
            for uuid, cast_info in web_cast_manager.chromecasts.items():
                if cast_info['name'] == device_name:
                    target_device = cast_info
                    break
            
            if target_device:
                return jsonify({
                    'success': True, 
                    'message': f'Successfully found {device_name}',
                    'device_info': {
                        'name': target_device['name'],
                        'host': target_device['host'],
                        'port': target_device['port'],
                        'model': target_device['model_name']
                    }
                })
            else:
                return jsonify({
                    'success': False, 
                    'error': f'Device "{device_name}" not found'
                })
        else:
            return jsonify({
                'success': False, 
                'error': 'No devices discovered'
            })
            
    except Exception as e:
        logging.error(f"Error testing connection to {device_name}: {e}")
        return jsonify({'success': False, 'error': str(e)})

@app.route('/api/config/reload', methods=['POST'])
def reload_config():
    """Manual config reload endpoint"""
    try:
        from config_manager import ConfigManager
        config_manager = ConfigManager()
        result = config_manager.reload_config()

        if result.get('success'):
            # Notify via WebSocket
            socketio.emit('config_reloaded', {
                'message': 'Configuration reloaded successfully',
                'timestamp': datetime.now().isoformat()
            }, broadcast=True)

            return jsonify({
                "success": True,
                "message": "Configuration reloaded successfully",
                "timestamp": datetime.now().isoformat()
            })
        else:
            return jsonify({
                "success": False,
                "error": result.get('error', 'Failed to reload configuration'),
                "timestamp": datetime.now().isoformat()
            }), 500
    except Exception as e:
        logging.error(f"Error reloading config: {e}")
        return jsonify({
            "success": False,
            "error": str(e),
            "timestamp": datetime.now().isoformat()
        }), 500

@app.route('/api/config/watcher/status')
def watcher_status():
    """Get config watcher status"""
    try:
        if web_config_watcher:
            status = web_config_watcher.get_status()
            return jsonify(status)
        else:
            return jsonify({
                "success": True,
                "running": False,
                "message": "Config watcher not initialized",
                "timestamp": datetime.now().isoformat()
            })
    except Exception as e:
        logging.error(f"Error getting watcher status: {e}")
        return jsonify({
            "success": False,
            "error": str(e),
            "timestamp": datetime.now().isoformat()
        }), 500

@app.route('/api/config/watcher/restart', methods=['POST'])
def restart_watcher():
    """Restart the config watcher"""
    try:
        if web_config_watcher:
            # Stop if running
            web_config_watcher.stop()

            # Start again
            result = web_config_watcher.start()

            return jsonify(result)
        else:
            return jsonify({
                "success": False,
                "error": "Config watcher not initialized",
                "timestamp": datetime.now().isoformat()
            }), 400
    except Exception as e:
        logging.error(f"Error restarting watcher: {e}")
        return jsonify({
            "success": False,
            "error": str(e),
            "timestamp": datetime.now().isoformat()
        }), 500

@app.route('/api/config/pre-fajr', methods=['POST'])
def toggle_pre_fajr():
    """Toggle pre-Fajr Quran feature"""
    try:
        data = request.json
        enable = data.get('enable', False)

        if web_scheduler and hasattr(web_scheduler, 'toggle_pre_fajr_quran'):
            result = web_scheduler.toggle_pre_fajr_quran(enable)

            # Update config file
            from config_manager import ConfigManager
            config_manager = ConfigManager()
            config_manager.update_setting('Settings', 'pre_fajr_enabled', str(enable))
            config_manager.save_config()

            # Notify via WebSocket
            socketio.emit('pre_fajr_toggled', {
                'enabled': enable,
                'timestamp': datetime.now().isoformat()
            }, broadcast=True)

            return jsonify(result)
        else:
            return jsonify({
                "success": False,
                "error": "Scheduler not available or feature not supported",
                "timestamp": datetime.now().isoformat()
            }), 400
    except Exception as e:
        logging.error(f"Error toggling pre-Fajr: {e}")
        return jsonify({
            "success": False,
            "error": str(e),
            "timestamp": datetime.now().isoformat()
        }), 500

@app.route('/api/save-config', methods=['POST'])
def api_save_config():
    """API endpoint to save configuration - supports all config options"""
    try:
        data = request.json

        # Initialize config manager
        config_manager = ConfigManager()

        # Validate required fields
        speakers_name = data.get('speakers_name', '').strip()
        if not speakers_name:
            return jsonify({'success': False, 'error': 'Speaker name is required'}), 400

        # Validate speakers name format
        if len(speakers_name) < 2:
            return jsonify({'success': False, 'error': 'Speaker name must be at least 2 characters long'}), 400

        if len(speakers_name) > 100:
            return jsonify({'success': False, 'error': 'Speaker name cannot exceed 100 characters'}), 400

        # Update all provided settings
        settings_updated = []

        # Update speakers group name
        result = config_manager.update_setting('Settings', 'speakers-group-name', speakers_name)
        if not result['success']:
            return jsonify({'success': False, 'error': f"Failed to update speakers name: {result.get('error')}"}), 500
        settings_updated.append('speakers-group-name')

        # Update location if provided
        location = data.get('location', '').strip()
        if location:
            # Validate location value
            valid_locations = ['naas', 'icci']
            if location not in valid_locations:
                return jsonify({'success': False, 'error': f'Invalid location. Must be one of: {", ".join(valid_locations)}'}), 400

            result = config_manager.update_setting('Settings', 'location', location)
            if not result['success']:
                return jsonify({'success': False, 'error': f"Failed to update location: {result.get('error')}"}), 500
            settings_updated.append('location')

        # Update pre-Fajr setting if provided
        if 'pre_fajr_enabled' in data:
            pre_fajr = data['pre_fajr_enabled']
            # Convert boolean to string for config file
            pre_fajr_value = 'True' if pre_fajr else 'False'
            result = config_manager.update_setting('Settings', 'pre_fajr_enabled', pre_fajr_value)
            if not result['success']:
                return jsonify({'success': False, 'error': f"Failed to update pre-Fajr setting: {result.get('error')}"}), 500
            settings_updated.append('pre_fajr_enabled')

        # Save configuration to file
        save_result = config_manager.save_config()
        if not save_result['success']:
            return jsonify({'success': False, 'error': f"Failed to save configuration: {save_result.get('error')}"}), 500

        # Update global current_config for backward compatibility
        global current_config
        current_config = load_config()

        # Emit real-time update
        socketio.emit('config_updated', {'config': current_config})

        return jsonify({
            'success': True,
            'message': f'Configuration saved successfully ({len(settings_updated)} settings updated)',
            'settings_updated': settings_updated,
            'config': current_config
        })

    except Exception as e:
        logging.error(f"Error saving config: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500


@app.route('/api/get-config', methods=['GET'])
def api_get_config():
    """API endpoint to get all configuration settings"""
    try:
        config_manager = ConfigManager()

        # Get all current settings
        speakers_result = config_manager.get_speakers_group_name()
        location_result = config_manager.get_location()
        pre_fajr_result = config_manager.is_pre_fajr_enabled()

        config_data = {
            'speakers_group_name': speakers_result.get('speakers_group_name', ''),
            'location': location_result.get('location', 'naas'),
            'pre_fajr_enabled': pre_fajr_result.get('pre_fajr_enabled', True)
        }

        return jsonify({
            'success': True,
            'config': config_data,
            'timestamp': datetime.now().isoformat()
        })

    except Exception as e:
        logging.error(f"Error getting config: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500


@app.route('/api/refresh-prayer-times', methods=['POST'])
def api_refresh_prayer_times():
    """API endpoint to refresh prayer times using scheduler"""
    try:
        # Use scheduler's refresh if available
        if web_scheduler:
            refresh_result = web_scheduler.refresh_schedule()
            if refresh_result.get('success'):
                prayer_times_data = refresh_result.get('prayer_times', {})

                # Emit real-time update using scheduler's data
                socketio.emit('prayer_times_updated', {'prayer_times': prayer_times_data})

                return jsonify({
                    'success': True,
                    'prayer_times': prayer_times_data,
                    'source': 'scheduler'
                })
            else:
                logging.warning(f"Scheduler refresh failed: {refresh_result.get('error')}")

        # Fallback to independent refresh
        load_prayer_times(force_reload=True)
        socketio.emit('prayer_times_updated', {'prayer_times': prayer_times})

        return jsonify({
            'success': True,
            'prayer_times': prayer_times,
            'source': 'fallback'
        })

    except Exception as e:
        logging.error(f"Error refreshing prayer times: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500

@app.route('/api/logs', methods=['GET'])
def api_logs():
    """API endpoint to get recent logs"""
    try:
        log_lines = []
        log_files = ['/var/log/azan_service.log', 'logs/azan_service.log', 'azan_service.log']
        
        for log_file in log_files:
            if os.path.exists(log_file):
                with open(log_file, 'r') as f:
                    lines = f.readlines()
                    log_lines = lines[-100:]  # Get last 100 lines
                break
        
        return jsonify({
            'success': True,
            'logs': ''.join(log_lines)
        })
        
    except Exception as e:
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500

      
@app.route('/api/test-adhan', methods=['POST'])
def api_test_adhan():
    """API endpoint to test Adhan playback"""
    try:
        adhan_type = request.json.get('type', 'regular') if request.json else 'regular'
        
        if not web_cast_manager:
            return jsonify({
                'success': False,
                'error': 'ChromecastManager not available'
            }), 500
        
        if adhan_type == 'fajr':
            result = web_cast_manager.start_adahn_alfajr()
            media_type = 'Fajr Adhan'
        else:
            result = web_cast_manager.start_adahn()
            media_type = 'Regular Adhan'

        # Extract success from the result dictionary
        success = result.get('success', False) if isinstance(result, dict) else bool(result)

        return jsonify({
            'success': success,
            'message': f'{media_type} playback {"started" if success else "failed"}',
            'result': result  # Include full result for debugging
        })
        
    except Exception as e:
        logging.error(f"Error testing Adhan: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500

@app.route('/api/media-info', methods=['GET'])
def api_media_info():
    """API endpoint to get media file information"""
    try:
        media_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'Media')
        media_files = []
        
        for filename in ['media_Athan.mp3', 'media_adhan_al_fajr.mp3']:
            file_path = os.path.join(media_dir, filename)
            if os.path.exists(file_path):
                file_stats = os.stat(file_path)
                media_files.append({
                    'filename': filename,
                    'size': file_stats.st_size,
                    'url': f'/media/{filename}',
                    'exists': True
                })
            else:
                media_files.append({
                    'filename': filename,
                    'exists': False
                })
        
        return jsonify({
            'success': True,
            'media_files': media_files
        })
        
    except Exception as e:
        logging.error(f"Error getting media info: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500

@app.route('/api/test-device-adhan', methods=['POST'])
def api_test_device_adhan():
    """API endpoint to test Adhan playback on specific device"""
    try:
        data = request.json or {}
        device_id = data.get('device_id')
        device_name = data.get('device_name', 'Unknown Device')
        adhan_type = data.get('type', 'regular')
        
        if not device_id:
            return jsonify({
                'success': False,
                'error': 'Device ID is required'
            }), 400
        
        if not web_cast_manager:
            return jsonify({
                'success': False,
                'error': 'ChromecastManager not available'
            }), 500
        
        # Create a temporary cast device for this specific device
        cast_info = None
        for uuid, info in web_cast_manager.chromecasts.items():
            if uuid == device_id:
                cast_info = info
                break
        
        if not cast_info:
            return jsonify({
                'success': False,
                'error': f'Device {device_name} not found'
            }), 404
        
        # Create the cast device
        cast_device = web_cast_manager._create_cast_device(cast_info)
        if not cast_device:
            return jsonify({
                'success': False,
                'error': f'Failed to connect to {device_name}'
            }), 500
        
        # Temporarily override the target device for this test
        original_target = getattr(web_cast_manager, 'target_device', None)
        web_cast_manager.target_device = cast_device

        # Use dedicated Adhan functions for better handling
        if adhan_type == 'fajr':
            media_type = 'Fajr Adhan'
            logging.info(f"Testing {media_type} on specific device: {device_name}")
            result = web_cast_manager.start_adahn_alfajr()
        else:
            media_type = 'Regular Adhan'
            logging.info(f"Testing {media_type} on specific device: {device_name}")
            result = web_cast_manager.start_adahn()

        # Restore original target device
        web_cast_manager.target_device = original_target

        # Extract success from the result dictionary
        success = result.get('success', False) if isinstance(result, dict) else bool(result)

        return jsonify({
            'success': success,
            'message': f'{media_type} {"started" if success else "failed"} on {device_name}',
            'result': result  # Include full result for debugging
        })
        
    except Exception as e:
        logging.error(f"Error testing device Adhan: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500

@app.route('/api/stop-device-playback', methods=['POST'])
def api_stop_device_playback():
    """API endpoint to stop playback on specific device"""
    try:
        data = request.json or {}
        device_id = data.get('device_id')
        device_name = data.get('device_name', 'Unknown Device')
        
        if not device_id:
            return jsonify({
                'success': False,
                'error': 'Device ID is required'
            }), 400
        
        if not web_cast_manager:
            return jsonify({
                'success': False,
                'error': 'ChromecastManager not available'
            }), 500
        
        # Find the device
        cast_info = None
        for uuid, info in web_cast_manager.chromecasts.items():
            if uuid == device_id:
                cast_info = info
                break
        
        if not cast_info:
            return jsonify({
                'success': False,
                'error': f'Device {device_name} not found'
            }), 404
        
        # Create the cast device
        cast_device = web_cast_manager._create_cast_device(cast_info)
        if not cast_device:
            return jsonify({
                'success': False,
                'error': f'Failed to connect to {device_name}'
            }), 500
        
        try:
            # Connect to device
            cast_device.wait(timeout=10)
            
            # Stop playback
            media_controller = cast_device.media_controller
            
            # Check if there's an active session
            try:
                media_controller.update_status()
                time.sleep(0.5)  # Give time for status update
                
                if media_controller.status.content_id:
                    media_controller.stop()
                    logging.info(f"Stopped active playback on {device_name}")
                    return jsonify({
                        'success': True,
                        'message': f'Playback stopped on {device_name}'
                    })
                else:
                    logging.info(f"No active playback found on {device_name}")
                    return jsonify({
                        'success': True,
                        'message': f'No active playback on {device_name}'
                    })
                    
            except Exception as stop_error:
                # Even if stop fails, consider it successful if there was no active session
                if "no session is active" in str(stop_error).lower():
                    return jsonify({
                        'success': True,
                        'message': f'No active session on {device_name}'
                    })
                else:
                    raise stop_error
            
        except Exception as e:
            logging.error(f"Error stopping playback on {device_name}: {e}")
            return jsonify({
                'success': False,
                'error': f'Failed to stop playback: {str(e)}'
            }), 500
        
    except Exception as e:
        logging.error(f"Error stopping device playback: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500

@app.route('/api/reset-playback-state', methods=['POST'])
def api_reset_playback_state():
    """Manually reset the internal playback state tracking"""
    try:
        logging.info("Manual playback state reset requested")

        if not web_cast_manager:
            return jsonify({
                'success': False,
                'error': 'ChromecastManager not available'
            }), 500

        # Reset playback state
        with web_cast_manager.playback_lock:
            was_playing = web_cast_manager.athan_playing
            elapsed_time = time.time() - web_cast_manager.athan_start_time if web_cast_manager.athan_start_time else 0

            web_cast_manager.athan_playing = False
            web_cast_manager.athan_start_time = None

            logging.info(f"Playback state reset - was_playing: {was_playing}, elapsed_time: {elapsed_time:.1f}s")

            return jsonify({
                'success': True,
                'message': 'Playback state has been reset',
                'was_playing': was_playing,
                'elapsed_time': round(elapsed_time, 1),
                'timestamp': datetime.now().isoformat()
            })

    except Exception as e:
        logging.error(f"Error resetting playback state: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500

# WebSocket events
@socketio.on('connect')
def handle_connect():
    """Handle client connection"""
    emit('connected', {'data': 'Connected to Automated Azan Web Interface'})

@socketio.on('request_status')
def handle_status_request():
    """Handle status request from client"""
    load_config()

    # Get prayer times from scheduler if available, otherwise fallback
    websocket_prayer_times = {}
    if web_scheduler:
        try:
            scheduler_result = web_scheduler.get_prayer_times()
            if scheduler_result.get('success'):
                websocket_prayer_times = scheduler_result['prayer_times']
                logging.info(f"[DEBUG] WebSocket using scheduler prayer times: {websocket_prayer_times}")
            else:
                logging.warning(f"WebSocket: Scheduler prayer times failed, using fallback: {scheduler_result.get('error')}")
                load_prayer_times()
                websocket_prayer_times = prayer_times
        except Exception as e:
            logging.error(f"WebSocket: Error getting scheduler prayer times: {e}")
            load_prayer_times()
            websocket_prayer_times = prayer_times
    else:
        logging.warning("[DEBUG] WebSocket: web_scheduler not available, using fallback")
        load_prayer_times()
        websocket_prayer_times = prayer_times

    # Get actual scheduler status
    actual_status = {"running": False, "next_prayer": None, "last_update": None}
    if web_scheduler:
        try:
            status_data = web_scheduler.get_scheduler_status()
            logging.info(f"[DEBUG] WebSocket raw scheduler status data: {status_data}")

            # Check if scheduler has jobs and is successful
            has_jobs = status_data.get("total_jobs", 0) > 0
            is_successful = status_data.get("success", False)
            actual_status["running"] = is_successful and has_jobs
            actual_status["next_prayer"] = status_data.get("next_run")
            actual_status["last_update"] = datetime.now().isoformat()

            logging.info(f"[DEBUG] WebSocket processed scheduler status - running: {actual_status['running']}, jobs: {status_data.get('total_jobs', 0)}")
        except Exception as e:
            logging.error(f"[DEBUG] WebSocket error getting scheduler status: {e}")

    # Get speaker status
    speaker_status = get_speaker_status()
    logging.info(f"[DEBUG] WebSocket speaker status: {speaker_status}")

    emit('status_update', {
        'config': current_config,
        'prayer_times': websocket_prayer_times,
        'scheduler_status': actual_status,
        'speaker_status': speaker_status,
        'devices': get_discovered_devices()
    })

def background_device_monitor():
    """Background task to monitor devices from scheduler's ChromecastManager"""

    # Initial wait for system to initialize
    initial_wait = 2  # Wait 2 seconds for system to initialize
    logging.info(f"Device monitor starting in {initial_wait} seconds...")
    time.sleep(initial_wait)

    while True:
        try:
            # Just monitor what devices the scheduler has discovered
            devices = get_discovered_devices()

            if devices:
                # Update cache timestamp to keep it fresh
                with cache_lock:
                    discovered_devices_cache['timestamp'] = time.time()

                # Emit update to connected clients if device count changed
                socketio.emit('chromecasts_discovered', {'devices': devices})
                logging.debug(f"Device monitor: {len(devices)} devices available from scheduler")
            else:
                logging.debug("Device monitor: No devices available from scheduler")

            # Check every 30 seconds
            time.sleep(30)

        except Exception as e:
            logging.error(f"Device monitor error: {e}")
            time.sleep(30)  # Wait before retry


def start_web_interface(chromecast_manager=None, scheduler=None, config_watcher=None):
    """Function to start the web interface - can be called from main.py"""
    global web_cast_manager, web_scheduler, web_config_watcher

    # Use provided chromecast manager or create new one
    if chromecast_manager:
        web_cast_manager = chromecast_manager
    else:
        web_cast_manager = ChromecastManager()

    # Store reference to scheduler for status monitoring
    if scheduler:
        web_scheduler = scheduler

    # Store reference to config watcher for hot reload monitoring
    if config_watcher:
        web_config_watcher = config_watcher


    # Load initial configuration
    load_config()

    # Populate cache timestamp if devices already found by ChromecastManager
    if web_cast_manager and web_cast_manager.chromecasts:
        logging.info(f"Populating cache with {len(web_cast_manager.chromecasts)} devices from ChromecastManager")
        update_discovered_devices_cache()

    # Start background device monitor thread (read-only monitoring)
    monitor_thread = threading.Thread(target=background_device_monitor, daemon=True)
    monitor_thread.start()
    
    # Run the web application
    logging.info("Starting Automated Azan Web Interface...")
    logging.info("Access the interface at: http://localhost:5000")
    
    socketio.run(app, host='0.0.0.0', port=5000, debug=False, allow_unsafe_werkzeug=True)


if __name__ == '__main__':
    start_web_interface()

