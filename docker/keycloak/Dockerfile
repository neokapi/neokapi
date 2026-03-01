# Keycloak with custom login theme + realm.
#
# The theme JAR is pre-built on the CI runner (keycloakify requires Maven)
# and passed in via the build context.
#
# Build: docker build -f docker/keycloak/Dockerfile .

FROM quay.io/keycloak/keycloak:26.1

# Install custom theme JAR (pre-built by `make keycloak-theme`).
COPY bowrain/apps/keycloak-theme/dist_keycloak/keycloak-theme-for-kc-all-other-versions.jar /opt/keycloak/providers/keycloak-theme.jar

# Register providers (theme JAR) so `start` (production mode) works.
RUN /opt/keycloak/bin/kc.sh build

# Seed realm configuration for initial import (writable for entrypoint sed).
COPY --chown=1000:0 docker/keycloak/realm.json /opt/keycloak/data/import/realm.json

# Entrypoint wrapper that substitutes OIDC_CLIENT_SECRET in realm.json before starting Keycloak.
COPY --chmod=0755 docker/keycloak/docker-entrypoint.sh /opt/keycloak/docker-entrypoint.sh

ENV KC_HEALTH_ENABLED=true

ENTRYPOINT ["/opt/keycloak/docker-entrypoint.sh"]
CMD ["start", "--import-realm"]
