FROM gcc:latest

# Install dependencies needed for repository setup
RUN apt-get update && apt-get install -y \
    curl \
    apt-transport-https \
    ca-certificates \
    gnupg

# Install MEGAcmd using the official MEGA repository
RUN mkdir -p /etc/apt/keyrings && \
    curl -fsSL https://mega.nz/keys/MEGA_signing.key | gpg --dearmor -o /etc/apt/keyrings/mega.gpg && \
    echo "deb [signed-by=/etc/apt/keyrings/mega.gpg] https://mega.nz/linux/repo/Debian_12/ ./" | \
    tee /etc/apt/sources.list.d/mega.list && \
    apt-get update && \
    # List available packages to see what's actually available
    apt-cache search mega && \
    # Try installing by pattern matching
    apt-get install -y megacmd || \
    # Alternative: Try installing old-style name
    apt-get install -y MEGAcmd || \
    # Alternative: Build from source if needed
    (apt-get install -y git build-essential libpcre3-dev libc-ares-dev libcrypto++-dev libssl-dev zlib1g-dev libsqlite3-dev libsodium-dev libtool autoconf && \
    git clone https://github.com/meganz/MEGAcmd.git && \
    cd MEGAcmd && \
    sh autogen.sh && \
    ./configure && \
    make && \
    make install)

# Set the working directory
WORKDIR /app

# Copy all files into the working directory 
COPY . .

# Compile the C++ application
RUN g++ -std=c++11 -O2 -o timestamp_app export.cpp

# Run the application when the container starts
CMD ["./timestamp_app"]