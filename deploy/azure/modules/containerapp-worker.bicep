// Bowrain Worker Container App with KEDA Service Bus scaler.

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

var minReplicas = environment == 'prod' ? 1 : 0
var maxReplicas = environment == 'prod' ? 20 : 5

resource workerApp 'Microsoft.App/containerApps@2025-07-01' = {
  name: '${prefix}-worker'
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
      secrets: [
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
          name: 'bowrain-worker'
          image: '${acrLoginServer}/bowrain-worker:${imageTag}'
          resources: {
            cpu: json('1')
            memory: '2Gi'
          }
          env: [
            { name: 'BOWRAIN_DATABASE_URL', value: 'host=${postgresHost} port=5432 dbname=${postgresDbName} sslmode=require' }
            { name: 'BOWRAIN_REDIS_URL', value: 'rediss://${redisHost}:6380' }
            { name: 'BOWRAIN_REDIS_PASSWORD', secretRef: 'redis-access-key' }
            { name: 'BOWRAIN_SERVICE_BUS_CONNECTION', secretRef: 'servicebus-connection' }
          ]
        }
      ]
      scale: {
        minReplicas: minReplicas
        maxReplicas: maxReplicas
        rules: [
          {
            name: 'servicebus-queue-depth'
            custom: {
              type: 'azure-servicebus'
              metadata: {
                queueName: 'translation-jobs'
                messageCount: '5'
              }
              auth: [
                {
                  secretRef: 'servicebus-connection'
                  triggerParameter: 'connection'
                }
              ]
            }
          }
        ]
      }
    }
  }
}

output name string = workerApp.name
