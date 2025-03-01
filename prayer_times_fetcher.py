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
        self.tz = tz.gettz("Europe/Dublin")  # Set timezone to Dublin

    def fetch_prayer_times(self, location: str):
        """
        Fetches prayer times from the selected API endpoint or via web scraping.

        :param location: 'naas' or 'icci' to specify the data source.
        :return: Dictionary containing today's prayer times.
        :raises ValueError: If an invalid location is provided.
        :raises ConnectionError: If the API request fails.
        """
        if location not in self.sources:
            logging.error("Invalid location provided: %s", location)
            raise ValueError("Invalid location. Choose either 'naas' or 'icci'.")

        logging.info("Fetching prayer times for %s", location)

        if location == "icci":
            api_url = self.sources[location]
            try:
                response = requests.get(api_url, timeout=10)
                response.raise_for_status()
                return self._extract_today_prayers(response.json(), location)
            except requests.RequestException as e:
                logging.error("Failed to fetch data from %s: %s", api_url, e)
                return {"error": f"Failed to fetch data from {api_url}"}

        elif location == "naas":
            return self._scrape_naas_prayers()

    def _scrape_naas_prayers(self):
        """
        Scrapes prayer times for Naas from the Mawaqit website using Selenium and BeautifulSoup.
        
        :return: Dictionary containing today's prayer times in 24-hour format.
        """
        logging.info("Starting web scraping for Naas prayer times...")

        # Automatically install ChromeDriver
        chromedriver_autoinstaller.install()

        # Set Chrome options
        chrome_options = Options()
        chrome_options.add_argument("--headless")  # Run in headless mode
        chrome_options.add_argument("--no-sandbox")
        chrome_options.add_argument("--disable-dev-shm-usage")

        # Start WebDriver
        driver = webdriver.Chrome(options=chrome_options)

        try:
            # Open the webpage
            driver.get(self.sources["naas"])

            # Extract page source after JavaScript execution
            html = driver.page_source
        except Exception as e:
            logging.error("Failed to load Naas prayer times page: %s", e)
            return {"error": "Failed to load Naas prayer times page."}
        finally:
            # Close the WebDriver
            driver.quit()

        # Parse HTML with BeautifulSoup
        soup = BeautifulSoup(html, "html.parser")

        # Find the prayers section
        prayers_section = soup.find(class_="prayers")
        prayer_times = {}

        if prayers_section:
            prayers = prayers_section.find_all("div", recursive=False)  # Get direct children only
            for prayer in prayers:
                name_tag = prayer.find(class_="name")
                time_tag = prayer.find(class_="time")
                if name_tag and time_tag:
                    prayer_name = name_tag.text.strip()
                    prayer_time = " ".join(time_tag.stripped_strings)  # Flatten text including AM/PM

                    # Ensure correct AM/PM format
                    prayer_time = re.sub(r"\s+", " ", prayer_time).strip()  # Remove extra spaces
                    prayer_time = prayer_time.replace("A M", "AM").replace("P M", "PM")  # Fix AM/PM format

                    # Convert to 24-hour format
                    try:
                        prayer_time_24h = datetime.strptime(prayer_time, "%I:%M %p").strftime("%H:%M")
                        prayer_times[prayer_name] = prayer_time_24h
                    except ValueError:
                        logging.error("Invalid time format for Naas prayer: '%s'", prayer_time)
                        prayer_times[prayer_name] = prayer_time  # Keep original if conversion fails

            logging.info("Successfully scraped prayer times for Naas")
            return prayer_times
        else:
            logging.warning("Could not find prayer times section in Naas page.")
            return {"error": "Could not extract prayer times for Naas."}

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
