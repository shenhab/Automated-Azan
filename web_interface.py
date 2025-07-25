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
import threading

app = Flask(__name__)
app.secret_key = 'your-secret-key-change-this'  # Change this in production
socketio = SocketIO(app, cors_allowed_origins="*", allow_unsafe_werkzeug=True)

# Global variables for real-time updates
discovered_devices = []
current_config = {}
prayer_times = {}
prayer_times_last_updated = None
scheduler_status = {"running": False, "next_prayer": None, "last_update": None}


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
    """Load current configuration from multiple possible locations"""
    global current_config
    
    # Try to load from different possible locations
    config_paths = [
        '/app/config/adahn.config',  # Docker volume location (writable)
        'config/adahn.config',       # Local config directory
        'adahn.config'               # Default location (may be read-only in Docker)
    ]
    
    config = configparser.ConfigParser()
    config_loaded = False
    
    for config_path in config_paths:
        if os.path.exists(config_path):
            try:
                config.read(config_path)
                current_config = {
                    'speakers_group_name': config.get('Settings', 'speakers-group-name', fallback=''),
                    'location': config.get('Settings', 'location', fallback='naas')
                }
                config_loaded = True
                logging.debug(f"Configuration loaded from {config_path}")
                break
            except Exception as e:
                logging.debug(f"Error reading config from {config_path}: {e}")
                continue
    
    if not config_loaded:
        # If no config file exists, copy from default location to writable location
        if os.path.exists('adahn.config'):
            try:
                import shutil
                os.makedirs('/app/config', exist_ok=True)
                shutil.copy2('adahn.config', '/app/config/adahn.config')
                logging.info("Copied default configuration to writable location")
                # Try to load again
                config.read('/app/config/adahn.config')
                current_config = {
                    'speakers_group_name': config.get('Settings', 'speakers-group-name', fallback=''),
                    'location': config.get('Settings', 'location', fallback='naas')
                }
                config_loaded = True
            except Exception as e:
                logging.warning(f"Could not copy config file: {e}")
        
        if not config_loaded:
            current_config = {'speakers_group_name': 'athan', 'location': 'naas'}
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
    load_prayer_times()
    
    return render_template('dashboard.html', 
                         config=current_config,
                         prayer_times=prayer_times,
                         scheduler_status=scheduler_status)

@app.route('/chromecasts')
def chromecasts():
    """Chromecast management page"""
    load_config()
    return render_template('chromecasts.html', 
                         config=current_config,
                         devices=discovered_devices)

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
    """API endpoint to get prayer times"""
    try:
        # Get the current location from config
        config = load_config()
        location = config.get('location', 'naas')  # Use 'naas' or 'icci'
        
        # Get prayer times using the cached load_prayer_times function
        times = load_prayer_times()
        
        return jsonify({
            'success': True,
            'prayer_times': times,
            'location': location
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
        return send_from_directory(media_dir, filename, mimetype='audio/mpeg')
        
    except Exception as e:
        logging.error(f"Error serving media file {filename}: {e}")
        return "Internal server error", 500

# API Routes
@app.route('/api/discover-chromecasts', methods=['POST'])
def api_discover_chromecasts():
    """API endpoint to discover Chromecast devices"""
    try:
        # Use ChromecastManager's discover_devices method
        web_cast_manager.discover_devices(force_rediscovery=True)
        
        devices = []
        if web_cast_manager.chromecasts:
            # Convert ChromecastManager's format to web interface format
            for uuid, cast_info in web_cast_manager.chromecasts.items():
                devices.append({
                    'name': cast_info['name'],
                    'host': cast_info['host'],
                    'port': cast_info['port'],
                    'uuid': uuid,
                    'model_name': cast_info['model_name'],
                    'manufacturer': cast_info['manufacturer']
                })
        
        # Emit real-time update to connected clients
        socketio.emit('chromecasts_discovered', {'devices': devices})
        
        return jsonify({
            'success': True,
            'devices': devices,
            'count': len(devices)
        })
        
    except Exception as e:
        logging.error(f"Error in discover_chromecasts: {e}")
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

@app.route('/api/save-config', methods=['POST'])
def api_save_config():
    """API endpoint to save configuration"""
    try:
        data = request.json
        speakers_name = data.get('speakers_name', '').strip()
        location = data.get('location', 'naas').strip()
        
        if not speakers_name:
            return jsonify({'success': False, 'error': 'Speaker name is required'}), 400
        
        save_config(speakers_name, location)
        
        # Emit real-time update
        socketio.emit('config_updated', {'config': current_config})
        
        return jsonify({
            'success': True,
            'message': 'Configuration saved successfully',
            'config': current_config
        })
        
    except Exception as e:
        logging.error(f"Error saving config: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500


@app.route('/api/refresh-prayer-times', methods=['POST'])
def api_refresh_prayer_times():
    """API endpoint to refresh prayer times"""
    try:
        load_prayer_times(force_reload=True)
        
        # Emit real-time update
        socketio.emit('prayer_times_updated', {'prayer_times': prayer_times})
        
        return jsonify({
            'success': True,
            'prayer_times': prayer_times
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
            success = web_cast_manager.start_adahn_alfajr()
            media_type = 'Fajr Adhan'
        else:
            success = web_cast_manager.start_adahn()
            media_type = 'Regular Adhan'
        
        return jsonify({
            'success': success,
            'message': f'{media_type} playback {"started" if success else "failed"}'
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
        
        # Get the appropriate media URL
        if adhan_type == 'fajr':
            media_url = web_cast_manager._get_media_url("media_adhan_al_fajr.mp3")
            media_type = 'Fajr Adhan'
        else:
            media_url = web_cast_manager._get_media_url("media_Athan.mp3")
            media_type = 'Regular Adhan'
        
        logging.info(f"Testing {media_type} on {device_name} with URL: {media_url}")
        
        # Temporarily override the target device for this test
        original_target = getattr(web_cast_manager, 'target_device', None)
        web_cast_manager.target_device = cast_device
        
        logging.info(f"Testing {media_type} on specific device: {device_name}")
        
        try:
            success = web_cast_manager.play_url_on_cast(media_url, preserve_target=True)
        finally:
            # Restore original target device
            web_cast_manager.target_device = original_target
        
        return jsonify({
            'success': success,
            'message': f'{media_type} {"started" if success else "failed"} on {device_name}'
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

# WebSocket events
@socketio.on('connect')
def handle_connect():
    """Handle client connection"""
    emit('connected', {'data': 'Connected to Automated Azan Web Interface'})

@socketio.on('request_status')
def handle_status_request():
    """Handle status request from client"""
    load_config()
    load_prayer_times()
    
    emit('status_update', {
        'config': current_config,
        'prayer_times': prayer_times,
        'scheduler_status': scheduler_status,
        'devices': discovered_devices
    })

def background_discovery():
    """Background task to periodically refresh device list"""
    global discovered_devices
    while True:
        try:
            # Use existing discovered devices (don't force rediscovery to avoid duplication)
            if web_cast_manager and web_cast_manager.chromecasts:

                # Convert ChromecastManager format to web interface format
                devices = []
                for uuid, cast_info in web_cast_manager.chromecasts.items():
                    devices.append({
                        'name': cast_info['name'],
                        'uuid': uuid,
                        'model': cast_info['model_name'],
                        'host': cast_info['host'],
                        'port': cast_info['port'],
                        'cast_type': cast_info.get('manufacturer', 'Unknown'),
                        'status': 'Available'
                    })
                
                discovered_devices = devices
                socketio.emit('chromecasts_discovered', {'devices': devices})
                logging.info(f"Background discovery found {len(devices)} devices")
            else:

                # Only force discovery if we have no devices
                if web_cast_manager:
                    web_cast_manager.discover_devices(force_rediscovery=True)
                
            time.sleep(600)  # Refresh every 10 minutes (reduced frequency for better performance)

        except Exception as e:
            logging.error(f"Background discovery error: {e}")
            time.sleep(600)  # Wait longer on error


def start_web_interface(chromecast_manager=None):
    """Function to start the web interface - can be called from main.py"""
    global web_cast_manager
    
    # Use provided chromecast manager or create new one
    if chromecast_manager:
        web_cast_manager = chromecast_manager
    else:
        web_cast_manager = ChromecastManager()

    
    # Load initial configuration
    load_config()
    
    # Start background discovery thread
    discovery_thread = threading.Thread(target=background_discovery, daemon=True)
    discovery_thread.start()
    
    # Run the web application
    logging.info("Starting Automated Azan Web Interface...")
    logging.info("Access the interface at: http://localhost:5000")
    
    socketio.run(app, host='0.0.0.0', port=5000, debug=False, allow_unsafe_werkzeug=True)


if __name__ == '__main__':
    start_web_interface()

