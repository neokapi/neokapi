// Bowrain API Container App with HTTP scaling, managed TLS, and Key Vault secrets.

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
param redisHost string
param serviceBusConnectionString string
param keyVaultUri string
param keycloakIssuerUrl string
param customDomain string

var minReplicas = environment == 'prod' ? 2 : 1
var maxReplicas = environment == 'prod' ? 10 : 3

resource apiApp 'Microsoft.App/containerApps@2025-07-01' = {
  name: '${prefix}-api'
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
      ingress: {
        external: true
        targetPort: 8080
        transport: 'http'
        corsPolicy: {
          allowedOrigins: ['https://${customDomain}']
          allowedMethods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS']
          allowedHeaders: ['*']
          allowCredentials: true
        }
        customDomains: [
          {
            name: customDomain
            bindingType: 'SniEnabled'
            certificateId: '${containerAppEnvId}/managedCertificates/${prefix}-api-cert'
          }
        ]
      }
      registries: [
        {
          server: acrLoginServer
          identity: managedIdentityId
        }
      ]
      secrets: [
        {
          name: 'jwt-secret'
          keyVaultUrl: '${keyVaultUri}secrets/jwt-secret'
          identity: managedIdentityId
        }
        {
          name: 'oidc-client-secret'
          keyVaultUrl: '${keyVaultUri}secrets/oidc-client-secret'
          identity: managedIdentityId
        }
        {
          name: 'redis-access-key'
          keyVaultUrl: '${keyVaultUri}secrets/redis-access-key'
          identity: managedIdentityId
        }
        {
          name: 'servicebus-connection'
          value: serviceBusConnectionString
        }
      ]
    }
    template: {
      containers: [
        {
          name: 'bowrain-api'
          image: '${acrLoginServer}/bowrain-server:${imageTag}'
          resources: {
            cpu: json('0.5')
            memory: '1Gi'
          }
          env: [
            { name: 'BOWRAIN_MODE', value: 'api' }
            { name: 'BOWRAIN_HOST', value: '0.0.0.0' }
            { name: 'BOWRAIN_PORT', value: '8080' }
            { name: 'BOWRAIN_DATABASE_URL', value: 'host=${postgresHost} port=5432 dbname=${postgresDbName} sslmode=require' }
            { name: 'BOWRAIN_REDIS_URL', value: 'rediss://${redisHost}:6380' }
            { name: 'BOWRAIN_OIDC_ISSUER_URL', value: keycloakIssuerUrl }
            { name: 'BOWRAIN_OIDC_CLIENT_ID', value: 'bowrain' }
            { name: 'BOWRAIN_JWT_SECRET', secretRef: 'jwt-secret' }
            { name: 'BOWRAIN_OIDC_CLIENT_SECRET', secretRef: 'oidc-client-secret' }
            { name: 'BOWRAIN_REDIS_PASSWORD', secretRef: 'redis-access-key' }
            { name: 'BOWRAIN_SERVICE_BUS_CONNECTION', secretRef: 'servicebus-connection' }
          ]
          probes: [
            {
              type: 'Liveness'
              httpGet: {
                path: '/health'
                port: 8080
              }
              periodSeconds: 10
            }
            {
              type: 'Readiness'
              httpGet: {
                path: '/health'
                port: 8080
              }
              initialDelaySeconds: 5
              periodSeconds: 5
            }
          ]
        }
      ]
      scale: {
        minReplicas: minReplicas
        maxReplicas: maxReplicas
        rules: [
          {
            name: 'http-scaling'
            http: {
              metadata: {
                concurrentRequests: '50'
              }
            }
          }
        ]
      }
    }
  }
}

output fqdn string = apiApp.properties.configuration.ingress.fqdn
output name string = apiApp.name
