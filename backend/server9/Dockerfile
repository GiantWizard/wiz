# ─── Builder Stage (for Rust and C++) ───────────────────────────────────
FROM gcc:latest AS builder
LABEL stage=builder

# 1) Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    build-essential \
    git

# 2) Install Rust toolchain
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
      | sh -s -- -y
ENV PATH=/root/.cargo/bin:$PATH

# 3) Build Rust binary (timestamp_generator)
WORKDIR /app/rust_app

# CORRECTED COPY commands - paths are relative to the './server9' context
COPY Cargo.toml .
# If you use a Cargo.lock file, uncomment the next line
# COPY Cargo.lock .
COPY src/ ./src/

RUN cargo build --release

# 4) Build the C++ export engine
WORKDIR /app/cpp_app

# CORRECTED COPY command
COPY export.cpp .

RUN g++ -std=c++11 -O2 -o export_engine export.cpp


# ─── Runtime Stage (minimal) ─────────────────────────────────────────
FROM debian:bookworm-slim AS final

# Install runtime dependencies and MEGAcmd CLI
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
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

CMD ["./timestamp_generator"]