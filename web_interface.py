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
from datetime import datetime
from flask import Flask, render_template, request, jsonify, redirect, url_for, flash
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
scheduler_status = {"running": False, "next_prayer": None, "last_update": None}

# Use our working ChromecastManager instead of WebChromecastManager
web_cast_manager = ChromecastManager()

def load_config():
    """Load current configuration"""
    global current_config
    
    config = configparser.ConfigParser()
    config_file = 'adahn.config'
    
    if os.path.exists(config_file):
        config.read(config_file)
        current_config = {
            'speakers_group_name': config.get('Settings', 'speakers-group-name', fallback=''),
            'location': config.get('Settings', 'location', fallback='naas')
        }
    else:
        current_config = {'speakers_group_name': '', 'location': 'naas'}
    
    return current_config

def save_config(speakers_name, location):
    """Save configuration to file"""
    config = configparser.ConfigParser()
    config.add_section('Settings')
    config.set('Settings', 'speakers-group-name', speakers_name)
    config.set('Settings', 'location', location)
    
    with open('adahn.config', 'w') as config_file:
        config.write(config_file)
    
    load_config()  # Reload to update global variable

def load_prayer_times():
    """Load current prayer times"""
    global prayer_times
    
    try:
        fetcher = PrayerTimesFetcher()
        location = current_config.get('location', 'naas')
        times = fetcher.fetch_prayer_times(location)
        
        if isinstance(times, dict) and 'error' not in times:
            prayer_times = times
        else:
            prayer_times = {'error': 'Failed to load prayer times'}
            
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
    return render_template('logs.html')

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

@app.route('/api/prayer-times', methods=['GET'])
def api_prayer_times():
    """API endpoint to get current prayer times"""
    load_prayer_times()
    return jsonify(prayer_times)

@app.route('/api/refresh-prayer-times', methods=['POST'])
def api_refresh_prayer_times():
    """API endpoint to refresh prayer times"""
    try:
        load_prayer_times()
        
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
    """Background task to periodically discover devices"""
    global discovered_devices
    while True:
        try:
            # Use ChromecastManager's discovery method
            web_cast_manager.discover_devices(force_rediscovery=True)
            
            if web_cast_manager.chromecasts:
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
                logging.warning("Background discovery found no devices")
                
            time.sleep(30)  # Refresh every 30 seconds
        except Exception as e:
            logging.error(f"Background discovery error: {e}")
            time.sleep(60)  # Wait longer on error

if __name__ == '__main__':
    # Setup logging
    logging.basicConfig(level=logging.INFO,
                       format='%(asctime)s - %(levelname)s - %(message)s')
    
    # Load initial configuration
    load_config()
    
    # Start background discovery thread
    discovery_thread = threading.Thread(target=background_discovery, daemon=True)
    discovery_thread.start()
    
    # Run the web application
    print("Starting Automated Azan Web Interface...")
    print("Access the interface at: http://localhost:5000")
    
    socketio.run(app, host='0.0.0.0', port=5000, debug=False, allow_unsafe_werkzeug=True)
