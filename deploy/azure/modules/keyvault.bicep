// Key Vault for secrets management with RBAC access.

param prefix string
param location string
param tags object
param principalId string

@secure()
param jwtSecret string

@secure()
param oidcClientSecret string

@secure()
param redisAccessKey string

resource keyVault 'Microsoft.KeyVault/vaults@2025-05-01' = {
  name: '${prefix}-kv'
  location: location
  tags: tags
  properties: {
    sku: {
      family: 'A'
      name: 'standard'
    }
    tenantId: subscription().tenantId
    enableRbacAuthorization: true
    enableSoftDelete: true
    softDeleteRetentionInDays: 7
  }
}

resource jwtSecretEntry 'Microsoft.KeyVault/vaults/secrets@2025-05-01' = {
  parent: keyVault
  name: 'jwt-secret'
  properties: {
    value: jwtSecret
  }
}

resource oidcSecretEntry 'Microsoft.KeyVault/vaults/secrets@2025-05-01' = {
  parent: keyVault
  name: 'oidc-client-secret'
  properties: {
    value: oidcClientSecret
  }
}

resource redisKeyEntry 'Microsoft.KeyVault/vaults/secrets@2025-05-01' = {
  parent: keyVault
  name: 'redis-access-key'
  properties: {
    value: redisAccessKey
  }
}

// Key Vault Secrets User role assignment for managed identity.
var keyVaultSecretsUserRole = '4633458b-17de-408a-b874-0445c86b69e6'

resource kvRoleAssignment 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: guid(keyVault.id, principalId, keyVaultSecretsUserRole)
  scope: keyVault
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', keyVaultSecretsUserRole)
    principalId: principalId
    principalType: 'ServicePrincipal'
  }
}

output uri string = keyVault.properties.vaultUri
output name string = keyVault.name
