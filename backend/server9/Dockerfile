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

# 1) Copy the built binaries
COPY --from=builder /usr/bin/mega-*                                 /usr/bin/
COPY --from=builder /app/target/release/timestamp_generator        /usr/local/bin/timestamp_generator
COPY --from=builder /app/export_engine                             /usr/local/bin/export_engine

# 2) Install TLS certs, Python, and MEGAcmd runtime dependencies
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
      python3 \
      fuse \
      libfuse2 \
      ffmpeg \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
EXPOSE 9000

# 3) Start MEGAcmd server, your worker, and the HTTP health server
CMD ["sh", "-c", "\
    mega-cmd-server & \
    timestamp_generator & \
    exec python3 -m http.server 9000 \
"]

