# ─── Builder Stage (for Rust and C++) ───────────────────────────────────
FROM gcc:latest AS builder
LABEL stage=builder

# 1) Install system deps needed for building Rust and C++ components
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    build-essential \
    git # If you fetch any code/deps via git during build

# 2) Install Rust toolchain
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
      | sh -s -- -y
ENV PATH=/root/.cargo/bin:$PATH

# 3) Build Rust binary (timestamp_generator)
WORKDIR /app/rust_app
COPY ./server9/Cargo.toml .
# If Cargo.lock exists in ./server9/, copy it too for reproducible builds
# COPY ./server9/Cargo.lock .
COPY ./server9/src/ ./src/   # Assuming src is in ./server9/src/
RUN cargo build --release

# 4) Build the C++ export engine
WORKDIR /app/cpp_app
COPY ./server9/export.cpp . # Assuming export.cpp is in ./server9/
RUN g++ -std=c++11 -O2 -o export_engine export.cpp


# ─── Runtime Stage (minimal) ─────────────────────────────────────────
FROM debian:bookworm-slim AS final

# Install runtime dependencies and MEGAcmd CLI in a single RUN layer
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
    # Add any other *runtime* shared libraries your compiled binaries need
    # Download and install MEGAcmd
    && curl -fsSL "https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb" -o /tmp/megacmd.deb \
    && apt-get install -y --no-install-recommends /tmp/megacmd.deb \
    # Clean up APT and downloaded .deb file
    && rm /tmp/megacmd.deb \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Create a non-root user
RUN addgroup --system appgroup && adduser --system --ingroup appgroup appuser

WORKDIR /app

# Copy compiled Rust binary from builder stage
COPY --from=builder /app/rust_app/target/release/timestamp_generator .
# Copy compiled C++ binary from builder stage
COPY --from=builder /app/cpp_app/export_engine .

# COPY ./server9/railway.toml . # If needed, adjust path relative to context. Probably remove.

USER appuser

# Assuming timestamp_generator is your main app and now includes an HTTP health check on port 9000
CMD ["./timestamp_generator"]