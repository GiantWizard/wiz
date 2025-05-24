# ─── Builder Stage (Rust & C++) ───────────────────────────────────
# Use a specific Rust version for reproducibility.
# The -slim-bookworm variant is smaller than the full one.
FROM rust:1.78-slim-bookworm AS builder
# You can update 1.78 to a newer stable version if desired.
# Check Docker Hub for available tags: https://hub.docker.com/_/rust
LABEL stage=builder

# 1) Install system deps for C++ compilation and other tools
#    build-essential includes g++. curl is for fetching. ca-certificates for HTTPS.
#    Removed git as it's likely not needed for cargo build with crates.io deps.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    curl \
    ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# Rust and Cargo are already available from the base image.
# The PATH for cargo is typically set by the official rust base images.

# 2) Build Rust binary (timestamp_generator)
WORKDIR /app/rust_app
COPY Cargo.toml .
# If you use Cargo.lock (recommended for reproducible builds), copy it too:
# COPY Cargo.lock .
COPY src/ ./src/
# Using --locked requires Cargo.lock to be present and up-to-date.
# This builds a release version, optimized and smaller.
RUN cargo build --release # --locked

# 3) Build the C++ export engine
WORKDIR /app/cpp_app
COPY export.cpp .
# -O2 for optimization.
# Link libstdc++ statically if possible, to reduce runtime deps, though it can make binary larger.
# On slim images, libstdc++ is usually present.
RUN g++ -std=c++11 -O2 -o export_engine export.cpp


# ─── Runtime Stage (minimal) ─────────────────────────────────────────
FROM debian:bookworm-slim AS final

# Install only essential runtime dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \ 
        curl \ 
    # libssl3 is likely NOT needed if reqwest uses rustls-tls and C++ links libstdc++ dynamically (usually present)
    # If you find you need it: libssl3
    # Download and install MEGAcmd
    && curl -fsSL "https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb" -o /tmp/megacmd.deb \
    && apt-get install -y --no-install-recommends /tmp/megacmd.deb \
    # Clean up
    && rm /tmp/megacmd.deb \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Create a non-root user for security
RUN addgroup --system appgroup && adduser --system --ingroup appgroup appuser

WORKDIR /app

# Copy compiled artifacts from the 'builder' stage
COPY --from=builder /app/rust_app/target/release/timestamp_generator .
COPY --from=builder /app/cpp_app/export_engine .

USER appuser

# CMD for the Rust application (which includes the health check server)
CMD ["./timestamp_generator"]