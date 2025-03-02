import os
import requests
import json
import logging
from datetime import datetime
from dateutil import tz
from bs4 import BeautifulSoup
import re

# Configure logging with debug level
logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s")


class PrayerTimesFetcher:
    def __init__(self):
        """
        Initializes the prayer time fetcher with sources, file paths, and timezone settings.
        """
        self.sources = {
            "icci": "https://islamireland.ie/api/timetable/",  # API source for ICCI
            "naas": "https://mawaqit.net/en/m/-34"  # Web scraping source for Naas
        }
        self.naas_prayers_timetable_file = "naas_prayers_timetable.json"
        self.icci_timetable_file = "icci_timetable.json"
        self.tz = tz.gettz("Europe/Dublin")  # Set timezone to Dublin

    def _download_icci_timetable(self):
        """
        Fetches and saves the ICCI prayer timetable from the API.
        """
        logging.debug("Attempting to download ICCI timetable.")
        try:
            response = requests.get(self.sources['icci'], timeout=10)
            response.raise_for_status()  # Raise error if response is not successful
            with open(self.icci_timetable_file, "w", encoding="utf-8") as f:
                json.dump(response.json(), f, indent=4)
            logging.info("✅ ICCI timetable downloaded and saved successfully.")
            return True
        except requests.RequestException as e:
            logging.error("❌ Failed to download ICCI timetable: %s", e)
            return False
    
    def _download_naas_timetable(self):
        """
        Fetches prayer timetable data for Naas using web scraping.
        Extracts 'calendar' data from the webpage and saves it to a JSON file.
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
                    break  # Stop searching once we find the calendar data
            
            if not calendar_data:
                logging.error("❌ Calendar data not found in Naas webpage!")
                return False

            calendar_list = json.loads(calendar_data)
            with open(self.naas_prayers_timetable_file, "w", encoding="utf-8") as file:
                json.dump(calendar_list, file, indent=2, ensure_ascii=False)

            logging.info("✅ Naas prayer timetable saved successfully.")
            return True
        except requests.RequestException as e:
            logging.error("❌ Error fetching Naas data: %s", e)
            return False
        except json.JSONDecodeError as e:
            logging.error("❌ Error parsing Naas JSON: %s", e)
            return False

    def _is_new_month(self):
        """
        Determines if the timetable should be refreshed by checking if a new month has started.
        Also, checks if timetable files exist.
        """
        logging.debug("Checking if a new month has started or timetable files are missing.")
        if not os.path.exists(self.icci_timetable_file) or not os.path.exists(self.naas_prayers_timetable_file):
            logging.info("One or both timetable files are missing. Need to download new ones.")
            return True  # Need to download a new timetable

        try:
            file_mod_time = datetime.fromtimestamp(os.path.getmtime(self.icci_timetable_file))
            current_month = datetime.today().month
            is_new_month = file_mod_time.month != current_month
            logging.debug(f"Timetable last modified in month: {file_mod_time.month}, Current month: {current_month}")
            return is_new_month
        except Exception as e:
            logging.error("Error reading timetable file modification date: %s", e)
            return True  # If any error occurs, assume we need to re-download

    def fetch_prayer_times(self, location: str = 'icci'):
        """
        Fetches today's prayer times for the specified location ('naas' or 'icci').
        If it's a new month, it downloads a fresh timetable.
        """
        if location not in ["naas", "icci"]:
            logging.error("Invalid location provided: %s", location)
            raise ValueError("Invalid location. Choose either 'naas' or 'icci'.")

        logging.info("Fetching prayer times for %s", location)

        if self._is_new_month():
            logging.info("New month detected. Downloading updated timetables.")
            if not self._download_icci_timetable() or not self._download_naas_timetable():
                return {"error": "Failed to download prayer timetable."}

        filename = self.icci_timetable_file if location == "icci" else self.naas_prayers_timetable_file
        if not os.path.exists(filename):
            logging.error(f"{location.upper()} timetable file not found.")
            return {"error": f"{location.upper()} timetable file not found."}

        try:
            with open(filename, "r", encoding="utf-8") as f:
                data = json.load(f)
            return self._extract_today_prayers(data, location)
        except (json.JSONDecodeError, KeyError) as e:
            logging.error("Error loading %s timetable: %s", location.upper(), e)
            return {"error": f"Failed to load {location.upper()} timetable."}

    def _extract_today_prayers(self, data, location: str):
        """
        Extracts today's prayer times from the provided timetable data.
        """
        today_date = datetime.now(self.tz)
        today_day = str(today_date.day)
        today_month = str(today_date.month)

        logging.debug(f"Extracting prayer times for {location.upper()} on {today_day}-{today_month}")

        if location == "icci":
            if "timetable" not in data or today_month not in data["timetable"] or today_day not in data["timetable"][today_month]:
                logging.warning(f"ICCI data missing for {today_day}-{today_month}")
                return {"error": "ICCI data missing for today."}

            day_prayers = data["timetable"][today_month][today_day]
            return {
                "Fajr": f"{day_prayers[0][0]:02}:{day_prayers[0][1]:02}",
                "Dhuhr": f"{day_prayers[2][0]:02}:{day_prayers[2][1]:02}",
                "Asr": f"{day_prayers[3][0]:02}:{day_prayers[3][1]:02}",
                "Maghrib": f"{day_prayers[4][0]:02}:{day_prayers[4][1]:02}",
                "Isha": f"{day_prayers[5][0]:02}:{day_prayers[5][1]:02}"
            }

        elif location == "naas":
            month = today_date.month - 1  # Adjust if needed based on JSON format
            if month < 0 or month >= len(data):
                logging.error("Naas prayer data is missing for the current month!")
                return {"error": "Naas prayer data not found for this month."}
            
            if today_day not in data[month]:
                logging.error(f"No Naas prayer times found for day {today_day}.")
                return {"error": f"No Naas prayer times found for {today_day}."}

            return {
                "Fajr": data[month][today_day][0],
                "Dhuhr": data[month][today_day][2],
                "Asr": data[month][today_day][3],
                "Maghrib": data[month][today_day][4],
                "Isha": data[month][today_day][5]
            }

# Example usage
if __name__ == "__main__":
    fetcher = PrayerTimesFetcher()

    # Fetch prayer times for Naas
    naas_prayers = fetcher.fetch_prayer_times("naas")
    print(json.dumps(naas_prayers, indent=4))

    # Fetch prayer times for ICCI
    icci_prayers = fetcher.fetch_prayer_times("icci")
    print(json.dumps(icci_prayers, indent=4))
