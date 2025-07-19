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
from pychromecast.discovery import CastBrowser, SimpleCastListener
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

class WebChromecastManager:
    """Enhanced Chromecast manager for web interface"""
    
    def __init__(self):
        self.devices = {}
        self.discovery_timeout = 10
        self.browser = None
        self.listener = None
        self.discovery_complete = threading.Event()
        
    def discover_devices(self, timeout=10):
        """Discover available Chromecast devices using CastBrowser"""
        global discovered_devices
        
        try:
            logging.info("Starting Chromecast discovery with CastBrowser...")
            
            # Stop previous browser if exists
            if self.browser:
                self.browser.stop_discovery()
                
            # Clear previous discoveries
            self.devices.clear()
            self.discovery_complete.clear()
            
            # Create a cast listener
            self.listener = SimpleCastListener(self._add_cast, self._remove_cast)
            
            # Create and start browser
            self.browser = CastBrowser(self.listener, None, None)
            self.browser.start_discovery()
            
            # Wait for discovery to find devices
            if self.discovery_complete.wait(timeout=timeout):
                logging.info(f"Discovery completed. Found {len(self.devices)} devices")
            else:
                logging.info(f"Discovery timeout reached. Found {len(self.devices)} devices so far")
            
            devices = []
            for uuid, device_info in self.devices.items():
                devices.append({
                    'name': device_info['name'],
                    'uuid': device_info['uuid'],
                    'model': device_info['model_name'],
                    'host': device_info['host'],
                    'port': device_info['port'],
                    'cast_type': device_info.get('cast_type', 'Unknown'),
                    'status': 'Available'
                })
                
            discovered_devices = devices
            
            # Stop browser to prevent resource leaks
            if self.browser:
                self.browser.stop_discovery()
            
            logging.info(f"Found {len(devices)} Chromecast devices")
            return devices
            
        except Exception as e:
            logging.error(f"Error discovering Chromecasts: {e}")
            return []
            
    def _add_cast(self, uuid, service):
        """Callback when a new cast device is discovered."""
        try:
            cast_info = {
                'uuid': uuid,
                'name': service.friendly_name,
                'host': service.host,
                'port': service.port,
                'model_name': service.model_name,
                'manufacturer': service.manufacturer,
                'cast_type': getattr(service, 'cast_type', 'Unknown'),
                'service': service
            }
            self.devices[uuid] = cast_info
            logging.debug(f"Added cast device: {service.friendly_name} ({service.model_name})")
            
            # Signal discovery completion if we found some devices
            if len(self.devices) >= 1:
                self.discovery_complete.set()
                
        except Exception as e:
            logging.error(f"Error adding cast device {uuid}: {e}")
            
    def _remove_cast(self, uuid, service):
        """Callback when a cast device is removed."""
        if uuid in self.devices:
            cast_info = self.devices.pop(uuid)
            logging.debug(f"Removed cast device: {cast_info['name']}")
    
    def test_connection(self, device_name):
        """Test connection to a specific Chromecast device"""
        try:
            # Find device in our discovered devices
            target_device = None
            for uuid, device_info in self.devices.items():
                if device_info['name'] == device_name:
                    target_device = device_info
                    break
                    
            if not target_device:
                return False, "Device not found"
                
            # Create Chromecast instance
            chromecast = pychromecast.get_chromecast_from_service(
                target_device['service'],
                zconf=self.browser.zc if self.browser else None
            )
            
            chromecast.wait(timeout=5)
            logging.info(f"Successfully connected to {device_name}")
            return True, "Connection successful"
            
        except Exception as e:
            logging.error(f"Error testing connection to {device_name}: {e}")
            return False, str(e)
            
        except Exception as e:
            logging.error(f"Error testing connection to {device_name}: {e}")
            return {'success': False, 'error': str(e)}

web_cast_manager = WebChromecastManager()

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
        devices = web_cast_manager.discover_devices()
        
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
    
    result = web_cast_manager.test_connection(device_name)
    return jsonify(result)

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
    while True:
        try:
            devices = web_cast_manager.discover_devices(timeout=5)
            socketio.emit('chromecasts_discovered', {'devices': devices})
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
