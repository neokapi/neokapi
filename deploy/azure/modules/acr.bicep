// Azure Container Registry with AcrPull role for managed identity.

param prefix string
param location string
param tags object
param principalId string

// ACR names must be alphanumeric.
var acrName = replace('${prefix}acr', '-', '')

resource acr 'Microsoft.ContainerRegistry/registries@2025-11-01' = {
  name: acrName
  location: location
  tags: tags
  sku: {
    name: 'Basic'
  }
  properties: {
    adminUserEnabled: false
  }
}

// AcrPull role assignment for managed identity.
var acrPullRole = '7f951dda-4ed3-4680-a7ca-43fe172d538d'

resource acrRoleAssignment 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: guid(acr.id, principalId, acrPullRole)
  scope: acr
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', acrPullRole)
    principalId: principalId
    principalType: 'ServicePrincipal'
  }
}

output loginServer string = acr.properties.loginServer
output name string = acr.name
output id string = acr.id
