#! /usr/bin/env python3

#this script will only work for one day.
#thus you need to add the below crontab records to your system:
#you can edit cron via this command: crontab -e
#1 0 * * * /scripts/update_prayers_times.py
#@reboot /scripts/update_prayers_times.py

from requests import get
from datetime import datetime
import dateutil.tz
import time, pause
import logging
logging_format = '%(asctime)s [%(levelname)s]: %(message)s'
logging.basicConfig(format=logging_format, filename="/var/log/azan_service.log", level=10)


#you need to install schedule via pip3
#https://pypi.org/project/schedule/
import schedule

#you need to install pychromecast via pip3
#https://pypi.org/project/pychromecast/
import pychromecast

#you need to get your own google home device name, as the below name won't work with you.
#find the device name from google home app (a speaker group name can be used)
google_home_device_name = 'All Speakers'


def get_azan_times():
    azan_times_url = 'https://3kdru4h1tg.execute-api.eu-west-1.amazonaws.com/default/ICCI_next_prayer'
    azan_times = get(azan_times_url)
    today_timetable = azan_times.json()["today prayers"]
    logging.debug('get azan times url status code: {}'.format(azan_times.status_code))
    logging.debug('get azan times url status data: {}'.format(azan_times.json()))
    return today_timetable


def execute_azan_on_device(prayer):
    if prayer == "Al Fajr":
        azan_url = 'https://www.gurutux.com/media/adhan_al_fajr.mp3'
        volume = 0.2
        logging.debug('Adhan Al Fajr.')
    else:
        azan_url = 'https://www.gurutux.com/media/azan.mp3'
        volume = 1
        logging.debug('Regular Adhan.')
    
    logging.debug('**Salat {}.**'.format(prayer))
    if google_home_device_name == 'All Speakers' :
        chromecast_devices =  pychromecast.get_chromecasts()[0]
    else:
        chromecast_devices, browser = pychromecast.get_listed_chromecasts(friendly_names = [google_home_device_name], timeout=5)

    for casting_device in chromecast_devices:
        casting_device.logger.setLevel(20)
        casting_device.wait()
        cast_media_controller = casting_device.media_controller
        cast_media_controller.play_media(azan_url, 'audio/mp3')
        #casting_device.set_volume(volume)
        cast_media_controller.block_until_active()
    return schedule.CancelJob


def scheduler():
    azan_times = get_azan_times()
    dublin_tz = dateutil.tz.gettz('Europe/Dublin')
    now = datetime.now(tz=dublin_tz)
    logging.debug('Generating today\'s jobs.')
    for prayer, azan_time in azan_times.items():
        if azan_time[0] > now.hour and prayer != 'Al Duha':
            if prayer == "Al Fajr":
                schedule.every().day.at('{:02}:{:02}'.format(azan_time[0],azan_time[1])).do(execute_azan_on_device, prayer)
            else:
                schedule.every().day.at('{:02}:{:02}'.format(azan_time[0],azan_time[1])).do(execute_azan_on_device, prayer)
        elif azan_time[0] == now.hour and azan_time[1] > now.minute and prayer != 'Al Duha':
            if prayer == "Al Fajr":
                schedule.every().day.at('{:02}:{:02}'.format(azan_time[0],azan_time[1])).do(execute_azan_on_device, prayer)
            else:
                schedule.every().day.at('{:02}:{:02}'.format(azan_time[0],azan_time[1])).do(execute_azan_on_device, prayer)
    logging.debug('jobs generated.')


def executer():
    scheduler()
    logging.debug('Generated jobs: {}'.format(schedule.get_jobs()))
    while True:
        logging.debug('running pending jobs.')
        schedule.run_pending()
        n = schedule.idle_seconds()
        if n is None:
            break
        elif n > 0:
            logging.debug('sleeping for {} hours.'.format(n/60/60))
            time.sleep(n)


def sleep_till_midnight():
    dublin_tz = dateutil.tz.gettz('Europe/Dublin')
    now = datetime.now(tz=dublin_tz)
    logging.debug('sleeping till midnight')
    pause.until(datetime(now.year, now.month, now.day+1, 0, 5))


while True:
    logging.debug('calling_executer')
    executer()
    sleep_till_midnight()
