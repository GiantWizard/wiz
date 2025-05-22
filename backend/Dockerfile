# Use Koyeb’s Compose-enabled base image
FROM koyeb/docker-compose:latest

# Set working directory
WORKDIR /app

# Copy everything (including your docker-compose.yml)
COPY . /app

# (Optional) If you need to pass build args or env-vars at build time, you can add:
# ARG MEGA_EMAIL
# ARG MEGA_PWD
# ARG MEGA_METRICS_FOLDER_PATH
# ENV MEGA_EMAIL=${MEGA_EMAIL} \
#     MEGA_PWD=${MEGA_PWD} \
#     MEGA_METRICS_FOLDER_PATH=${MEGA_METRICS_FOLDER_PATH}

# The base image’s entrypoint will auto-detect and run:
#    docker-compose up --no-deps --build
# pointing at the ./docker-compose.yml in /app
