#! /usr/bin/env python3
from requests import get, post
from datetime import date, datetime
import schedule
import time
import json

hass_node = "192.168.86.111"
hass_port = "8123"
device_entity_id = 'media_player.all_speakers'

icci_url = 'https://islamireland.ie/api/timetable/'
hass_api_token = 'eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiI5Y2Y0NWQyNWNkNjc0MjUxYTNhNjNkZGFhOTdmNWViYyIsImlhdCI6MTY0MzY4NTgxMCwiZXhwIjoxOTU5MDQ1ODEwfQ.1g8CUn-WdmE3078irFJKorcUeqo6b1BJrW4-LOBkMds'
auth_value = 'Bearer {}'.format(hass_api_token)

def get_azan_times():
    today = date.today()
    icci_timetable = get(icci_url).json()['timetable']
    today_timetable = icci_timetable[str(today.month)][str(today.day)]
    return {'fajr_time': today_timetable[0],
            'Duhur_time': today_timetable[2],
            'Asr_time': today_timetable[3],
            'maghreb_time': today_timetable[4],
            'Isha_time': today_timetable[5]}

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
