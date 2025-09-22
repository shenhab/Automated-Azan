import logging
import os
import json
from datetime import datetime


def setup_logging(log_file=None):
    """
    Configure logging for the Azan application.

    Args:
        log_file (str): Path to the log file. If None, uses environment variable or default.

    Returns:
        dict: JSON response with logging setup status
    """
    try:
        if log_file is None:
            log_file = os.environ.get('LOG_FILE', '/var/log/azan_service.log')

        # Configure basic logging
        logging.basicConfig(
            format='%(asctime)s [%(levelname)s]: %(message)s',
            filename=log_file,
            level=logging.INFO
        )

        # Also log to console for Docker logs
        console_handler = logging.StreamHandler()
        console_handler.setFormatter(
            logging.Formatter('%(asctime)s [%(levelname)s]: %(message)s')
        )
        logging.getLogger().addHandler(console_handler)

        # Reduce pychromecast logging verbosity to reduce connection error spam
        logging.getLogger('pychromecast').setLevel(logging.WARNING)
        logging.getLogger('pychromecast.socket_client').setLevel(logging.ERROR)

        logging.info(f"Logging configured. Log file: {log_file}")

        return {
            "success": True,
            "log_file": log_file,
            "level": "INFO",
            "console_logging": True,
            "pychromecast_reduced": True,
            "message": f"Logging configured successfully. Log file: {log_file}",
            "timestamp": datetime.now().isoformat()
        }

    except Exception as e:
        return {
            "success": False,
            "error": str(e),
            "log_file": log_file,
            "timestamp": datetime.now().isoformat()
        }


def setup_debug_logging(log_file=None):
    """
    Configure debug-level logging for the Azan application.

    Args:
        log_file (str): Path to the log file. If None, uses environment variable or default.

    Returns:
        dict: JSON response with debug logging setup status
    """
    try:
        if log_file is None:
            log_file = os.environ.get('LOG_FILE', '/var/log/azan_service.log')

        # Configure debug logging
        logging.basicConfig(
            format='%(asctime)s [%(levelname)s] %(name)s: %(message)s',
            filename=log_file,
            level=logging.DEBUG
        )

        # Also log to console for Docker logs
        console_handler = logging.StreamHandler()
        console_handler.setFormatter(
            logging.Formatter('%(asctime)s [%(levelname)s] %(name)s: %(message)s')
        )
        logging.getLogger().addHandler(console_handler)

        # Even in debug mode, reduce pychromecast logging verbosity
        logging.getLogger('pychromecast').setLevel(logging.INFO)
        logging.getLogger('pychromecast.socket_client').setLevel(logging.WARNING)

        logging.info(f"Debug logging configured. Log file: {log_file}")

        return {
            "success": True,
            "log_file": log_file,
            "level": "DEBUG",
            "console_logging": True,
            "pychromecast_level": "INFO",
            "extended_format": True,
            "message": f"Debug logging configured successfully. Log file: {log_file}",
            "timestamp": datetime.now().isoformat()
        }

    except Exception as e:
        return {
            "success": False,
            "error": str(e),
            "log_file": log_file,
            "timestamp": datetime.now().isoformat()
        }


def get_logger(name):
    """
    Get a logger instance with the specified name.

    Args:
        name (str): Logger name

    Returns:
        dict: JSON response with logger information
    """
    try:
        logger = logging.getLogger(name)

        return {
            "success": True,
            "logger_name": name,
            "level": logging.getLevelName(logger.level),
            "effective_level": logging.getLevelName(logger.getEffectiveLevel()),
            "handlers_count": len(logger.handlers),
            "propagate": logger.propagate,
            "message": f"Logger '{name}' retrieved successfully",
            "timestamp": datetime.now().isoformat()
        }

    except Exception as e:
        return {
            "success": False,
            "logger_name": name,
            "error": str(e),
            "timestamp": datetime.now().isoformat()
        }


def set_log_level(level, logger_name=None):
    """
    Set logging level for a specific logger or root logger.

    Args:
        level (str): Logging level (DEBUG, INFO, WARNING, ERROR, CRITICAL)
        logger_name (str): Logger name. If None, sets root logger level.

    Returns:
        dict: JSON response with level change status
    """
    try:
        level_map = {
            'DEBUG': logging.DEBUG,
            'INFO': logging.INFO,
            'WARNING': logging.WARNING,
            'ERROR': logging.ERROR,
            'CRITICAL': logging.CRITICAL
        }

        if level.upper() not in level_map:
            return {
                "success": False,
                "error": f"Invalid log level: {level}. Valid levels: {list(level_map.keys())}",
                "timestamp": datetime.now().isoformat()
            }

        logger = logging.getLogger(logger_name) if logger_name else logging.getLogger()
        old_level = logging.getLevelName(logger.level)
        logger.setLevel(level_map[level.upper()])

        return {
            "success": True,
            "logger_name": logger_name or "root",
            "old_level": old_level,
            "new_level": level.upper(),
            "message": f"Log level changed from {old_level} to {level.upper()}",
            "timestamp": datetime.now().isoformat()
        }

    except Exception as e:
        return {
            "success": False,
            "logger_name": logger_name or "root",
            "level": level,
            "error": str(e),
            "timestamp": datetime.now().isoformat()
        }


def get_logging_status():
    """
    Get current logging configuration status.

    Returns:
        dict: JSON response with logging status information
    """
    try:
        root_logger = logging.getLogger()
        handlers_info = []

        for handler in root_logger.handlers:
            handler_info = {
                "type": type(handler).__name__,
                "level": logging.getLevelName(handler.level),
                "formatter": str(handler.formatter._fmt) if handler.formatter else None
            }

            # Add file-specific info for FileHandler
            if hasattr(handler, 'baseFilename'):
                handler_info["file_path"] = handler.baseFilename

            handlers_info.append(handler_info)

        # Get specific logger levels
        specific_loggers = {}
        for name in ['pychromecast', 'pychromecast.socket_client']:
            logger = logging.getLogger(name)
            specific_loggers[name] = {
                "level": logging.getLevelName(logger.level),
                "effective_level": logging.getLevelName(logger.getEffectiveLevel())
            }

        return {
            "success": True,
            "root_logger": {
                "level": logging.getLevelName(root_logger.level),
                "effective_level": logging.getLevelName(root_logger.getEffectiveLevel()),
                "handlers_count": len(root_logger.handlers)
            },
            "handlers": handlers_info,
            "specific_loggers": specific_loggers,
            "message": "Logging status retrieved successfully",
            "timestamp": datetime.now().isoformat()
        }

    except Exception as e:
        return {
            "success": False,
            "error": str(e),
            "timestamp": datetime.now().isoformat()
        }


def add_file_handler(log_file, level="INFO", logger_name=None):
    """
    Add a file handler to a logger.

    Args:
        log_file (str): Path to the log file
        level (str): Logging level for the handler
        logger_name (str): Logger name. If None, adds to root logger.

    Returns:
        dict: JSON response with handler addition status
    """
    try:
        level_map = {
            'DEBUG': logging.DEBUG,
            'INFO': logging.INFO,
            'WARNING': logging.WARNING,
            'ERROR': logging.ERROR,
            'CRITICAL': logging.CRITICAL
        }

        if level.upper() not in level_map:
            return {
                "success": False,
                "error": f"Invalid log level: {level}. Valid levels: {list(level_map.keys())}",
                "timestamp": datetime.now().isoformat()
            }

        logger = logging.getLogger(logger_name) if logger_name else logging.getLogger()

        # Create file handler
        file_handler = logging.FileHandler(log_file)
        file_handler.setLevel(level_map[level.upper()])
        file_handler.setFormatter(
            logging.Formatter('%(asctime)s [%(levelname)s]: %(message)s')
        )

        logger.addHandler(file_handler)

        return {
            "success": True,
            "logger_name": logger_name or "root",
            "log_file": log_file,
            "level": level.upper(),
            "message": f"File handler added successfully for {log_file}",
            "timestamp": datetime.now().isoformat()
        }

    except Exception as e:
        return {
            "success": False,
            "logger_name": logger_name or "root",
            "log_file": log_file,
            "level": level,
            "error": str(e),
            "timestamp": datetime.now().isoformat()
        }


def remove_all_handlers(logger_name=None):
    """
    Remove all handlers from a logger.

    Args:
        logger_name (str): Logger name. If None, removes from root logger.

    Returns:
        dict: JSON response with handler removal status
    """
    try:
        logger = logging.getLogger(logger_name) if logger_name else logging.getLogger()
        handler_count = len(logger.handlers)

        for handler in logger.handlers[:]:  # Copy list to avoid modification during iteration
            logger.removeHandler(handler)
            handler.close()

        return {
            "success": True,
            "logger_name": logger_name or "root",
            "removed_handlers": handler_count,
            "message": f"Removed {handler_count} handlers from logger",
            "timestamp": datetime.now().isoformat()
        }

    except Exception as e:
        return {
            "success": False,
            "logger_name": logger_name or "root",
            "error": str(e),
            "timestamp": datetime.now().isoformat()
        }


def cleanup_logging():
    """
    Cleanup logging configuration by removing all handlers.
    Alias for remove_all_handlers for backward compatibility.

    Returns:
        dict: JSON response with cleanup status
    """
    result = remove_all_handlers()
    if result['success']:
        result['handlers_removed'] = result['removed_handlers']
        result['message'] = f"Logging cleanup completed. {result['removed_handlers']} handlers removed."
    return result