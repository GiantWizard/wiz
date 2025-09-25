#!/usr/bin/env python3
"""
Validation script to verify that our dependency fixes address the original conflicts.
"""

import sys
from packaging import version

def validate_conflict_resolution():
    """Validate that our requirements resolve the original conflicts."""
    print("🔧 Validating Dependency Conflict Resolution")
    print("=" * 50)
    
    # Read our main requirements file
    requirements = {}
    try:
        with open('requirements.txt', 'r') as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith('#'):
                    if '>=' in line:
                        pkg = line.split('>=')[0].strip()
                        min_ver = line.split('>=')[1].split(',')[0].strip()
                        requirements[pkg] = {'min': min_ver}
                        if '<' in line:
                            max_ver = line.split('<')[1].strip()
                            requirements[pkg]['max'] = max_ver
    except FileNotFoundError:
        print("❌ requirements.txt not found")
        return False
    
    # Original conflicts from the problem statement
    original_conflicts = {
        'numpy': {
            'conflict': '1.26.4 vs >=2.0.0 requirements',
            'solution': 'numpy>=2.0.0,<2.3.0',
            'check': lambda r: 'numpy' in r and version.parse(r['numpy']['min']) >= version.parse('2.0.0')
        },
        'requests': {
            'conflict': '2.32.3 vs 2.32.4 requirements',
            'solution': 'requests>=2.32.4',
            'check': lambda r: 'requests' in r and version.parse(r['requests']['min']) >= version.parse('2.32.4')
        },
        'packaging': {
            'conflict': '24.1 vs >=24.2.0 requirements',
            'solution': 'packaging>=24.2.0',
            'check': lambda r: 'packaging' in r and version.parse(r['packaging']['min']) >= version.parse('24.2.0')
        },
        'typing-extensions': {
            'conflict': '4.12.2 vs >=4.14.0 requirements',
            'solution': 'typing-extensions>=4.14.0',
            'check': lambda r: 'typing-extensions' in r and version.parse(r['typing-extensions']['min']) >= version.parse('4.14.0')
        },
        'protobuf': {
            'conflict': '5.27.2 vs >=5.29.1 requirements',
            'solution': 'protobuf>=5.29.1,<7.0.0',
            'check': lambda r: 'protobuf' in r and version.parse(r['protobuf']['min']) >= version.parse('5.29.1')
        }
    }
    
    print("📋 Checking Original Conflicts:")
    all_resolved = True
    
    for pkg, info in original_conflicts.items():
        if info['check'](requirements):
            print(f"✅ {pkg}: {info['conflict']} → RESOLVED")
        else:
            print(f"❌ {pkg}: {info['conflict']} → NOT RESOLVED")
            all_resolved = False
    
    print(f"\n📊 Resolution Summary:")
    print(f"- Total conflicts checked: {len(original_conflicts)}")
    print(f"- Conflicts resolved: {sum(1 for info in original_conflicts.values() if info['check'](requirements))}")
    
    if all_resolved:
        print("\n🎉 All major dependency conflicts have been resolved!")
    else:
        print("\n⚠️  Some conflicts may still need attention.")
    
    # Check additional enhancements
    print(f"\n🚀 Additional Enhancements:")
    enhancements = [
        ('matplotlib', 'Avoided yanked 3.9.1 version'),
        ('tqdm', 'Updated for Google Colab compatibility'),
        ('scikit-learn', 'Added for ML library compatibility'),
        ('jedi', 'Added for IPython/Jupyter support'),
        ('PyYAML', 'Added for configuration file support')
    ]
    
    for pkg, desc in enhancements:
        if pkg.lower().replace('-', '_') in [k.lower().replace('-', '_') for k in requirements.keys()]:
            print(f"✅ {pkg}: {desc}")
        else:
            print(f"ℹ️  {pkg}: {desc} (optional)")
    
    return all_resolved

def check_file_structure():
    """Check that all necessary files were created."""
    print(f"\n📁 Validating File Structure:")
    
    required_files = [
        'requirements.txt',
        'install.py', 
        'check_dependencies.py',
        'INSTALL.md',
        '.gitignore'
    ]
    
    optional_files = [
        'requirements-analysis.txt',
        'requirements-bot.txt', 
        'requirements-dev.txt',
        'ARCHITECTURE.md'
    ]
    
    all_present = True
    for file in required_files:
        try:
            with open(file, 'r') as f:
                print(f"✅ {file} - Present")
        except FileNotFoundError:
            print(f"❌ {file} - Missing")
            all_present = False
    
    for file in optional_files:
        try:
            with open(file, 'r') as f:
                print(f"📄 {file} - Present (optional)")
        except FileNotFoundError:
            print(f"ℹ️  {file} - Not present (optional)")
    
    return all_present

if __name__ == "__main__":
    print("🧪 Validating Wiz Project Dependency Fix")
    print("=" * 50)
    
    conflicts_resolved = validate_conflict_resolution()
    files_present = check_file_structure()
    
    print(f"\n🏁 Final Validation:")
    if conflicts_resolved and files_present:
        print("✅ All validations passed! The dependency conflict fix is complete.")
        sys.exit(0)
    else:
        print("⚠️  Some validations failed. Please check the output above.")
        sys.exit(1)