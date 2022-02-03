#! /usr/bin/env python3

#this script will only work for one day.
#thus you need to add the below crontab records to your system:
#you can edit cron via this command: crontab -e
#1 0 * * * /scripts/update_prayers_times.py
#@reboot /scripts/update_prayers_times.py

from requests import get, post
from datetime import date, datetime

#you need to install schedule via pip3
#https://pypi.org/project/schedule/
import schedule

import time
import json

#homeassistant have to be up and running somewhere in your network
#This script will only be able to utilize the devices that are
#discovered in homeassistant.
#you need to enable the REST API on you homeassistant
# https://www.home-assistant.io/integrations/api
hass_node = "192.168.86.111"
hass_port = "8123"

#you need to get your own entity ID, this won't work with you.
#find the entity id in the home assistant configuration > devices & services > entities
device_entity_id = 'media_player.all_speakers'

icci_url = 'https://islamireland.ie/api/timetable/'

#get one for your user from homeassistant
#https://developers.home-assistant.io/docs/auth_api/#long-lived-access-token
hass_api_token = 'eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiI5Y2Y0NWQyNWNkNjc0MjUxYTNhNjNkZGFhOTdmNWViYyIsImlhdCI6MTY0MzY4NTgxMCwiZXhwIjoxOTU5MDQ1ODEwfQ.1g8CUn-WdmE3078irFJKorcUeqo6b1BJrW4-LOBkMds'
auth_value = 'Bearer {}'.format(hass_api_token)

def get_azan_times():
    azan_times_url = 'https://3kdru4h1tg.execute-api.eu-west-1.amazonaws.com/default/ICCI_next_prayer'
    azan_times = get(azan_times_url).json()
    today_timetable = azan_times["today prayers"]
    return today_timetable

def execute_azan_on_device():
    azan_url = 'https://www.gurutux.com/media/azan.mp3'
    api_uri = 'api/services/media_player/play_media'
    header = {'content-type': 'application/json', 'Authorization': auth_value}
    data = {'entity_id': device_entity_id}
    data['media_content_id'] = azan_url
    data['media_content_type'] = 'music'
    uri = "http://{}:{}/{}".format(hass_node, hass_port, api_uri)
    response = post(uri, headers=header, json=data)
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
