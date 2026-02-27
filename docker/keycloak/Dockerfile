# ── Stage 1: Build custom login theme ────────────────────────────────────────
FROM node:22-alpine AS theme-builder
WORKDIR /src

# Build shared UI package (TypeScript project reference for the theme).
COPY packages/ui/package.json packages/ui/package-lock.json* packages/ui/
RUN cd packages/ui && npm ci
COPY packages/ui/ packages/ui/
RUN cd packages/ui && npx tsc

# Build the Keycloak theme JAR.
COPY bowrain/apps/keycloak-theme/package.json bowrain/apps/keycloak-theme/package-lock.json bowrain/apps/keycloak-theme/
RUN cd bowrain/apps/keycloak-theme && npm ci
COPY bowrain/apps/keycloak-theme/ bowrain/apps/keycloak-theme/
RUN cd bowrain/apps/keycloak-theme && npm run build-keycloak-theme

# ── Stage 2: Keycloak with theme + realm ─────────────────────────────────────
FROM quay.io/keycloak/keycloak:26.1

# Install custom theme JAR.
COPY --from=theme-builder /src/bowrain/apps/keycloak-theme/dist_keycloak/keycloak-theme-for-kc-all-other-versions.jar /opt/keycloak/providers/keycloak-theme.jar

# Seed realm configuration for initial import.
COPY docker/keycloak/realm.json /opt/keycloak/data/import/realm.json

# Entrypoint wrapper that substitutes OIDC_CLIENT_SECRET in realm.json before starting Keycloak.
COPY docker/keycloak/docker-entrypoint.sh /opt/keycloak/docker-entrypoint.sh
RUN chmod +x /opt/keycloak/docker-entrypoint.sh

ENV KC_HEALTH_ENABLED=true

ENTRYPOINT ["/opt/keycloak/docker-entrypoint.sh"]
CMD ["start", "--import-realm"]
