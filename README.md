# Wiz Project

A comprehensive market analysis and automation platform with multi-language components.

## Quick Start

### Installation

```bash
# Automated installation (recommended)
python3 install.py

# Or install manually
pip install -r requirements.txt
```

### Check Dependencies

```bash
python3 check_dependencies.py
```

## Project Structure

- **backend/** - Rust/C++ metrics generation and Go calculation engine
- **server10/** - Python web server for market analysis
- **zzz/** - Analysis scripts and data processing tools
- **bot-test/** - Discord bot components
- **fusion_dashboard/** - Web dashboard components

## Components

### Server Components
- **server10/fetchur.py** - Market stability analysis server
- **backend/server9** - Rust-based metrics generator with pattern detection
- **calculation_engine** - Go-based dual expansion calculation engine

### Analysis Tools
- **zzz/fun2.py** - High-velocity market analysis
- **zzz/fun3.py** - Trend deviation analysis
- **zzz/shard2.py** - Market price processing

## Installation Options

### Core Dependencies
```bash
pip install -r requirements.txt
```

### Component-Specific
```bash
# Analysis scripts
pip install -r requirements-analysis.txt

# Discord bot
pip install -r requirements-bot.txt

# Development tools
pip install -r requirements-dev.txt
```

## Dependencies

This project resolves common Python dependency conflicts found in:
- Google Colab environments
- Jupyter notebooks  
- Cloud deployment platforms

### Key Libraries
- **numpy** (≥2.0.0, <2.3.0) - Compatible with ML libraries
- **pandas** (≥2.2.0) - Data processing
- **matplotlib** (≥3.8.0) - Visualization
- **requests** (≥2.32.4) - HTTP client
- **flask** (≥3.0.0) - Web framework

## Architecture Flow

For the detailed system architecture flowchart, see [ARCHITECTURE.md](ARCHITECTURE.md).

## Troubleshooting

For dependency conflicts or installation issues, see [INSTALL.md](INSTALL.md).