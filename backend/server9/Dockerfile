# server9/Dockerfile

# Use Debian with build tools for Rust & C++
FROM gcc:latest

# 1) Install system deps
RUN apt-get update && apt-get install -y \
    curl \
    apt-transport-https \
    ca-certificates \
    gnupg \
    build-essential \
    libpcre3-dev \
    libc-ares-dev \
    libcrypto++-dev \
    libssl-dev \
    zlib1g-dev \
    libsqlite3-dev \
    libsodium-dev \
    autoconf \
    libtool \
    git

# 2) (Optional) Install MEGAcmd via upstream repo
RUN mkdir -p /etc/apt/keyrings \
 && curl -fsSL https://mega.nz/keys/MEGA_signing.key \
      | gpg --dearmor -o /etc/apt/keyrings/mega.gpg \
 && echo "deb [signed-by=/etc/apt/keyrings/mega.gpg] \
      https://mega.nz/linux/repo/Debian_12/ ./" \
      > /etc/apt/sources.list.d/mega.list \
 && apt-get update \
 && apt-get install -y megacmd || echo "MEGAcmd install fallback..."

# 3) Install Rust toolchain
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
      | sh -s -- -y
ENV PATH=/root/.cargo/bin:$PATH

# 4) Set workdir & copy in code
WORKDIR /app

COPY Cargo.toml .
COPY src/ src/

# Build Rust binary
RUN cargo build --release

# 5) Build the C++ export engine
COPY export.cpp .
RUN g++ -std=c++11 -O2 -o export_engine export.cpp

# 6) (Optional) Copy any config
COPY railway.toml .

# 7) Default command
CMD ["./target/release/timestamp_generator"]
