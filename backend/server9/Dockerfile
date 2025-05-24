# ─── Runtime Stage (minimal) ─────────────────────────────────────────
FROM debian:bookworm-slim AS final

# Install runtime dependencies and MEGAcmd CLI in a single RUN layer
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
    # Add any other *runtime* shared libraries your compiled binaries need (e.g., libssl3, zlib1g)
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

# Copy compiled Rust binary from builder
COPY --from=builder --chown=appuser:appgroup /app/rust_app/target/release/timestamp_generator .
# Copy compiled C++ binary from builder
COPY --from=builder --chown=appuser:appgroup /app/cpp_app/export_engine .

# COPY --chown=appuser:appgroup railway.toml . # Consider removing if not used by app

USER appuser

CMD ["./timestamp_generator"]