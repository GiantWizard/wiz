# STAGE 1: Go Builder for Calculation Engine
FROM golang:1.23-alpine AS go-builder
LABEL stage=go-builder
WORKDIR /app
COPY calculation_engine/go.mod calculation_engine/go.sum ./
RUN go mod download
RUN go mod verify
COPY calculation_engine/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o /go_app_binary .

# STAGE 2: Rust/C++ Builder for Metrics Generator and Export Engine
FROM rust:1.87.0-slim-bookworm AS rust-cpp-builder
LABEL stage=rust-cpp-builder
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        g++ \
        make \
        curl \
        ca-certificates \
        pkg-config \
        libssl-dev && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app/metrics_generator
COPY server9/Cargo.toml server9/Cargo.lock* ./
RUN mkdir -p src && echo "fn main(){println!(\"Dummy main for cargo fetch\");}" > src/main.rs
RUN cargo fetch
COPY server9/src ./src
RUN cargo build --release

WORKDIR /app/export_engine_build
COPY server9/export.cpp .
RUN g++ -std=c++11 -O2 -Wall -o /app_export_engine export.cpp
# No megacmd install needed in builder stage

# STAGE 3: Final Runtime Image
FROM debian:bookworm-slim AS final

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        supervisor \
        ca-certificates \
        libfuse2 \
        libssl3 \
        dbus \
        curl && \
    echo "Installing MEGAcmd in final stage..." && \
    curl -fsSL https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb -o /tmp/megacmd.deb && \
    apt-get update && \
    apt-get install -y /tmp/megacmd.deb && \
    rm /tmp/megacmd.deb && \
    echo "--- [FINAL STAGE] Creating symlinks for mega commands to /usr/local/bin ---" && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/megals && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/megaget && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/megalogin && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/megaput && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-ipc && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-whoami && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-logout && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-mkdir && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-cd && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-attr && \
    # Add other mega- commands you use if they are wrapper scripts for mega-cmd
    # Ensure mega-cmd-server is also correctly pathed or managed
    # If mega-cmd-server is a separate binary in /usr/bin, ensure it's found or symlinked if needed.
    # For now, assume client tools manage the server correctly if HOME is set.
    echo "--- [FINAL STAGE] Verification after megacmd install and symlinking ---" && \
    echo "--- [FINAL STAGE] which megals (should be /usr/local/bin/megals if PATH is standard): ---" && \
    (which megals || echo "[FINAL INFO] 'megals' not found in PATH by 'which'") && \
    echo "--- [FINAL STAGE] ls -l /usr/local/bin/megals: ---" && \
    (ls -l /usr/local/bin/megals || echo "[FINAL INFO] '/usr/local/bin/megals' does not exist") && \
    echo "--- [FINAL STAGE] ls -l /usr/bin/mega-cmd: ---" && \
    (ls -l /usr/bin/mega-cmd || echo "[FINAL INFO] '/usr/bin/mega-cmd' does not exist") && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*


RUN groupadd --gid 999 appgroup && useradd --uid 999 --gid appgroup --shell /bin/bash --create-home appuser

RUN mkdir -p /app /data/metrics_archive /var/log/supervisor \
    && chown -R appuser:appgroup /app /data \
    && chown -R root:root /var/log/supervisor

WORKDIR /app

COPY --from=go-builder --chown=appuser:appgroup /go_app_binary /app/calculation_service
RUN chmod +x /app/calculation_service

COPY --chown=appuser:appgroup calculation_engine/dependencies /app/dependencies

COPY --from=rust-cpp-builder --chown=appuser:appgroup /app/metrics_generator/target/release/timestamp_generator /app/metrics_generator
RUN chmod +x /app/metrics_generator

COPY --from=rust-cpp-builder --chown=appuser:appgroup /app_export_engine /app/export_engine
RUN chmod +x /app/export_engine

COPY supervisor.conf /etc/supervisor/supervisord.conf

EXPOSE 9000

CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/supervisord.conf", "-n"]