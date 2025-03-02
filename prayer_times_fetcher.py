import os
import requests
import json
import logging
from datetime import datetime
from dateutil import tz
from selenium import webdriver
from selenium.webdriver.chrome.options import Options
import chromedriver_autoinstaller
from bs4 import BeautifulSoup
from datetime import datetime
import re

# Configure logging
logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s")


class PrayerTimesFetcher:
    """
    A class to fetch prayer times for either Naas (via Mawaqit Web Scraping) or ICCI (via IslamIreland API).
    """

    def __init__(self):
        """
        Initializes the prayer time fetcher with API endpoints and timezone settings.
        """
        self.sources = {
            "icci": "https://islamireland.ie/api/timetable/",
            "naas": "https://mawaqit.net/en/m/-34"  # Used for Selenium scraping
        }
        self.naas_prayers_timetable_file = "naas_prayers_timetable.json"
        self.icci_timetable_file = "icci_timetable.json"
        self.tz = tz.gettz("Europe/Dublin")  # Set timezone to Dublin

    def _download_icci_timetable(self):
        """
        Downloads the latest ICCI timetable and saves it as icci_timetable.json.
        """
        try:
            response = requests.get(self.sources['icci'], timeout=10)
            response.raise_for_status()
            with open(self.icci_timetable_file, "w", encoding="utf-8") as f:
                json.dump(response.json(), f, indent=4)
            logging.info("ICCI timetable downloaded and saved successfully.")
        except requests.RequestException as e:
            logging.error("Failed to download ICCI timetable: %s", e)
            return False
        return True
    
    def _is_new_month(self):
        """
        Checks if it's the start of a new month based on the file modification date of ICCI timetable.
        """
        if not os.path.exists(self.icci_timetable_file) or not os.path.exists(self.naas_prayers_timetable_file):
            return True
        try:
            # Get the file modification time
            file_mod_time = datetime.fromtimestamp(os.path.getmtime(self.icci_timetable_file))
            current_month = datetime.today().month
            return file_mod_time.month != current_month
        except Exception as e:
            logging.error("Error reading ICCI timetable file modification date: %s", e)
            return True

    def fetch_prayer_times(self, location: str):
        """
        Fetches prayer times from the ICCI timetable file or downloads it if necessary.

        :param location: 'naas' or 'icci' to specify the data source.
        :return: Dictionary containing today's prayer times.
        :raises ValueError: If an invalid location is provided.
        """
        if location not in ["naas", "icci"]:
            logging.error("Invalid location provided: %s", location)
            raise ValueError("Invalid location. Choose either 'naas' or 'icci'.")

        logging.info("Fetching prayer times for %s", location)
        
        # Check if we need to download a new timetable
        if self._is_new_month():
            self._download_icci_timetable()
            self._download_naas_timetable()

        if location == "icci":
            # Load the stored timetable
            if os.path.exists(self.icci_timetable_file):
                try:
                    with open(self.icci_timetable_file, "r", encoding="utf-8") as f:
                        data = json.load(f)
                    return self._extract_today_prayers(data, location)
                except (json.JSONDecodeError, KeyError) as e:
                    logging.error("Error loading ICCI timetable: %s", e)
                    return {"error": "Failed to load ICCI timetable."}
            else:
                return {"error": "ICCI timetable file not found."}
        
        elif location == "naas":
            if os.path.exists(self.naas_prayers_timetable_file):
                try:
                    with open(self.naas_prayers_timetable_file, "r", encoding="utf-8") as f:
                        data = json.load(f)
                    return self._extract_today_prayers(data, location)
                except (json.JSONDecodeError, KeyError) as e:
                    logging.error("Error loading ICCI timetable: %s", e)
                    return {"error": "Failed to load ICCI timetable."}

    def _download_naas_timetable(self):
        """
        Fetches prayer timetable data from the given URL, extracts the 'calendar' list, and saves it to a JSON file.
        """
        try:
            # Step 1: Fetch the page content
            response = requests.get(self.sources['naas'])
            response.raise_for_status()  # Raise error if request fails
            # Step 2: Parse HTML content
            soup = BeautifulSoup(response.text, "html.parser")
            script_tags = soup.find_all("script")
            # Step 3: Extract calendar data
            calendar_data = None
            for script in script_tags:
                if "calendar" in script.text:
                    match = re.search(r'"calendar"\s*:\s*(\[\{.*?\}\])', script.text, re.DOTALL)
                    if match:
                        calendar_data = match.group(1)
                    break  # Stop once found
            if not calendar_data:
                print("❌ Calendar data not found in the page!")
                return
            # Step 4: Convert extracted string to JSON
            calendar_list = json.loads(calendar_data)
            # Step 5: Save to JSON file
            with open(self.naas_prayers_timetable_file, "w", encoding="utf-8") as file:
                json.dump(calendar_list, file, indent=2, ensure_ascii=False)
            print(f"✅ Prayer timetable saved successfully to {self.naas_prayers_timetable_file}")
        except requests.RequestException as e:
            print(f"❌ Error fetching data: {e}")
        except json.JSONDecodeError as e:
            print(f"❌ Error parsing JSON: {e}")


    def _extract_today_prayers(self, data, location: str):
        """
        Extracts today's prayer times from the ICCI API response.

        :param data: JSON data from the API.
        :param location: 'icci'.
        :return: Dictionary containing today's prayer times.
        """
        today_date = datetime.now(self.tz)
        today_day = str(today_date.day)  # Extract day as a string (e.g., "1", "2", ...)
        today_month = str(today_date.month)  # Extract month as a string (e.g., "3" for March)
        if location == "icci":

            logging.info("Extracting prayer times for ICCI (Date: %s-%s)", today_day, today_month)

            # Ensure response contains 'timetable'
            if "timetable" not in data:
                logging.error("ICCI API response does not contain 'timetable': %s", data)
                return {"error": "ICCI API returned an unexpected format"}

            timetable = data["timetable"]

            # Ensure the current month exists in the timetable
            if today_month not in timetable:
                logging.error("ICCI API response does not contain data for month: %s", today_month)
                return {"error": f"No prayer times found for month {today_month}."}

            month_data = timetable[today_month]

            # Ensure the current day exists in the month's data
            if today_day not in month_data:
                logging.warning("ICCI API response does not contain data for day: %s", today_day)
                return {"error": f"No prayer times found for {today_date.strftime('%Y-%m-%d')}."}

            day_prayers = month_data[today_day]

            # Extract prayer times (ICCI's order: Fajr, Sunrise, Dhuhr, Asr, Maghrib, Isha)
            if len(day_prayers) < 6:
                logging.error("Unexpected prayer times format for ICCI on %s", today_date.strftime("%Y-%m-%d"))
                return {"error": "Prayer times format mismatch from ICCI API"}

            return {
                "Fajr": f"{day_prayers[0][0]:02}:{day_prayers[0][1]:02}",
                "Dhuhr": f"{day_prayers[2][0]:02}:{day_prayers[2][1]:02}",
                "Asr": f"{day_prayers[3][0]:02}:{day_prayers[3][1]:02}",
                "Maghrib": f"{day_prayers[4][0]:02}:{day_prayers[4][1]:02}",
                "Isha": f"{day_prayers[5][0]:02}:{day_prayers[5][1]:02}"
            }
        elif location == "naas":
            month = today_date.month - 1
            return {
                "Fajr": f"{data[month][today_day][0]}",
                "Dhuhr": f"{data[month][today_day][2]}",
                "Asr": f"{data[month][today_day][3]}",
                "Maghrib": f"{data[month][today_day][4]}",
                "Isha": f"{data[month][today_day][5]}"
            }
            


    def get_prayer_times(self, location: str):
        """
        Public method to fetch and return today's prayer times.

        :param location: 'naas' or 'icci'.
        :return: Dictionary containing today's date and prayer times.
        """
        try:
            prayers = self.fetch_prayer_times(location)
            if "error" in prayers:
                return prayers  # Return error message

            return {
                "date": datetime.now(self.tz).strftime("%Y-%m-%d"),
                "prayers": prayers
            }
        except Exception as e:
            logging.error("Error retrieving prayer times: %s", e)
            return {"error": str(e)}


# Example usage
if __name__ == "__main__":
    fetcher = PrayerTimesFetcher()

    # Fetch prayer times for Naas (Scraped)
    naas_prayers = fetcher.get_prayer_times("naas")
    print(json.dumps(naas_prayers, indent=4))

    # Fetch prayer times for ICCI (API)
    icci_prayers = fetcher.get_prayer_times("icci")
    print(json.dumps(icci_prayers, indent=4))
