### server9/Dockerfile

# ─── Builder Stage (for Rust and C++) ───────────────────────────────────
FROM gcc:latest AS builder
LABEL stage=builder

# 1) Install system deps needed for building Rust and C++ components
#    Only install what's needed for *compilation*.
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    build-essential \
    # libpcre3-dev \ # Only if your Rust/C++ code specifically links against it at compile time
    # libc-ares-dev \ # Only if specifically needed for compilation
    # libcrypto++-dev \ # Only if specifically needed for compilation
    # libssl-dev \ # Rust often links against this, keep if needed
    # zlib1g-dev \ # Keep if needed
    # libsqlite3-dev \ # Only if specifically needed for compilation
    # libsodium-dev \ # Only if specifically needed for compilation
    # autoconf \ # Usually for C projects with ./configure, maybe not for yours
    # libtool \ # Usually for C projects with ./configure
    git # If you fetch any code/deps via git during build

# If MEGAcmd was needed for *build steps only* (e.g., to download build-time deps)
# it would go here. But based on your previous Go app, it seems like a runtime dep.
# For now, assuming it's NOT needed for server9 build.

# 2) Install Rust toolchain
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
      | sh -s -- -y
ENV PATH=/root/.cargo/bin:$PATH

# 3) Build Rust binary
WORKDIR /app/rust_app
COPY Cargo.toml .
# If Cargo.lock exists, copy it too for reproducible builds
# COPY Cargo.lock . 
COPY src/ ./src/
# Build release binary. Target dir will be /app/rust_app/target/release/
RUN cargo build --release

# 4) Build the C++ export engine
WORKDIR /app/cpp_app
COPY export.cpp .
RUN g++ -std=c++11 -O2 -o export_engine export.cpp


# ─── Runtime Stage (minimal) ─────────────────────────────────────────
FROM debian:bookworm-slim AS final
# Or consider alpine if your binaries are fully static and compatible

# Install only ABSOLUTELY NECESSARY runtime dependencies.
# If your Rust/C++ binaries are dynamically linked to something not in bookworm-slim,
# you'll need to identify and install them here.
# For example, if they link against libssl or zlib:
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    # libssl3 \ # If Rust binary needs it at runtime
    # zlib1g \  # If Rust binary needs it at runtime
    # Add any other *runtime* shared libraries your compiled binaries need
 && rm -rf /var/lib/apt/lists/*

# Create a non-root user
RUN addgroup --system appgroup && adduser --system --ingroup appgroup appuser

WORKDIR /app

# Copy compiled Rust binary from builder
COPY --from=builder --chown=appuser:appgroup /app/rust_app/target/release/timestamp_generator .
# Copy compiled C++ binary from builder
COPY --from=builder --chown=appuser:appgroup /app/cpp_app/export_engine .

# Copy runtime configuration files
COPY --chown=appuser:appgroup railway.toml .

USER appuser

# Define the default command to run your main server9 application
# This needs to be the actual binary that starts your server9 service.
# If timestamp_generator is the main one:
CMD ["./timestamp_generator"]
# If export_engine is the main one, or if they are run differently, adjust CMD.
# If server9 is a web server, ensure EXPOSE <port> is also here.