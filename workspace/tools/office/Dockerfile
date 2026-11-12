FROM node:22-bookworm-slim AS base
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-venv libreoffice-writer libreoffice-calc libreoffice-impress \
    pandoc poppler-utils fonts-noto-cjk fonts-liberation fontconfig \
    zip unzip gcc libc6-dev \
    && rm -rf /var/lib/apt/lists/*
RUN python3 -m venv /opt/office/venv
ENV PATH="/opt/office/venv/bin:${PATH}" \
    NODE_PATH=/opt/office/node_modules \
    PYTHONDONTWRITEBYTECODE=1 \
    LANG=C.UTF-8
FROM base AS runtime
WORKDIR /opt/office
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt && pip check
COPY package.json package-lock.json ./
RUN npm ci --omit=dev --no-audit --no-fund && npm cache clean --force
COPY entrypoint.sh /opt/office/entrypoint.sh
ENTRYPOINT ["/bin/bash", "/opt/office/entrypoint.sh"]
