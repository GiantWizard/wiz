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

# 3) Fetch & install MEGAcmd .deb, fix deps
RUN curl -fsSL https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb \
      -o /tmp/megacmd.deb \
 && dpkg -i /tmp/megacmd.deb || (apt-get update && apt-get -f install -y) \
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

# 6) Purge curl (done with builder steps)
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get purge -y curl \
 && apt-get autoremove -y \
 && rm -rf /var/lib/apt/lists/*



# ─── STAGE 2: RUNTIME ──────────────────────────────────────────
FROM gcc:latest

# Copy MEGAcmd binaries
COPY --from=builder /usr/bin/mega-*                   /usr/bin/

# Copy our compiled binaries into the path
COPY --from=builder /app/target/release/timestamp_generator /usr/local/bin/timestamp_generator
COPY --from=builder /app/export_engine                    /usr/local/bin/export_engine

# Install TLS certs + python3 for HTTP health server
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
      python3 \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
EXPOSE 9000

# Run the background worker & HTTP server for health checks
CMD ["sh", "-c", "\
    # 1) Start the MEGAcmd server so mega-login/put will work \
    mega-cmd-server & \
    # 2) Run your worker in the background \
    timestamp_generator & \
    # 3) Serve HTTP on 9000 for health checks \
    exec python3 -m http.server 9000 \
"]
