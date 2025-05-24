FROM gcc:latest

# Install dependencies needed for repository setup.
RUN apt-get update && apt-get install -y \
    curl \
    apt-transport-https \
    ca-certificates \
    gnupg \
    build-essential

# Install MEGAcmd using the official MEGA repository.
RUN mkdir -p /etc/apt/keyrings && \
    curl -fsSL https://mega.nz/keys/MEGA_signing.key | gpg --dearmor -o /etc/apt/keyrings/mega.gpg && \
    echo "deb [signed-by=/etc/apt/keyrings/mega.gpg] https://mega.nz/linux/repo/Debian_12/ ./" | tee /etc/apt/sources.list.d/mega.list && \
    apt-get update && \
    apt-cache search mega && \
    apt-get install -y megacmd || \
    (apt-get install -y git build-essential libpcre3-dev libc-ares-dev libcrypto++-dev libssl-dev zlib1g-dev libsqlite3-dev libsodium-dev libtool autoconf && \
    git clone https://github.com/meganz/MEGAcmd.git && \
    cd MEGAcmd && \
    sh autogen.sh && \
    ./configure && \
    make && \
    make install)

# Install Rust toolchain.
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"

# Set the working directory.
WORKDIR /app

# Build the Rust binary.
COPY Cargo.toml .
COPY src/ src/
RUN cargo build --release

# Build the C++ export engine.
COPY export.cpp .
RUN g++ -std=c++11 -O2 -o export_engine export.cpp

# Copy railway configuration (if needed).
COPY koyeb.toml .

# Run the Rust binary which loops and calls the export engine.
CMD ["./target/release/timestamp_generator"]
