# ─── STAGE 1: BUILD ────────────────────────────────────────────
FROM gcc:latest AS builder

# 1) Install ca-certificates + curl (we’ll purge curl only at the very end)
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
 && rm -rf /var/lib/apt/lists/*

# 2) Install Rust toolchain (needs curl)
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"

# 3) Fetch & install MEGAcmd .deb (dpkg already in base), fix deps
RUN curl -fsSL https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb \
      -o /tmp/megacmd.deb \
 && dpkg -i /tmp/megacmd.deb || apt-get update && apt-get -f install -y \
 && rm /tmp/megacmd.deb \
 # MEGAcmd pulled in libfuse2 & fuse; that's fine
 && rm -rf /var/lib/apt/lists/*

# 4) Build the Rust app
WORKDIR /app
COPY Cargo.toml .
COPY src/ src/
RUN cargo build --release

# 5) Build the C++ export engine
COPY export.cpp .
RUN g++ -std=c++11 -O2 -o export_engine export.cpp

# 6) Now that we’ve done both rustup + megacmd, purge curl to slim down
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get purge -y curl \
 && apt-get autoremove -y \
 && rm -rf /var/lib/apt/lists/*

# Copy any platform config
COPY koyeb.toml .



# ─── STAGE 2: RUNTIME ───────────────────────────────────────────
FROM gcc:latest

# Pull in only what we need:
#  - MEGAcmd binaries under /usr/bin/mega-*
#  - our Rust binary + C++ export_engine
COPY --from=builder /usr/bin/mega-*               /usr/bin/
COPY --from=builder /app/target/release/timestamp_generator /usr/local/bin/timestamp_generator
COPY --from=builder /app/export_engine            /usr/local/bin/export_engine

# We still need TLS certs for HTTPS
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

CMD ["timestamp_generator"]
