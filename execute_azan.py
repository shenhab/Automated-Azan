#! /usr/bin/env python3

from requests import get, post
from datetime import date
import schedule
import time
import json

hass_node = "192.168.86.111"
hass_port = "8123"
hass_api_token = 'eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiI5Y2Y0NWQyNWNkNjc0MjUxYTNhNjNkZGFhOTdmNWViYyIsImlhdCI6MTY0MzY4NTgxMCwiZXhwIjoxOTU5MDQ1ODEwfQ.1g8CUn-WdmE3078irFJKorcUeqo6b1BJrW4-LOBkMds'
auth_value = 'Bearer {}'.format(hass_api_token)

icci_url = 'https://islamireland.ie/api/timetable/'

def get_azan_times():
    today = date.today()
    icci_timetable = get(icci_url).json()['timetable']
    today_timetable = icci_timetable[str(today.month)][str(today.day)]
    return {'fajr_time': today_timetable[0],
            'Duhur_time': today_timetable[2],
            'Asr_time': today_timetable[3],
            'maghreb_time': today_timetable[4],
            'Isha_time': today_timetable[5]}


def execute_azan():
    api_uri = 'api/services/script/turn_on'
    header = {'content-type': 'application/json', 'Authorization': auth_value}
    data = {'entity_id': 'script.azan'}
    uri = "http://{}:{}/{}".format(hass_node, hass_port, api_uri)
    response = post(uri, headers=header, json=data)
    return schedule.CancelJob


def scheduler():
    azan_times = get_azan_times()
    for prayer, azan_time in azan_times.items():
        schedule.every().day.at('{:02}:{:02}'.format(azan_time[0],azan_time[1])).do(execute_azan)


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
