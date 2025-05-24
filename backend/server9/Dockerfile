# ─── STAGE 1: BUILD ────────────────────────────────────────────
FROM gcc:latest AS builder

# 1) Install ca-certificates + curl
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
 && rm -rf /var/lib/apt/lists/*

# 2) Install Rust toolchain
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"

# 3) Fetch & install MEGAcmd .deb (dpkg is in the base), fix deps
RUN curl -fsSL https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb \
      -o /tmp/megacmd.deb \
 && dpkg -i /tmp/megacmd.deb || apt-get update && apt-get -f install -y \
 && rm /tmp/megacmd.deb \
 && rm -rf /var/lib/apt/lists/*

# 4) Build the Rust timestamp_generator
WORKDIR /app
COPY Cargo.toml .
COPY src/ src/
RUN cargo build --release

# 5) Build the C++ export engine
COPY export.cpp .
RUN g++ -std=c++11 -O2 -o export_engine export.cpp

# 6) Now that both rustup & megacmd are done, purge curl to slim down
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get purge -y curl \
 && apt-get autoremove -y \
 && rm -rf /var/lib/apt/lists/*

# 7) Copy any platform config
COPY railway.toml .



# ─── STAGE 2: RUNTIME ───────────────────────────────────────────
FROM gcc:latest

# Copy only what we need at runtime:
COPY --from=builder /usr/bin/mega-*                                /usr/bin/
COPY --from=builder /app/target/release/timestamp_generator       /usr/local/bin/timestamp_generator
COPY --from=builder /app/export_engine                            /usr/local/bin/export_engine

# Install TLS certs + netcat for dummy listener
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
      netcat-openbsd \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Expose port 9000 for Koyeb health checks
EXPOSE 9000

# Start the generator in the background, then listen on 9000 forever
CMD ["sh", "-c", "\
    timestamp_generator & \
    exec nc -l -k -p 9000 \
"]
