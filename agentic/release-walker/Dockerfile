# Stage 1: Build TypeScript
FROM node:22-alpine AS builder

WORKDIR /build
COPY package.json package-lock.json* ./
RUN npm install
COPY tsconfig.json ./
COPY src/ ./src/
RUN npm run build

# Stage 2: Production image
FROM node:22-alpine

# Install git and gh CLI (needed for merge operations and GitHub API fallback).
RUN apk add --no-cache git github-cli

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm install --omit=dev

COPY --from=builder /build/dist/ ./dist/

# Config and forks are mounted as volumes.
VOLUME ["/app/config", "/app/forks"]

ENTRYPOINT ["node", "dist/index.js"]
