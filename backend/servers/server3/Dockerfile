# ===== Builder Stage =====
FROM rust:1.85-bullseye-slim as builder

# Install build prerequisites.
RUN apt-get update && apt-get install -y wget gnupg

WORKDIR /app
# Copy the entire project.
COPY . .

# Build the Rust application.
RUN cargo build --release

# ===== Runtime Stage =====
FROM debian:bullseye-slim

# Install runtime prerequisites.
RUN apt-get update && apt-get install -y wget

WORKDIR /app

# Download and install MEGAcmd from its Debian package.
RUN wget -O megacmd.deb "https://github.com/meganz/MEGAcmd/releases/download/v1.5.0/megacmd-Debian10_x86_64.deb" && \
    apt-get update && \
    apt-get install -y ./megacmd.deb && \
    rm megacmd.deb

# Copy the compiled binary and any required assets.
COPY --from=builder /app/target/release/server3 .
COPY --from=builder /app/exports ./exports

EXPOSE 3000

CMD ["./server3"]
