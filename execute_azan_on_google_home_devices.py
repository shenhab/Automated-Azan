#! /usr/bin/env python3

#this script will only work for one day.
#thus you need to add the below crontab records to your system:
#you can edit cron via this command: crontab -e
#1 0 * * * /scripts/update_prayers_times.py
#@reboot /scripts/update_prayers_times.py

from requests import get
from datetime import datetime
import time

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
    azan_times = get(azan_times_url).json()
    today_timetable = azan_times["today prayers"]
    return today_timetable

def execute_azan_on_device():
    azan_url = 'https://www.gurutux.com/media/azan.mp3'
    chromecasts, browser = pychromecast.get_listed_chromecasts(friendly_names=[google_home_device_name])
    cast = chromecasts[0]
    cast.wait()
    cast_media_controler = cast.media_controller
    cast_media_controler.play_media(azan_url, 'audio/mp3')
    pychromecast.discovery.stop_discovery(browser)
    return schedule.CancelJob

def scheduler():
    azan_times = get_azan_times()
    now = datetime.now()
    for prayer, azan_time in azan_times.items():
        if azan_time[0] > now.hour:
            schedule.every().day.at('{:02}:{:02}'.format(azan_time[0],azan_time[1])).do(execute_azan_on_device)
        elif azan_time[0] == now.hour and azan_time[1] > now.minute:
            schedule.every().day.at('{:02}:{:02}'.format(azan_time[0],azan_time[1])).do(execute_azan_on_device)

def executer():
    scheduler()
    while True:
        schedule.run_pending()
        n = schedule.idle_seconds()
        if n is None:
            break
        elif n > 0:
            time.sleep(n)

executer()
exit
