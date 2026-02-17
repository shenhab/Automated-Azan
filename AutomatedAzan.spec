# -*- mode: python ; coding: utf-8 -*-

from PyInstaller.utils.hooks import collect_all

# List of packages you want to collect all for
packages = [
'aiohappyeyeballs',
'aiohttp',
'aiosignal',
'attrs',
'bidict',
'blinker',
'bs4',
'casttube',
'certifi',
'click',
'flask',
'flask_socketio',
'frozenlist',
'h11',
'idna',
'ifaddr',
'itsdangerous',
'jinja2',
'markupsafe',
'multidict',
'propcache',
'pychromecast',
'pyjwt',
'dateutil',
'dotenv',
'engineio',
'socketio',
'pytz',
'requests',
'schedule',
'soupsieve',
'werkzeug',
'wsproto',
'yarl',
'zeroconf',
'pystray',
'pillow'
]

all_datas = []
all_binaries = []
all_hiddenimports = []

for pkg in packages:
    datas, binaries, hiddenimports = collect_all(pkg)
    all_datas += datas
    all_binaries += binaries
    all_hiddenimports += hiddenimports

a = Analysis(
    ['main.py'],
    pathex=[],
    binaries=all_binaries,
    datas=[
        ('*.py', '.'),
        ('adahn.config', '.'),
        ('media/*', 'media'),
        ('templates/*', 'templates'),
        ('static/**/*', 'static'),
        ('data/*.json', 'data')
    ] + all_datas,
    hiddenimports=all_hiddenimports,
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=[],
    noarchive=False,
    optimize=0,
)

pyz = PYZ(a.pure)

exe = EXE(
    pyz,
    a.scripts,
    a.binaries,
    a.datas,
    [],
    name='AutomatedAzan',
    debug=False,
    bootloader_ignore_signals=False,
    strip=False,
    upx=True,
    upx_exclude=[],
    runtime_tmpdir=None,
    console=False,
    disable_windowed_traceback=False,
    argv_emulation=False,
    target_arch=None,
    codesign_identity=None,
    entitlements_file=None,
    icon='media/azan.ico',
)
