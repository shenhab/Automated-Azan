#!/usr/bin/env python3
"""
Startup launcher for Automated Azan with Web Interface
Can run both services in standalone mode or separately
"""

import os
import sys
import signal
import time
import subprocess
import threading
from multiprocessing import Process

def run_main_app():
    """Run the main Azan application"""
    try:
        import main
        print("Starting main Azan application...")
        main.main() if hasattr(main, 'main') else exec(open('main.py').read())
    except Exception as e:
        print(f"Error running main application: {e}")
        sys.exit(1)

def run_web_interface():
    """Run the web interface"""
    try:
        import web_interface
        print("Starting web interface...")
        web_interface.socketio.run(web_interface.app, host='0.0.0.0', port=5000, debug=False)
    except Exception as e:
        print(f"Error running web interface: {e}")
        sys.exit(1)

def signal_handler(sig, frame):
    """Handle shutdown signals"""
    print("\nShutting down Automated Azan services...")
    sys.exit(0)

def main():
    """Main launcher function"""
    # Register signal handlers
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    
    mode = os.environ.get('RUN_MODE', 'both').lower()
    
    if mode == 'web-only':
        print("Running in web interface only mode")
        run_web_interface()
    elif mode == 'main-only':
        print("Running in main application only mode")
        run_main_app()
    else:
        print("Running in combined mode (both services)")
        
        # Start web interface in a separate process
        web_process = Process(target=run_web_interface)
        web_process.start()
        
        # Give web interface time to start
        time.sleep(2)
        
        try:
            # Run main application in current process
            run_main_app()
        except KeyboardInterrupt:
            print("Received interrupt, shutting down...")
        finally:
            # Clean up web process
            if web_process.is_alive():
                web_process.terminate()
                web_process.join(timeout=5)
                if web_process.is_alive():
                    web_process.kill()

if __name__ == "__main__":
    main()
