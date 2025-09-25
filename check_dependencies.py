#!/usr/bin/env python3
"""
Dependency conflict checker for the wiz project.
This script checks for common dependency conflicts and provides solutions.
"""

import importlib
import sys
import subprocess
from packaging import version

def check_package(package_name, min_version=None, max_version=None):
    """Check if a package is installed and meets version requirements."""
    try:
        module = importlib.import_module(package_name)
        if hasattr(module, '__version__'):
            pkg_version = module.__version__
        else:
            # Try to get version from pip list
            try:
                result = subprocess.run(['pip', 'show', package_name], 
                                      capture_output=True, text=True)
                for line in result.stdout.split('\n'):
                    if line.startswith('Version:'):
                        pkg_version = line.split(':')[1].strip()
                        break
                else:
                    pkg_version = "unknown"
            except:
                pkg_version = "unknown"
        
        status = "✅"
        message = f"{package_name} {pkg_version}"
        
        if min_version and pkg_version != "unknown":
            try:
                if version.parse(pkg_version) < version.parse(min_version):
                    status = "⚠️"
                    message += f" (needs >={min_version})"
            except:
                pass
                
        if max_version and pkg_version != "unknown":
            try:
                if version.parse(pkg_version) >= version.parse(max_version):
                    status = "⚠️"
                    message += f" (needs <{max_version})"
            except:
                pass
        
        return status, message
        
    except ImportError:
        return "❌", f"{package_name} not installed"

def check_python_version():
    """Check Python version compatibility."""
    py_version = f"{sys.version_info.major}.{sys.version_info.minor}.{sys.version_info.micro}"
    if sys.version_info >= (3, 8):
        return "✅", f"Python {py_version}"
    else:
        return "❌", f"Python {py_version} (need 3.8+)"

def main():
    """Main dependency check."""
    print("🔍 Wiz Project Dependency Check")
    print("=" * 50)
    
    # Check Python version
    status, message = check_python_version()
    print(f"{status} {message}")
    
    # Core dependencies with version requirements
    dependencies = [
        ("numpy", "2.0.0", "2.3.0"),
        ("pandas", "2.2.0", None),
        ("matplotlib", "3.8.0", None),
        ("requests", "2.32.4", None),
        ("flask", "3.0.0", None),
        ("packaging", "24.2.0", None),
        ("tqdm", "4.67.0", None),
        ("typing_extensions", "4.14.0", None),
    ]
    
    print("\n📦 Core Dependencies:")
    for pkg, min_ver, max_ver in dependencies:
        status, message = check_package(pkg, min_ver, max_ver)
        print(f"{status} {message}")
    
    # Optional dependencies
    optional_deps = [
        ("discord", None, None),
        ("dotenv", None, None),
        ("json5", None, None),
        ("jedi", "0.16", None),
        ("scikit-learn", "1.6.0", None),
        ("protobuf", "5.29.1", "7.0.0"),
    ]
    
    print("\n🔧 Optional Dependencies:")
    for pkg, min_ver, max_ver in optional_deps:
        status, message = check_package(pkg, min_ver, max_ver)
        print(f"{status} {message}")
    
    print("\n💡 Installation Help:")
    print("- Run: python3 install.py")
    print("- Or: pip install -r requirements.txt")
    print("- See INSTALL.md for detailed instructions")
    
    # Check for common conflict patterns
    print("\n🚨 Known Conflict Checks:")
    
    # Check numpy version conflicts
    numpy_status, numpy_msg = check_package("numpy")
    if "❌" not in numpy_status:
        try:
            import numpy as np
            if hasattr(np, '__version__'):
                np_ver = np.__version__
                if version.parse(np_ver) < version.parse("2.0.0"):
                    print("⚠️  NumPy version may conflict with OpenCV/scikit-learn")
                elif version.parse(np_ver) >= version.parse("2.3.0"):
                    print("⚠️  NumPy version may conflict with some ML libraries")
                else:
                    print("✅ NumPy version compatible with most ML libraries")
        except:
            pass
    
    print("\nFor more details, see INSTALL.md")

if __name__ == "__main__":
    main()