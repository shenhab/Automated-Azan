#!/usr/bin/env bash
virtualenv environment
export PYTHONPATH=
./environment/bin/pip install -r requirements.pip
source environment/bin/activate