# Sandbox image for @bravo Python code execution.
# Minimal Python runtime with common localization libraries pre-installed.
# The container runs with a read-only root filesystem; /workspace is the
# only writable mount (tmpfs, noexec).
#
# Build: docker build -t ghcr.io/neokapi/bravo-sandbox-python:latest -f platform/docker/bravo-sandbox-python/Dockerfile .

FROM python:3.13-slim

# Install localization-relevant libraries.
RUN pip install --no-cache-dir \
    polib==1.2.0 \
    babel==2.16.0 \
    lxml==5.3.0 \
    pyyaml==6.0.2 \
    toml==0.10.2 \
    && rm -rf /root/.cache

# Non-root user for additional isolation.
RUN adduser --disabled-password --gecos "" --home /workspace sandbox
USER sandbox
WORKDIR /workspace
