FROM rust:1.85-slim as builder
WORKDIR /app

# Install dependencies for the mega crate
RUN apt-get update && \
    apt-get install -y pkg-config libssl-dev && \
    rm -rf /var/lib/apt/lists/*

# Copy source code and build
COPY . .
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && \
    apt-get install -y ca-certificates libssl3 && \
    rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/target/release/mega_exporter /app/mega_exporter

# Create a directory for local files
RUN mkdir -p /app/exports

CMD ["/app/mega_exporter"]