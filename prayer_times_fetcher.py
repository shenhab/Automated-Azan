import os
import requests
import json
import logging
from datetime import datetime
from dateutil import tz
from bs4 import BeautifulSoup
import re
from typing import Dict, Optional, Any, List, Tuple
from prayer_times_exceptions import (
    PrayerTimesError,
    InvalidLocationError,
    DataNotFoundError,
    NetworkError,
    FileOperationError,
    ValidationError,
    ParsingError,
    RefreshRequiredError
)
from prayer_times_config import PrayerTimesConfig, load_config
import time


class PrayerTimesFetcher:
    """
    A class to fetch prayer times from various sources.
    All methods return JSON responses for API compatibility.
    """

    def __init__(self, config: Optional[PrayerTimesConfig] = None) -> None:
        """
        Initializes the prayer time fetcher with configuration settings.

        Args:
            config: Optional configuration object. If not provided, uses defaults.
        """
        self.config = config or load_config()

        # For backward compatibility, expose commonly used attributes
        self.sources = self.config.sources
        self.naas_prayers_timetable_file = self.config.naas_timetable_path
        self.icci_timetable_file = self.config.icci_timetable_path
        self.tz = self.config.timezone

        # Initialize cache
        self._cache: Dict[str, Tuple[Any, float]] = {}  # key: (data, timestamp)

    def _get_cache_key(self, location: str, date: datetime) -> str:
        """Generate cache key for a specific location and date."""
        return f"{location}_{date.strftime('%Y-%m-%d')}"

    def _is_cache_valid(self, key: str) -> bool:
        """Check if cached data is still valid."""
        if not self.config.cache_enabled:
            return False

        if key not in self._cache:
            return False

        _, timestamp = self._cache[key]
        age = time.time() - timestamp
        return age < self.config.cache_ttl_seconds

    def _get_from_cache(self, key: str) -> Optional[Dict[str, Any]]:
        """Get data from cache if valid."""
        if self._is_cache_valid(key):
            data, _ = self._cache[key]
            logging.debug(f"Cache hit for key: {key}")
            return data
        return None

    def _put_in_cache(self, key: str, data: Dict[str, Any]) -> None:
        """Store data in cache."""
        if self.config.cache_enabled:
            self._cache[key] = (data, time.time())
            logging.debug(f"Cached data for key: {key}")

    def clear_cache(self) -> None:
        """Clear all cached data."""
        self._cache.clear()
        logging.info("Cache cleared")

    def _download_icci_timetable(self) -> Dict[str, Any]:
        """
        Fetches and saves the ICCI prayer timetable from the API.

        Returns:
            dict: JSON response with download status
        """
        logging.debug("Attempting to download ICCI timetable.")
        try:
            response = requests.get(self.sources['icci'], timeout=10)
            response.raise_for_status()

            with open(self.icci_timetable_file, "w", encoding="utf-8") as f:
                json.dump(response.json(), f, indent=4)

            logging.info("✅ ICCI timetable downloaded and saved successfully.")
            return {
                "success": True,
                "source": "icci",
                "url": self.sources['icci'],
                "file_path": self.icci_timetable_file,
                "message": "ICCI timetable downloaded and saved successfully",
                "timestamp": datetime.now(self.tz).isoformat()
            }

        except requests.RequestException as e:
            logging.error("❌ Failed to download ICCI timetable: %s", e)
            return {
                "success": False,
                "source": "icci",
                "url": self.sources['icci'],
                "error": str(e),
                "error_type": "network_error",
                "timestamp": datetime.now(self.tz).isoformat()
            }
        except Exception as e:
            logging.error("❌ Unexpected error downloading ICCI timetable: %s", e)
            return {
                "success": False,
                "source": "icci",
                "error": str(e),
                "error_type": "file_error",
                "timestamp": datetime.now(self.tz).isoformat()
            }

    def _download_naas_timetable(self) -> Dict[str, Any]:
        """
        Fetches prayer timetable data for Naas using web scraping.
        Extracts 'calendar' data from the webpage and saves it to a JSON file.

        Returns:
            dict: JSON response with download status
        """
        logging.debug("Attempting to download Naas timetable from Mawaqit.")
        try:
            response = requests.get(self.sources['naas'], timeout=10)
            response.raise_for_status()
            soup = BeautifulSoup(response.text, "html.parser")
            script_tags = soup.find_all("script")

            calendar_data = None
            for script in script_tags:
                if "calendar" in script.text:
                    match = re.search(r'"calendar"\s*:\s*(\[\{.*?\}\])', script.text, re.DOTALL)
                    if match:
                        calendar_data = match.group(1)
                    break

            if not calendar_data:
                logging.error("❌ Calendar data not found in Naas webpage!")
                return {
                    "success": False,
                    "source": "naas",
                    "url": self.sources['naas'],
                    "error": "Calendar data not found in webpage",
                    "error_type": "parsing_error",
                    "timestamp": datetime.now(self.tz).isoformat()
                }

            calendar_list = json.loads(calendar_data)
            with open(self.naas_prayers_timetable_file, "w", encoding="utf-8") as file:
                json.dump(calendar_list, file, indent=2, ensure_ascii=False)

            logging.info("✅ Naas prayer timetable saved successfully.")
            return {
                "success": True,
                "source": "naas",
                "url": self.sources['naas'],
                "file_path": self.naas_prayers_timetable_file,
                "calendar_entries": len(calendar_list),
                "message": "Naas prayer timetable saved successfully",
                "timestamp": datetime.now(self.tz).isoformat()
            }

        except requests.RequestException as e:
            logging.error("❌ Error fetching Naas data: %s", e)
            return {
                "success": False,
                "source": "naas",
                "url": self.sources['naas'],
                "error": str(e),
                "error_type": "network_error",
                "timestamp": datetime.now(self.tz).isoformat()
            }
        except json.JSONDecodeError as e:
            logging.error("❌ Error parsing Naas JSON: %s", e)
            return {
                "success": False,
                "source": "naas",
                "error": str(e),
                "error_type": "json_error",
                "timestamp": datetime.now(self.tz).isoformat()
            }
        except Exception as e:
            logging.error("❌ Unexpected error downloading Naas timetable: %s", e)
            return {
                "success": False,
                "source": "naas",
                "error": str(e),
                "error_type": "file_error",
                "timestamp": datetime.now(self.tz).isoformat()
            }

    def _is_new_month(self, location: str = "icci", target_date: Optional[datetime] = None) -> Dict[str, Any]:
        """
        Determines if the timetable should be refreshed by checking if a new month has started.
        Also checks if timetable files exist for the specified location.

        Args:
            location (str): Location to check ('icci' or 'naas')
            target_date (datetime): Date to check against (defaults to today)

        Returns:
            dict: JSON response with refresh status
        """
        if target_date is None:
            target_date = datetime.now(self.tz)

        logging.debug(f"Checking if a new month has started or {location} timetable file is missing.")

        timetable_file = self.icci_timetable_file if location == "icci" else self.naas_prayers_timetable_file

        if not os.path.exists(timetable_file):
            logging.info(f"{location.upper()} timetable file is missing. Need to download new one.")
            return {
                "success": True,
                "needs_refresh": True,
                "reason": "file_missing",
                "location": location,
                "file_path": timetable_file,
                "message": f"{location.upper()} timetable file is missing",
                "timestamp": datetime.now(self.tz).isoformat()
            }

        try:
            file_mod_time = datetime.fromtimestamp(os.path.getmtime(timetable_file))
            target_month = target_date.month
            is_new_month = file_mod_time.month != target_month

            logging.debug(f"{location.upper()} timetable last modified in month: {file_mod_time.month}, Target month: {target_month}")

            return {
                "success": True,
                "needs_refresh": is_new_month,
                "reason": "new_month" if is_new_month else "current_month",
                "location": location,
                "file_path": timetable_file,
                "file_modified_time": file_mod_time.isoformat(),
                "file_month": file_mod_time.month,
                "target_month": target_month,
                "message": f"File {'needs refresh (new month)' if is_new_month else 'is current'}",
                "timestamp": datetime.now(self.tz).isoformat()
            }

        except Exception as e:
            logging.error(f"Error reading {location} timetable file modification date: %s", e)
            return {
                "success": False,
                "needs_refresh": True,  # Default to refresh on error
                "reason": "file_error",
                "location": location,
                "file_path": timetable_file,
                "error": str(e),
                "message": f"Error reading {location} timetable file, defaulting to refresh",
                "timestamp": datetime.now(self.tz).isoformat()
            }

    def _validate_location(self, location: str) -> Dict[str, Any]:
        """
        Validates that the location is supported.

        Args:
            location (str): Location to validate

        Returns:
            dict: Validation result
        """
        if not self.config.is_valid_location(location):
            logging.error("Invalid location provided: %s", location)
            error = InvalidLocationError(location, self.config.valid_locations)
            result = error.to_dict()
            result["timestamp"] = datetime.now(self.tz).isoformat()
            return result
        return {"success": True, "location": location}

    def _validate_prayer_times(self, prayer_times: Dict[str, str]) -> Dict[str, Any]:
        """
        Validates that prayer times dictionary contains all required prayers
        with valid time format (HH:MM).

        Args:
            prayer_times (dict): Dictionary of prayer times to validate

        Returns:
            dict: Validation result
        """
        time_pattern = re.compile(r'^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$')

        missing_prayers = []
        invalid_formats = []

        for prayer in self.config.required_prayers:
            if prayer not in prayer_times:
                missing_prayers.append(prayer)
            elif not time_pattern.match(prayer_times[prayer]):
                invalid_formats.append(f"{prayer}: {prayer_times[prayer]}")

        if missing_prayers or invalid_formats:
            error_msg = []
            if missing_prayers:
                error_msg.append(f"Missing prayers: {', '.join(missing_prayers)}")
            if invalid_formats:
                error_msg.append(f"Invalid time format: {', '.join(invalid_formats)}")

            logging.error("Prayer times validation failed: %s", "; ".join(error_msg))
            validation_error = ValidationError(
                "prayer_times",
                {
                    "missing_prayers": missing_prayers,
                    "invalid_formats": invalid_formats
                }
            )
            result = validation_error.to_dict()
            result["timestamp"] = datetime.now(self.tz).isoformat()
            return result

        logging.debug("Prayer times validation successful")
        return {
            "success": True,
            "message": "All prayer times are valid",
            "timestamp": datetime.now(self.tz).isoformat()
        }

    def _ensure_current_data(self, location: str, target_date: Optional[datetime] = None, force_download: bool = False) -> Dict[str, Any]:
        """
        Ensures timetable data is current for the target date.
        Downloads new data if needed.

        Args:
            location (str): Location to check
            target_date (datetime): Date to check data for (defaults to today)
            force_download (bool): Force download even if data is current

        Returns:
            dict: Result of ensuring current data
        """
        if target_date is None:
            target_date = datetime.now(self.tz)

        # Check if refresh is needed
        refresh_check = self._is_new_month(location, target_date)
        needs_refresh = force_download or refresh_check.get('needs_refresh', True)

        download_result = None
        if needs_refresh:
            logging.info("New month detected or force download requested. Downloading updated timetables.")
            if location == "icci":
                download_result = self._download_icci_timetable()
            else:
                download_result = self._download_naas_timetable()

            if not download_result.get('success', False):
                return {
                    "success": False,
                    "location": location,
                    "error": f"Failed to download {location.upper()} timetable",
                    "download_result": download_result,
                    "timestamp": datetime.now(self.tz).isoformat()
                }

        return {
            "success": True,
            "download_performed": download_result is not None,
            "download_result": download_result,
            "refresh_check": refresh_check
        }

    def _load_and_extract_times(self, location: str, target_date: Optional[datetime] = None) -> Dict[str, Any]:
        """
        Loads timetable file and extracts prayer times for the target date.

        Args:
            location (str): Location to load times for
            target_date (datetime): Date to extract times for (defaults to today)

        Returns:
            dict: Extracted prayer times or error
        """
        if target_date is None:
            target_date = datetime.now(self.tz)

        filename = self.icci_timetable_file if location == "icci" else self.naas_prayers_timetable_file

        if not os.path.exists(filename):
            logging.error(f"{location.upper()} timetable file not found.")
            return {
                "success": False,
                "location": location,
                "file_path": filename,
                "error": f"{location.upper()} timetable file not found",
                "timestamp": datetime.now(self.tz).isoformat()
            }

        try:
            with open(filename, "r", encoding="utf-8") as f:
                data = json.load(f)

            extraction_result = self._extract_today_prayers(data, location, target_date)

            if "error" in extraction_result:
                return {
                    "success": False,
                    "location": location,
                    "file_path": filename,
                    "error": extraction_result["error"],
                    "timestamp": datetime.now(self.tz).isoformat()
                }

            # Validate the extracted prayer times
            validation = self._validate_prayer_times(extraction_result)
            if not validation['success']:
                return {
                    "success": False,
                    "location": location,
                    "file_path": filename,
                    "error": validation['error'],
                    "validation_result": validation,
                    "timestamp": datetime.now(self.tz).isoformat()
                }

            return {
                "success": True,
                "location": location,
                "prayer_times": extraction_result,
                "file_path": filename,
                "timezone": str(self.tz),
                "date": target_date.strftime("%Y-%m-%d"),
                "timestamp": datetime.now(self.tz).isoformat()
            }

        except (json.JSONDecodeError, KeyError) as e:
            logging.error("Error loading %s timetable: %s", location.upper(), e)
            return {
                "success": False,
                "location": location,
                "file_path": filename,
                "error": f"Failed to load {location.upper()} timetable",
                "error_details": str(e),
                "error_type": "file_parsing_error",
                "timestamp": datetime.now(self.tz).isoformat()
            }

    def fetch_prayer_times(self, location: str = 'icci', target_date: Optional[datetime] = None, force_download: bool = False) -> Dict[str, Any]:
        """
        Fetches prayer times for the specified location and date.
        If it's a new month or force_download=True, it downloads a fresh timetable.

        Args:
            location (str): Location to fetch times for ('naas' or 'icci')
            target_date (datetime): Date to fetch prayer times for (defaults to today)
            force_download (bool): Force download even if file exists

        Returns:
            dict: JSON response with prayer times or error
        """
        if target_date is None:
            target_date = datetime.now(self.tz)

        # Step 1: Validate location
        validation = self._validate_location(location)
        if not validation['success']:
            return validation

        # Check cache first (unless force_download is set)
        if not force_download:
            cache_key = self._get_cache_key(location, target_date)
            cached_result = self._get_from_cache(cache_key)
            if cached_result:
                cached_result['from_cache'] = True
                return cached_result

        # Step 2: Ensure data is current
        update_result = self._ensure_current_data(location, target_date, force_download)
        if not update_result['success']:
            return update_result

        # Step 3: Load and extract prayer times
        extract_result = self._load_and_extract_times(location, target_date)

        # Add update metadata to the result
        if extract_result.get('success'):
            extract_result['download_performed'] = update_result.get('download_performed', False)
            extract_result['download_result'] = update_result.get('download_result')
            extract_result['refresh_check'] = update_result.get('refresh_check')
            extract_result['from_cache'] = False

            # Cache successful results
            cache_key = self._get_cache_key(location, target_date)
            self._put_in_cache(cache_key, extract_result)

        return extract_result

    def _extract_today_prayers(self, data: Any, location: str, target_date: Optional[datetime] = None) -> Dict[str, Any]:
        """
        Extracts prayer times for the target date from the provided timetable data.

        Args:
            data: The timetable data
            location (str): Location ('icci' or 'naas')
            target_date (datetime): Date to extract times for (defaults to today)

        Returns:
            dict: Prayer times or error information
        """
        if target_date is None:
            target_date = datetime.now(self.tz)

        today_day = str(target_date.day)
        today_month = str(target_date.month)

        logging.debug(f"Extracting prayer times for {location.upper()} on {today_day}-{today_month}")

        if location == "icci":
            if "timetable" not in data or today_month not in data["timetable"] or today_day not in data["timetable"][today_month]:
                logging.warning(f"ICCI data missing for {today_day}-{today_month}")
                return {"error": f"ICCI data missing for {today_day}-{today_month}"}

            day_prayers = data["timetable"][today_month][today_day]
            return {
                "Fajr": f"{day_prayers[0][0]:02}:{day_prayers[0][1]:02}",
                "Dhuhr": f"{day_prayers[2][0]:02}:{day_prayers[2][1]:02}",
                "Asr": f"{day_prayers[3][0]:02}:{day_prayers[3][1]:02}",
                "Maghrib": f"{day_prayers[4][0]:02}:{day_prayers[4][1]:02}",
                "Isha": f"{day_prayers[5][0]:02}:{day_prayers[5][1]:02}"
            }

        elif location == "naas":
            # Month in the Naas data is 0-indexed (January is 0, April is 3)
            month = target_date.month - 1

            # Verify data existence
            if month < 0 or month >= len(data):
                logging.error(f"Naas prayer data is missing for month {target_date.month}!")
                return {"error": f"Naas prayer data not found for month {target_date.month}"}

            if today_day not in data[month]:
                logging.error(f"No Naas prayer times found for day {today_day} in month {target_date.month}.")
                return {"error": f"No Naas prayer times found for day {today_day}"}

            # Log the prayer times for debugging
            prayer_times = data[month][today_day]
            logging.debug(f"Retrieved Naas times for {today_day}-{today_month}: {prayer_times}")

            return {
                "Fajr": prayer_times[0],
                "Dhuhr": prayer_times[2],
                "Asr": prayer_times[3],
                "Maghrib": prayer_times[4],
                "Isha": prayer_times[5]
            }

    def get_available_sources(self) -> Dict[str, Any]:
        """
        Get information about available prayer time sources.

        Returns:
            dict: JSON response with available sources
        """
        return {
            "success": True,
            "sources": {
                "icci": {
                    "name": "Islamic Cultural Centre of Ireland",
                    "url": self.sources["icci"],
                    "type": "api",
                    "file_path": self.icci_timetable_file
                },
                "naas": {
                    "name": "Naas Mosque (Mawaqit)",
                    "url": self.sources["naas"],
                    "type": "web_scraping",
                    "file_path": self.naas_prayers_timetable_file
                }
            },
            "timezone": str(self.tz),
            "data_directory": os.path.dirname(self.icci_timetable_file),
            "timestamp": datetime.now(self.tz).isoformat()
        }

    def get_file_status(self, location: Optional[str] = None) -> Dict[str, Any]:
        """
        Get status information about timetable files.

        Args:
            location (str): Specific location to check, or None for all

        Returns:
            dict: JSON response with file status
        """
        try:
            locations_to_check = [location] if location else ["icci", "naas"]
            file_status = {}

            for loc in locations_to_check:
                if loc not in ["icci", "naas"]:
                    continue

                file_path = self.icci_timetable_file if loc == "icci" else self.naas_prayers_timetable_file

                if os.path.exists(file_path):
                    stat = os.stat(file_path)
                    file_status[loc] = {
                        "exists": True,
                        "file_path": file_path,
                        "size_bytes": stat.st_size,
                        "modified_time": datetime.fromtimestamp(stat.st_mtime).isoformat(),
                        "is_current_month": datetime.fromtimestamp(stat.st_mtime).month == datetime.now().month
                    }
                else:
                    file_status[loc] = {
                        "exists": False,
                        "file_path": file_path
                    }

            return {
                "success": True,
                "file_status": file_status,
                "current_date": datetime.now(self.tz).isoformat(),
                "timestamp": datetime.now(self.tz).isoformat()
            }

        except Exception as e:
            return {
                "success": False,
                "error": str(e),
                "timestamp": datetime.now(self.tz).isoformat()
            }

    def force_refresh(self, location: Optional[str] = None) -> Dict[str, Any]:
        """
        Force refresh of timetable data for specified location(s).

        Args:
            location (str): Specific location to refresh, or None for all

        Returns:
            dict: JSON response with refresh results
        """
        locations_to_refresh = [location] if location else ["icci", "naas"]
        refresh_results = {}

        for loc in locations_to_refresh:
            if loc not in ["icci", "naas"]:
                refresh_results[loc] = {
                    "success": False,
                    "error": f"Invalid location: {loc}"
                }
                continue

            if loc == "icci":
                refresh_results[loc] = self._download_icci_timetable()
            else:
                refresh_results[loc] = self._download_naas_timetable()

        return {
            "success": all(result.get("success", False) for result in refresh_results.values()),
            "refresh_results": refresh_results,
            "timestamp": datetime.now(self.tz).isoformat()
        }


# Example usage and testing
if __name__ == "__main__":
    fetcher = PrayerTimesFetcher()

    print("=== Prayer Times Fetcher JSON API Demo ===\n")

    # Get available sources
    sources = fetcher.get_available_sources()
    print("Available sources:")
    print(json.dumps(sources, indent=2))

    # Get file status
    status = fetcher.get_file_status()
    print("\nFile status:")
    print(json.dumps(status, indent=2))

    # Fetch prayer times for Naas
    print("\nFetching Naas prayer times:")
    naas_prayers = fetcher.fetch_prayer_times("naas")
    print(json.dumps(naas_prayers, indent=2))

    # Fetch prayer times for ICCI
    print("\nFetching ICCI prayer times:")
    icci_prayers = fetcher.fetch_prayer_times("icci")
    print(json.dumps(icci_prayers, indent=2))