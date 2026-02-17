"""
Chromecast Manager Package

A modular, robust Chromecast management system for the Automated Azan application.
"""

from .discovery import ChromecastDiscovery
from .connection import DeviceConnectionPool
from .manager import ChromecastManager

__all__ = ['ChromecastDiscovery', 'DeviceConnectionPool', 'ChromecastManager']
__version__ = '2.0.0'