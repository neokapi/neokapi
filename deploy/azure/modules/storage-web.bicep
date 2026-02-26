// Azure Storage Account for static web hosting (SPA).
// Static website is enabled via a deployment script because Bicep
// does not expose the staticWebsite blob-service property natively.

param prefix string
param location string
param tags object
param managedIdentityId string
param managedIdentityPrincipalId string

// Storage account names must be alphanumeric, 3–24 chars.
var storageAccountName = replace('${prefix}web', '-', '')

resource storageAccount 'Microsoft.Storage/storageAccounts@2024-01-01' = {
  name: storageAccountName
  location: location
  tags: tags
  kind: 'StorageV2'
  sku: {
    name: 'Standard_LRS'
  }
  properties: {
    allowBlobPublicAccess: true
    minimumTlsVersion: 'TLS1_2'
    supportsHttpsTrafficOnly: true
  }
}

// Enable static website hosting via AzureCLI deployment script.
// Bicep does not support the staticWebsite property on blob services.
resource enableStaticWebsite 'Microsoft.Resources/deploymentScripts@2023-08-01' = {
  name: '${prefix}-enable-static-website'
  location: location
  tags: tags
  kind: 'AzureCLI'
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${managedIdentityId}': {}
    }
  }
  properties: {
    azCliVersion: '2.63.0'
    retentionInterval: 'PT1H'
    scriptContent: 'az storage blob service-properties update --account-name ${storageAccount.name} --static-website --index-document index.html --404-document index.html --auth-mode login'
  }
}

// Storage Blob Data Contributor for managed identity (CI uploads).
var storageBlobDataContributorRole = 'ba92f5b4-2d11-453d-a403-e96b0029c9fe'

resource storageBlobRoleAssignment 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: guid(storageAccount.id, managedIdentityPrincipalId, storageBlobDataContributorRole)
  scope: storageAccount
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', storageBlobDataContributorRole)
    principalId: managedIdentityPrincipalId
    principalType: 'ServicePrincipal'
  }
}

output storageAccountName string = storageAccount.name
output webEndpoint string = storageAccount.properties.primaryEndpoints.web
output webHostname string = replace(replace(storageAccount.properties.primaryEndpoints.web, 'https://', ''), '/', '')
