import configparser
import logging
import os
import json
from dotenv import load_dotenv
from datetime import datetime


class ConfigManager:
    """
    A class to handle configuration management for the Azan application.
    All methods return JSON responses for API compatibility.
    """

    def __init__(self, config_file="adahn.config"):
        """
        Initialize the configuration manager.

        Args:
            config_file (str): Path to the configuration file
        """
        self.config_file = config_file
        self.config = configparser.ConfigParser()
        self.load_environment()
        self.load_config()

    def load_environment(self):
        """
        Load environment variables from .env file.

        Returns:
            dict: JSON response with environment loading status
        """
        try:
            load_dotenv()
            logging.debug("Environment variables loaded")
            return {
                "success": True,
                "message": "Environment variables loaded successfully",
                "timestamp": datetime.now().isoformat()
            }
        except Exception as e:
            logging.error(f"Failed to load environment variables: {e}")
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def load_config(self):
        """
        Load configuration from the config file.

        Returns:
            dict: JSON response with configuration loading status
        """
        try:
            self.config.read(self.config_file)
            logging.info(f"Configuration loaded from {self.config_file}")
            return {
                "success": True,
                "config_file": self.config_file,
                "sections": list(self.config.sections()),
                "message": f"Configuration loaded from {self.config_file}",
                "timestamp": datetime.now().isoformat()
            }
        except Exception as e:
            logging.error(f"Failed to load configuration from {self.config_file}: {e}")
            return {
                "success": False,
                "config_file": self.config_file,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def get_setting(self, section, key, fallback=None):
        """
        Get a setting from the configuration.

        Args:
            section (str): Configuration section
            key (str): Configuration key
            fallback: Default value if key is not found

        Returns:
            dict: JSON response with setting value or error
        """
        try:
            value = self.config[section][key]
            return {
                "success": True,
                "section": section,
                "key": key,
                "value": value,
                "timestamp": datetime.now().isoformat()
            }
        except KeyError:
            if fallback is not None:
                logging.warning(f"Configuration key {section}.{key} not found, using fallback: {fallback}")
                return {
                    "success": True,
                    "section": section,
                    "key": key,
                    "value": fallback,
                    "fallback_used": True,
                    "message": f"Using fallback value for {section}.{key}",
                    "timestamp": datetime.now().isoformat()
                }
            else:
                error_msg = f"Missing required configuration key: {section}.{key}"
                logging.error(error_msg)
                return {
                    "success": False,
                    "section": section,
                    "key": key,
                    "error": error_msg,
                    "timestamp": datetime.now().isoformat()
                }

    def get_speakers_group_name(self):
        """
        Get the speakers group name from configuration.

        Returns:
            dict: JSON response with speakers group name
        """
        result = self.get_setting("Settings", "speakers-group-name")
        if result["success"]:
            return {
                "success": True,
                "speakers_group_name": result["value"],
                "timestamp": datetime.now().isoformat()
            }
        else:
            return {
                "success": False,
                "error": result["error"],
                "timestamp": datetime.now().isoformat()
            }

    def get_location(self):
        """
        Get the location from configuration.

        Returns:
            dict: JSON response with location
        """
        result = self.get_setting("Settings", "location")
        if result["success"]:
            return {
                "success": True,
                "location": result["value"],
                "timestamp": datetime.now().isoformat()
            }
        else:
            return {
                "success": False,
                "error": result["error"],
                "timestamp": datetime.now().isoformat()
            }

    def get_prayer_source(self):
        """
        Get the prayer source from configuration.

        Returns:
            dict: JSON response with prayer source
        """
        result = self.get_setting("Settings", "prayer_source", "icci")
        return {
            "success": True,
            "prayer_source": result["value"],
            "fallback_used": result.get("fallback_used", False),
            "timestamp": datetime.now().isoformat()
        }

    def is_pre_fajr_enabled(self):
        """
        Check if pre-Fajr Quran is enabled.

        Returns:
            dict: JSON response with pre-Fajr status
        """
        try:
            result = self.get_setting("Settings", "pre_fajr_enabled", "True")
            value = result["value"]
            enabled = value.lower() in ['true', '1', 'yes', 'on']

            return {
                "success": True,
                "pre_fajr_enabled": enabled,
                "raw_value": value,
                "fallback_used": result.get("fallback_used", False),
                "timestamp": datetime.now().isoformat()
            }
        except Exception as e:
            return {
                "success": True,  # Default to enabled even on error
                "pre_fajr_enabled": True,
                "default_used": True,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def get_log_file(self):
        """
        Get the log file path from environment or default.

        Returns:
            dict: JSON response with log file path
        """
        try:
            log_file = os.environ.get('LOG_FILE', '/var/log/azan_service.log')
            env_used = 'LOG_FILE' in os.environ

            return {
                "success": True,
                "log_file": log_file,
                "environment_variable_used": env_used,
                "timestamp": datetime.now().isoformat()
            }
        except Exception as e:
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def validate_config(self):
        """
        Validate that all required configuration keys are present.

        Returns:
            dict: JSON response with validation results
        """
        required_settings = [
            ("Settings", "speakers-group-name"),
            ("Settings", "location")
        ]

        validation_results = []
        all_valid = True

        for section, key in required_settings:
            result = self.get_setting(section, key)
            validation_results.append({
                "section": section,
                "key": key,
                "valid": result["success"],
                "error": result.get("error") if not result["success"] else None
            })

            if not result["success"]:
                all_valid = False

        if all_valid:
            logging.info("Configuration validation successful")
            return {
                "success": True,
                "message": "Configuration validation successful",
                "validated_settings": validation_results,
                "timestamp": datetime.now().isoformat()
            }
        else:
            return {
                "success": False,
                "message": "Configuration validation failed",
                "validated_settings": validation_results,
                "timestamp": datetime.now().isoformat()
            }

    def get_all_settings(self):
        """
        Get all configuration settings as a dictionary.

        Returns:
            dict: JSON response with all configuration settings
        """
        try:
            settings = {}
            for section in self.config.sections():
                settings[section] = dict(self.config[section])

            return {
                "success": True,
                "settings": settings,
                "sections_count": len(settings),
                "config_file": self.config_file,
                "timestamp": datetime.now().isoformat()
            }
        except Exception as e:
            logging.error(f"Error getting all settings: {e}")
            return {
                "success": False,
                "error": str(e),
                "config_file": self.config_file,
                "timestamp": datetime.now().isoformat()
            }

    def update_setting(self, section, key, value):
        """
        Update a configuration setting.

        Args:
            section (str): Configuration section
            key (str): Configuration key
            value (str): New value

        Returns:
            dict: JSON response with update result
        """
        try:
            if section not in self.config:
                self.config.add_section(section)

            old_value = self.config.get(section, key, fallback=None)
            self.config.set(section, key, str(value))

            return {
                "success": True,
                "section": section,
                "key": key,
                "old_value": old_value,
                "new_value": str(value),
                "message": f"Setting {section}.{key} updated successfully",
                "timestamp": datetime.now().isoformat()
            }
        except Exception as e:
            logging.error(f"Error updating setting {section}.{key}: {e}")
            return {
                "success": False,
                "section": section,
                "key": key,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def save_config(self):
        """
        Save the current configuration to file.

        Returns:
            dict: JSON response with save result
        """
        try:
            with open(self.config_file, 'w') as configfile:
                self.config.write(configfile)

            logging.info(f"Configuration saved to {self.config_file}")
            return {
                "success": True,
                "config_file": self.config_file,
                "message": f"Configuration saved to {self.config_file}",
                "timestamp": datetime.now().isoformat()
            }
        except Exception as e:
            logging.error(f"Error saving configuration to {self.config_file}: {e}")
            return {
                "success": False,
                "config_file": self.config_file,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def reload_config(self):
        """
        Reload configuration from file.

        Returns:
            dict: JSON response with reload result
        """
        try:
            # Clear current config
            self.config.clear()

            # Reload from file
            result = self.load_config()

            if result["success"]:
                return {
                    "success": True,
                    "config_file": self.config_file,
                    "sections": result["sections"],
                    "message": "Configuration reloaded successfully",
                    "timestamp": datetime.now().isoformat()
                }
            else:
                return result

        except Exception as e:
            logging.error(f"Error reloading configuration: {e}")
            return {
                "success": False,
                "config_file": self.config_file,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }

    def get_config_info(self):
        """
        Get information about the configuration file and current state.

        Returns:
            dict: JSON response with configuration information
        """
        try:
            file_exists = os.path.exists(self.config_file)
            file_size = os.path.getsize(self.config_file) if file_exists else 0
            file_mtime = os.path.getmtime(self.config_file) if file_exists else None

            return {
                "success": True,
                "config_file": self.config_file,
                "file_exists": file_exists,
                "file_size_bytes": file_size,
                "file_modified_time": datetime.fromtimestamp(file_mtime).isoformat() if file_mtime else None,
                "sections_count": len(self.config.sections()),
                "sections": list(self.config.sections()),
                "timestamp": datetime.now().isoformat()
            }
        except Exception as e:
            logging.error(f"Error getting config info: {e}")
            return {
                "success": False,
                "config_file": self.config_file,
                "error": str(e),
                "timestamp": datetime.now().isoformat()
            }