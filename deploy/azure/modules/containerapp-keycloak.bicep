// Keycloak Container App for identity and authentication.

param prefix string
param location string
param tags object
param containerAppEnvId string
param managedIdentityId string
param acrLoginServer string
param imageTag string
param environment string
param postgresHost string
param postgresDbName string
param customDomain string

@secure()
param postgresAdminLogin string

@secure()
param postgresAdminPassword string

@secure()
param keycloakAdminPassword string

var minReplicas = environment == 'prod' ? 2 : 1
var maxReplicas = environment == 'prod' ? 2 : 1

resource keycloakApp 'Microsoft.App/containerApps@2025-07-01' = {
  name: '${prefix}-keycloak'
  location: location
  tags: tags
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${managedIdentityId}': {}
    }
  }
  properties: {
    managedEnvironmentId: containerAppEnvId
    configuration: {
      activeRevisionsMode: 'Single'
      registries: [
        {
          server: acrLoginServer
          identity: managedIdentityId
        }
      ]
      ingress: {
        external: true
        targetPort: 8080
        transport: 'http'
        customDomains: [
          {
            name: customDomain
            bindingType: 'SniEnabled'
            certificateId: '${containerAppEnvId}/managedCertificates/${prefix}-kc-cert'
          }
        ]
      }
      secrets: [
        {
          name: 'kc-db-password'
          value: postgresAdminPassword
        }
        {
          name: 'kc-admin-password'
          value: keycloakAdminPassword
        }
      ]
    }
    template: {
      containers: [
        {
          name: 'keycloak'
          image: '${acrLoginServer}/bowrain-keycloak:${imageTag}'
          command: ['start']
          resources: {
            cpu: json('0.5')
            memory: '1Gi'
          }
          env: [
            { name: 'KC_DB', value: 'postgres' }
            { name: 'KC_DB_URL', value: 'jdbc:postgresql://${postgresHost}:5432/${postgresDbName}?sslmode=require' }
            { name: 'KC_DB_USERNAME', value: postgresAdminLogin }
            { name: 'KC_DB_PASSWORD', secretRef: 'kc-db-password' }
            { name: 'KC_HOSTNAME', value: customDomain }
            { name: 'KC_PROXY_HEADERS', value: 'xforwarded' }
            { name: 'KC_HTTP_ENABLED', value: 'true' }
            { name: 'KC_HEALTH_ENABLED', value: 'true' }
            { name: 'KEYCLOAK_ADMIN', value: 'admin' }
            { name: 'KEYCLOAK_ADMIN_PASSWORD', secretRef: 'kc-admin-password' }
          ]
          probes: [
            {
              type: 'Liveness'
              httpGet: {
                path: '/health/live'
                port: 8080
              }
              periodSeconds: 10
            }
            {
              type: 'Readiness'
              httpGet: {
                path: '/health/ready'
                port: 8080
              }
              initialDelaySeconds: 30
              periodSeconds: 10
            }
          ]
        }
      ]
      scale: {
        minReplicas: minReplicas
        maxReplicas: maxReplicas
      }
    }
  }
}

output fqdn string = keycloakApp.properties.configuration.ingress.fqdn
output name string = keycloakApp.name
