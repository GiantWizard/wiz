# Installation Guide for Wiz Project

This guide helps you install the wiz project dependencies while avoiding common version conflicts.

## Quick Start

### Option 1: Automated Installation (Recommended)
```bash
python3 install.py
```

### Option 2: Manual Installation
```bash
# Install core dependencies first to resolve conflicts
pip install packaging>=24.2.0 requests>=2.32.4 numpy>=2.0.0,<2.3.0 typing-extensions>=4.14.0

# Install main requirements
pip install -r requirements.txt
```

## Component-Specific Installation

### Server Components
```bash
# For server10 (minimal web server)
pip install -r server10/requirements.txt

# For main backend (includes analysis tools)
pip install -r requirements.txt -r requirements-analysis.txt
```

### Bot Components
```bash
# For Discord bot functionality
pip install -r requirements.txt -r requirements-bot.txt
```

## Dependency Conflict Resolution

This project includes version-pinned requirements that resolve common conflicts found in:
- Google Colab environments
- Jupyter notebooks
- Cloud deployment platforms

### Key Resolved Conflicts:
- **numpy**: Compatible versions for OpenCV, scikit-learn, and PyTorch
- **requests**: Version 2.32.4+ for security and compatibility
- **packaging**: Version 24.2.0+ for modern Python packaging
- **protobuf**: Version 5.29.1+ for gRPC and ML frameworks
- **matplotlib**: Avoiding yanked version 3.9.1

### Common Issues and Solutions

#### Google Colab Conflicts
If you see conflicts with existing packages in Colab:
```python
# In Colab, restart runtime after installation
import os
os.kill(os.getpid(), 9)  # Restart runtime
```

#### Local Development
For local development with conda:
```bash
conda create -n wiz python=3.11
conda activate wiz
pip install -r requirements.txt
```

#### Docker Development
```bash
# Use the provided Dockerfile in backend/
docker build -t wiz-backend backend/
```

## Verification

Test your installation:
```python
import numpy, pandas, matplotlib, requests, flask
print("✅ All core dependencies installed successfully")
```

## Troubleshooting

### Version Conflicts
If you encounter version conflicts:
1. Try installing core dependencies first (see automated script)
2. Use `--force-reinstall` flag for specific packages
3. Consider using a fresh virtual environment

### Network Issues
If PyPI downloads fail:
1. Try using a different index: `pip install -i https://pypi.org/simple/`
2. Use pip with retries: `pip install --retries 5`

### Environment-Specific Issues
- **Colab**: Use `!pip install` instead of `pip install`
- **Kaggle**: Some packages may be pre-installed
- **Local**: Ensure you're using a virtual environment

## Requirements Files Overview

- `requirements.txt` - Main dependencies with conflict resolution
- `requirements-analysis.txt` - Additional packages for analysis scripts
- `requirements-bot.txt` - Discord bot dependencies
- `server10/requirements.txt` - Minimal server requirements