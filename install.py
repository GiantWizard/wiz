#!/usr/bin/env python3
"""
Installation script for the wiz project.
This script attempts to install dependencies while resolving common conflicts.
"""

import subprocess
import sys
import os

def run_command(cmd, description):
    """Run a command and handle errors gracefully."""
    print(f"📦 {description}...")
    try:
        result = subprocess.run(cmd, shell=True, check=True, capture_output=True, text=True)
        print(f"✅ {description} completed successfully")
        return True
    except subprocess.CalledProcessError as e:
        print(f"❌ {description} failed:")
        print(f"Command: {cmd}")
        print(f"Error: {e.stderr}")
        return False

def check_python_version():
    """Check if Python version is compatible."""
    if sys.version_info < (3, 8):
        print("❌ Python 3.8+ is required")
        return False
    print(f"✅ Python {sys.version_info.major}.{sys.version_info.minor} is compatible")
    return True

def install_dependencies():
    """Install dependencies with conflict resolution."""
    
    if not check_python_version():
        return False
    
    print("🚀 Installing wiz project dependencies...")
    print("This script resolves common dependency conflicts found in Google Colab and other environments.\n")
    
    # Install core dependencies first
    core_packages = [
        "packaging>=24.2.0",
        "requests>=2.32.4", 
        "numpy>=2.0.0,<2.3.0",
        "typing-extensions>=4.14.0"
    ]
    
    for package in core_packages:
        if not run_command(f"pip install '{package}' --upgrade", f"Installing {package}"):
            print(f"⚠️  Failed to install {package}, continuing...")
    
    # Install main requirements
    if os.path.exists("requirements.txt"):
        run_command("pip install -r requirements.txt --upgrade", "Installing main requirements")
    else:
        print("⚠️  requirements.txt not found, installing minimal dependencies")
        minimal_deps = ["flask>=3.0.0", "gunicorn>=21.0.0", "tqdm>=4.67.0"]
        for dep in minimal_deps:
            run_command(f"pip install '{dep}' --upgrade", f"Installing {dep}")
    
    print("\n✅ Installation completed!")
    print("\nOptional components:")
    print("- For analysis scripts: pip install -r requirements-analysis.txt")
    print("- For Discord bot: pip install -r requirements-bot.txt")
    print("- For server10 only: pip install -r server10/requirements.txt")

if __name__ == "__main__":
    install_dependencies()