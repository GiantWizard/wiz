# Stage 1: build environment
FROM gcc:latest AS builder

# 1) Install only runtime deps + curl for fetching MEGAcmd, then clean up.
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
 && curl -fsSL https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb \
      -o /tmp/megacmd.deb \
 # Install the MEGAcmd .deb and fix any missing dependencies
 && dpkg -i /tmp/megacmd.deb || apt-get -f install -y \
 && rm /tmp/megacmd.deb \
 # Remove curl and clean apt cache
 && apt-get purge -y curl \
 && apt-get autoremove -y \
 && rm -rf /var/lib/apt/lists/*

# 2) Install Rust toolchain
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"

# 3) Prepare build directory and compile Rust app
WORKDIR /app
COPY Cargo.toml .
COPY src/ src/
RUN cargo build --release

# 4) Compile the C++ export engine
COPY export.cpp .
RUN g++ -std=c++11 -O2 -o export_engine export.cpp

# Include your Railway (or Koyeb) config if needed
COPY railway.toml .

# Stage 2: runtime image
FROM gcc:latest

# Copy over only what we need at runtime
COPY --from=builder /usr/bin/mega-* /usr/bin/
COPY --from=builder /root/.cargo/bin/cargo /usr/local/bin/ 
COPY --from=builder /root/.cargo /root/.cargo 
COPY --from=builder /app/target/release/timestamp_generator /usr/local/bin/timestamp_generator
COPY --from=builder /app/export_engine /usr/local/bin/export_engine

# Install only ca-certificates for TLS
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Set the default command
CMD
