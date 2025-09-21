@echo off
REM Automated Azan - Windows Build Script with Nuitka
REM Optimized for Windows 10/11 x64

echo 🚀 Automated Azan - Windows Build
echo ================================

REM Check if Python is available
python --version >nul 2>&1
if errorlevel 1 (
    echo ❌ Python not found. Please install Python 3.11 or later.
    pause
    exit /b 1
)

REM Check if UV is available
uv --version >nul 2>&1
if errorlevel 1 (
    echo 📦 Installing UV...
    pip install uv
)

REM Install build dependencies
echo 📦 Installing build dependencies...
uv pip install -e ".[build,gui]"

REM Check for C compiler (Visual Studio or MinGW)
cl >nul 2>&1
if errorlevel 1 (
    gcc --version >nul 2>&1
    if errorlevel 1 (
        echo ❌ No C compiler found!
        echo.
        echo Please install one of:
        echo   - Visual Studio Build Tools (recommended)
        echo   - MinGW-w64
        echo   - Visual Studio Community
        echo.
        echo Download: https://visualstudio.microsoft.com/downloads/
        pause
        exit /b 1
    ) else (
        echo ✅ MinGW GCC compiler found
    )
) else (
    echo ✅ Visual Studio C compiler found
)

REM Create output directory
if not exist "dist" mkdir dist

REM Run Nuitka build
echo 🔨 Compiling with Nuitka...
echo This will take 5-15 minutes...

uv run python -m nuitka ^
    --standalone ^
    --onefile ^
    --assume-yes-for-downloads ^
    --lto=yes ^
    --enable-plugin=anti-bloat ^
    --include-data-dir=Media=Media ^
    --include-data-file=adahn.config.example=adahn.config.example ^
    --include-package=flask ^
    --include-package=flask_socketio ^
    --include-package=pychromecast ^
    --include-package=schedule ^
    --include-package=requests ^
    --include-package=beautifulsoup4 ^
    --include-package=python_dateutil ^
    --include-package=dotenv ^
    --include-package-data=pystray ^
    --include-package-data=PIL ^
    --windows-icon-from-ico=Media/azan.ico ^
    --windows-company-name="Automated Azan" ^
    --windows-product-name="Automated Azan" ^
    --windows-file-version=1.0.0 ^
    --windows-product-version=1.0.0 ^
    --windows-file-description="Islamic Prayer Time Scheduler" ^
    --output-filename=AutomatedAzan.exe ^
    --output-dir=dist ^
    main.py

if errorlevel 1 (
    echo ❌ Build failed!
    echo.
    echo Common solutions:
    echo   - Ensure C compiler is properly installed
    echo   - Check all dependencies are available
    echo   - Try running with --verbose for more details
    pause
    exit /b 1
)

REM Check if build succeeded
if exist "dist\AutomatedAzan.exe" (
    echo.
    echo 🎉 Build successful!
    echo 📦 Executable: dist\AutomatedAzan.exe

    REM Get file size
    for %%A in ("dist\AutomatedAzan.exe") do (
        set /a size=%%~zA/1024/1024
        echo 💾 Size: !size! MB
    )

    echo.
    echo 🧪 Testing executable...
    "dist\AutomatedAzan.exe" --help >nul 2>&1
    if errorlevel 1 (
        echo ⚠️  Executable test failed (may be normal)
    ) else (
        echo ✅ Executable test passed
    )

    echo.
    echo 🎯 Next steps:
    echo   1. Test: dist\AutomatedAzan.exe --no-tray --debug
    echo   2. Copy adahn.config.example to adahn.config and configure
    echo   3. Distribute dist\AutomatedAzan.exe to users
    echo.
) else (
    echo ❌ Build failed - executable not found
    pause
    exit /b 1
)

pause