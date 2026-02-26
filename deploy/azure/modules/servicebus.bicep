// Service Bus namespace with translation-jobs queue and KEDA scaler auth.

param prefix string
param location string
param tags object

resource serviceBusNamespace 'Microsoft.ServiceBus/namespaces@2024-01-01' = {
  name: '${prefix}-sb'
  location: location
  tags: tags
  sku: {
    name: 'Standard'
    tier: 'Standard'
  }
}

resource translationJobsQueue 'Microsoft.ServiceBus/namespaces/queues@2024-01-01' = {
  parent: serviceBusNamespace
  name: 'translation-jobs'
  properties: {
    lockDuration: 'PT5M'
    maxDeliveryCount: 3
    deadLetteringOnMessageExpiration: true
    defaultMessageTimeToLive: 'P1D'
    duplicateDetectionHistoryTimeWindow: 'PT10M'
  }
}

// Shared access policy for KEDA scaler (needs Manage for queue metrics).
resource kedaAuthRule 'Microsoft.ServiceBus/namespaces/AuthorizationRules@2024-01-01' = {
  parent: serviceBusNamespace
  name: 'keda-scaler'
  properties: {
    rights: ['Manage', 'Send', 'Listen']
  }
}

output namespaceName string = serviceBusNamespace.name
#disable-next-line outputs-should-not-contain-secrets // consumed by container app modules
output connectionString string = kedaAuthRule.listKeys().primaryConnectionString
output queueName string = translationJobsQueue.name
