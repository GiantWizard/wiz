# ─── Builder Stage (for Rust and C++) ───────────────────────────────────
# Use an official Rust base image. Choose a specific version for stability.
# e.g., rust:1.78-slim-bookworm (check Docker Hub for latest -slim tags)
FROM rust:1.87-slim-bookworm AS builder
LABEL stage=builder

# 1) Install system deps needed for C++ compilation and other tools
#    build-essential includes g++. git is only if cargo needs it for deps.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    curl \
    ca-certificates \
    # git # Uncomment if your cargo dependencies require git to be fetched
 && rm -rf /var/lib/apt/lists/*

# Rust and Cargo are already available from the base image.
# ENV PATH for cargo is typically already set by the rust base image.

# 3) Build Rust binary (timestamp_generator)
WORKDIR /app/rust_app
COPY Cargo.toml .
# If you use a Cargo.lock file, include it for reproducible builds
# COPY Cargo.lock .
COPY src/ ./src/
RUN cargo build --release

# 4) Build the C++ export engine
WORKDIR /app/cpp_app
COPY export.cpp .
RUN g++ -std=c++11 -O2 -o export_engine export.cpp


# ─── Runtime Stage (minimal) ─────────────────────────────────────────
FROM debian:bookworm-slim AS final

# Install runtime dependencies and MEGAcmd CLI
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
    # libssl might be needed by Rust reqwest if it uses native-tls and isn't static
    # Add other runtime libs if ldd shows they are needed by your binaries
    # && apt-get install -y --no-install-recommends libssl3 zlib1g # Example
    && curl -fsSL "https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb" -o /tmp/megacmd.deb \
    && apt-get install -y --no-install-recommends /tmp/megacmd.deb \
    && rm /tmp/megacmd.deb \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN addgroup --system appgroup && adduser --system --ingroup appgroup appuser

WORKDIR /app

# Copy compiled artifacts from the 'builder' stage
COPY --from=builder /app/rust_app/target/release/timestamp_generator .
COPY --from=builder /app/cpp_app/export_engine .

USER appuser

# This assumes your timestamp_generator Rust app includes the Axum HTTP health server
CMD ["./timestamp_generator"]