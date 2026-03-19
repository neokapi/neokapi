# ── Build stage ──────────────────────────────────────────────────
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm install
COPY tsconfig.json ./
COPY src/ src/
RUN npm run build

# ── Run stage ────────────────────────────────────────────────────
FROM node:22-alpine
WORKDIR /app
COPY --from=build /app/package.json /app/package-lock.json* ./
RUN npm install --omit=dev
COPY --from=build /app/dist/ dist/
EXPOSE 3001
CMD ["node", "dist/index.js"]
