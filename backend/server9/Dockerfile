# ─── Builder Stage ────────────────────────────────────────────────────────
FROM rust:1.78-slim-bookworm AS builder

# 1) Install only what we need to compile Rust & C++
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      build-essential \
      ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# 2) Compile the Rust app in release mode
WORKDIR /app/rust_app
# Copy only the manifest; let Cargo generate a lockfile if missing
COPY Cargo.toml ./
COPY src/ ./src/
RUN cargo build --release

# 3) Compile and statically link the C++ export_engine
WORKDIR /app/cpp_app
COPY export.cpp .
RUN g++ -std=c++11 -O2 -static -s -o export_engine export.cpp

# ─── Final Runtime Stage ─────────────────────────────────────────────────
FROM debian:bookworm-slim

# 1) Install only runtime deps + acquire Mega CMD, then remove curl immediately
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
      dpkg \
    && curl -fsSL "https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb" -o /tmp/megacmd.deb \
    && dpkg -i /tmp/megacmd.deb || apt-get -f install -y \
    && rm /tmp/megacmd.deb \
    && apt-get purge -y curl dpkg \
    && apt-get autoremove -y \
    && rm -rf /var/lib/apt/lists/*

# 2) Create a non-root user
RUN addgroup --system appgroup && adduser --system --ingroup appgroup appuser

WORKDIR /app
USER appuser

# 3) Copy in the two static, stripped binaries
COPY --from=builder /app/rust_app/target/release/timestamp_generator ./
COPY --from=builder /app/cpp_app/export_engine       ./

# 4) Default command
CMD ["./timestamp_generator"]
