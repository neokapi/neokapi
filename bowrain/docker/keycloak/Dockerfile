# Keycloak with custom login theme + realm.
#
# The theme JAR is pre-built on the CI runner (keycloakify requires Maven)
# and passed in via the build context.
#
# Build: docker build -f bowrain/docker/keycloak/Dockerfile .

FROM quay.io/keycloak/keycloak:26.6.3

# Install custom theme JAR (pre-built by `make keycloak-theme`).
COPY bowrain/apps/keycloak-theme/dist_keycloak/keycloak-theme-for-kc-all-other-versions.jar /opt/keycloak/providers/keycloak-theme.jar

# Install Apple identity provider extension (Sign in with Apple).
# https://github.com/klausbetz/apple-identity-provider-keycloak
ADD --chmod=644 --checksum=sha256:4091dee2a1ec9e0771bef4bd46005197d86b0a2b1f25198c41738476b1d102bb https://github.com/klausbetz/apple-identity-provider-keycloak/releases/download/1.17.0/apple-identity-provider-1.17.0.jar /opt/keycloak/providers/apple-identity-provider.jar

# Pre-build with runtime flags so Keycloak skips the Quarkus rebuild at startup.
RUN /opt/keycloak/bin/kc.sh build --health-enabled=true --db=postgres

# Seed realm configuration for initial import (writable for entrypoint sed).
COPY --chown=1000:0 bowrain/docker/keycloak/realm.json /opt/keycloak/data/import/realm.json

# Entrypoint wrapper that substitutes OIDC_CLIENT_SECRET in realm.json before starting Keycloak.
COPY --chmod=0755 bowrain/docker/keycloak/docker-entrypoint.sh /opt/keycloak/docker-entrypoint.sh

ENV KC_HEALTH_ENABLED=true

ENTRYPOINT ["/opt/keycloak/docker-entrypoint.sh"]
CMD ["start", "--optimized", "--import-realm"]
