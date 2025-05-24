# ─── STAGE 1: BUILD ───────────────────────────────────────────

FROM gcc:latest AS builder

# 1) Install ca-certificates + curl, fetch & install MEGAcmd, then clean up curl & cache
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
 && curl -fsSL https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb \
      -o /tmp/megacmd.deb \
 && dpkg -i /tmp/megacmd.deb || apt-get -f install -y \
 && rm /tmp/megacmd.deb \
 && apt-get purge -y curl \
 && apt-get autoremove -y \
 && rm -rf /var/lib/apt/lists/*

# 2) Install Rust toolchain (for building your Rust app)
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"

# 3) Compile the Rust timestamp_generator
WORKDIR /app
COPY Cargo.toml .
COPY src/ src/
RUN cargo build --release

# 4) Compile the C++ export engine
COPY export.cpp .
RUN g++ -std=c++11 -O2 -o export_engine export.cpp

# Copy any config (railway.toml, etc.)
COPY koyeb.toml .



# ─── STAGE 2: RUNTIME ─────────────────────────────────────────

FROM gcc:latest

# Copy only the runtime bits from builder
# (note: MEGAcmd installs binaries into /usr/bin/mega-*)
COPY --from=builder /usr/bin/mega-* /usr/bin/
COPY --from=builder /app/target/release/timestamp_generator /usr/local/bin/timestamp_generator
COPY --from=builder /app/export_engine                 /usr/local/bin/export_engine

# Install only ca-certificates for HTTPS
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

CMD ["timestamp_generator"]
