### Stage 1: Build Calculation Engine in Go
FROM golang:1.23-alpine AS go-builder
WORKDIR /app
COPY calculation_engine/go.mod calculation_engine/go.sum ./
RUN go mod download && go mod verify
COPY calculation_engine/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o /go_app_binary

### Stage 2: Build Metrics Generator (Rust) and Export Engine (C++)
FROM rust:1.87.0-slim-bookworm AS rust-cpp-builder
RUN apt-get update && \
    apt-get install -y --no-install-recommends g++ make curl ca-certificates pkg-config libssl-dev && \
    rm -rf /var/lib/apt/lists/*

# Rust (metrics_generator)
WORKDIR /app/metrics_generator
COPY server9/Cargo.toml server9/Cargo.lock* ./
RUN mkdir -p src && echo "fn main() {}" > src/main.rs
RUN cargo fetch
COPY server9/src ./src
RUN cargo build --release

# C++ (export_engine)
WORKDIR /app/export_engine_build
COPY server9/export.cpp .
RUN g++ -std=c++11 -O2 -Wall -o /app_export_engine export.cpp

### Stage 3: Final Runtime Image
FROM debian:bookworm-slim AS final
RUN apt-get update && \
    apt-get install -y --no-install-recommends supervisor ca-certificates libfuse2 libssl3 dbus curl && \
    curl -fsSL https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb -o /tmp/megacmd.deb && \
    apt-get install -y /tmp/megacmd.deb && \
    rm -f /tmp/megacmd.deb && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-ls && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-get && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-login && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-put && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-ipc && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-whoami && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-logout && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-mkdir && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Create non-root user and directories
RUN groupadd --gid 999 appgroup && useradd --uid 999 --gid appgroup --shell /bin/bash --create-home appuser && \
    mkdir -p /app /data/metrics_archive /var/log/supervisor /var/run && \
    chown -R appuser:appgroup /app /data /home/appuser /var/run /var/log/supervisor
WORKDIR /app

# Copy binaries
COPY --from=go-builder --chown=appuser:appgroup /go_app_binary /app/calculation_service
RUN chmod +x /app/calculation_service
COPY --from=rust-cpp-builder --chown=appuser:appgroup /app/metrics_generator/target/release/timestamp_generator /app/metrics_generator
RUN chmod +x /app/metrics_generator
COPY --from=rust-cpp-builder --chown=appuser:appgroup /app_export_engine /app/export_engine
RUN chmod +x /app/export_engine

# Copy session-keeper script and Supervisor config
# bump cache at 2025‑07‑25T04:00:00Z
COPY session-keeper.sh /usr/local/bin/session-keeper.sh
COPY supervisor.conf /etc/supervisor/supervisor.conf
RUN chmod +x /usr/local/bin/session-keeper.sh

# Expose port and start Supervisor
EXPOSE 9000
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/supervisor.conf", "-n"]
