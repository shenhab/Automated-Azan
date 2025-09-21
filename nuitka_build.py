#!/usr/bin/env python3
"""
Nuitka Build Script for Automated Azan
Optimized compilation with platform detection and dependency handling
"""

import os
import sys
import platform
import subprocess
from pathlib import Path

def get_platform_info():
    """Get current platform information"""
    system = platform.system().lower()
    machine = platform.machine().lower()
    return system, machine

def get_nuitka_command():
    """Build optimized Nuitka command for current platform"""
    system, machine = get_platform_info()

    base_cmd = [
        sys.executable, "-m", "nuitka",
        "--standalone",
        "--onefile",
        "--assume-yes-for-downloads",

        # Performance optimizations
        "--lto=yes",
        "--enable-plugin=anti-bloat",

        # Include data files
        "--include-data-dir=Media=Media",
        "--include-data-file=adahn.config.example=adahn.config.example",

        # Python package inclusions
        "--include-package=flask",
        "--include-package=flask_socketio",
        "--include-package=pychromecast",
        "--include-package=schedule",
        "--include-package=requests",
        "--include-package=beautifulsoup4",
        "--include-package=python_dateutil",
        "--include-package=dotenv",

        # Optional GUI packages (with error handling)
        "--include-package-data=pystray",
        "--include-package-data=PIL",

        # Disable unnecessary features for smaller size
        "--disable-console",  # Remove for debugging

        # Main script
        "main.py"
    ]

    # Platform-specific optimizations
    if system == "windows":
        base_cmd.extend([
            "--windows-icon-from-ico=Media/azan.ico",
            "--windows-company-name=Automated Azan",
            "--windows-product-name=Automated Azan",
            "--windows-file-version=1.0.0",
            "--windows-product-version=1.0.0",
            "--windows-file-description=Islamic Prayer Time Scheduler",
        ])
        output_name = "AutomatedAzan.exe"

    elif system == "darwin":  # macOS
        base_cmd.extend([
            "--macos-app-icon=Media/azan.ico",
            "--macos-app-name=Automated Azan",
            "--macos-app-version=1.0.0",
        ])
        output_name = "AutomatedAzan"

    else:  # Linux and others
        output_name = "AutomatedAzan"

    base_cmd.extend(["--output-filename", output_name])

    return base_cmd

def check_dependencies():
    """Check if all build dependencies are available"""
    print("🔍 Checking build dependencies...")

    # Check if Nuitka is installed
    try:
        result = subprocess.run([sys.executable, "-m", "nuitka", "--version"],
                              capture_output=True, text=True)
        print(f"✅ Nuitka {result.stdout.strip()} installed")
    except subprocess.CalledProcessError:
        print("❌ Nuitka not installed. Run: uv pip install nuitka")
        return False

    # Check for C compiler
    system, _ = get_platform_info()
    if system == "windows":
        # Check for MSVC or MinGW
        compilers = ["cl", "gcc", "clang"]
    else:
        # Check for GCC or Clang on Unix-like systems
        compilers = ["gcc", "clang", "cc"]

    compiler_found = False
    for compiler in compilers:
        try:
            subprocess.run([compiler, "--version"],
                         capture_output=True, check=True)
            print(f"✅ C compiler ({compiler}) available")
            compiler_found = True
            break
        except (subprocess.CalledProcessError, FileNotFoundError):
            continue

    if not compiler_found:
        print("❌ No C compiler found!")
        print("Install build tools:")
        if system == "windows":
            print("  - Visual Studio Build Tools")
            print("  - Or MinGW-w64")
        elif system == "darwin":
            print("  - Xcode Command Line Tools: xcode-select --install")
        else:
            print("  - Ubuntu/Debian: sudo apt install build-essential")
            print("  - CentOS/RHEL: sudo yum groupinstall 'Development Tools'")
        return False

    # Check required files
    required_files = ["main.py", "Media/", "adahn.config.example"]
    for file_path in required_files:
        if not os.path.exists(file_path):
            print(f"❌ Required file/directory missing: {file_path}")
            return False

    print("✅ All dependencies satisfied")
    return True

def optimize_for_size():
    """Additional size optimization steps"""
    print("🗜️  Applying size optimizations...")

    # Remove unnecessary files that might be included
    exclude_patterns = [
        "__pycache__",
        "*.pyc",
        "*.pyo",
        ".git",
        ".venv",
        "tests/",
        "docs/",
        "*.md",
    ]

    return exclude_patterns

def build():
    """Main build function"""
    print("🚀 Starting Nuitka compilation for Automated Azan")
    print("=" * 50)

    # Check dependencies
    if not check_dependencies():
        print("❌ Build dependencies not satisfied")
        sys.exit(1)

    # Get build command
    cmd = get_nuitka_command()

    print("🔧 Build configuration:")
    system, machine = get_platform_info()
    print(f"   Platform: {system.title()} {machine}")
    print(f"   Output: {cmd[-1]}")
    print(f"   Mode: Standalone executable")

    # Show command for debugging
    print("\n🔨 Nuitka command:")
    print(" ".join(cmd))
    print()

    # Create build directory
    build_dir = Path("build")
    build_dir.mkdir(exist_ok=True)

    try:
        print("⏳ Compiling... (this may take 5-15 minutes)")
        print("💡 Tip: Add --verbose to see detailed progress")

        # Run Nuitka compilation
        result = subprocess.run(cmd, cwd=".", check=True)

        print("\n🎉 Build successful!")

        # Find and report the output file
        output_file = None
        for pattern in ["AutomatedAzan*", "main.dist/main*"]:
            import glob
            matches = glob.glob(pattern)
            if matches:
                output_file = matches[0]
                break

        if output_file:
            size_mb = os.path.getsize(output_file) / (1024 * 1024)
            print(f"📦 Executable: {output_file}")
            print(f"💾 Size: {size_mb:.1f} MB")

            # Test the executable
            print("\n🧪 Testing executable...")
            try:
                test_result = subprocess.run([output_file, "--help"],
                                           capture_output=True, text=True, timeout=10)
                if test_result.returncode == 0:
                    print("✅ Executable test passed")
                else:
                    print("⚠️  Executable test returned non-zero exit code")
            except subprocess.TimeoutExpired:
                print("⚠️  Executable test timed out (may be normal)")
            except Exception as e:
                print(f"⚠️  Could not test executable: {e}")

        print(f"\n🎯 Next steps:")
        print(f"   1. Test the executable: ./{output_file or 'AutomatedAzan'}")
        print(f"   2. Copy to target systems")
        print(f"   3. Distribute to users")

    except subprocess.CalledProcessError as e:
        print(f"\n❌ Build failed with exit code {e.returncode}")
        print("💡 Common solutions:")
        print("   - Ensure all dependencies are installed")
        print("   - Check C compiler is available")
        print("   - Try adding --verbose for more details")
        print("   - Check Nuitka documentation for platform-specific issues")
        sys.exit(1)

    except KeyboardInterrupt:
        print("\n⏹️  Build interrupted by user")
        sys.exit(1)

if __name__ == "__main__":
    build()