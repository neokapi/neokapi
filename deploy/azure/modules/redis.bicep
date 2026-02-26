// Azure Cache for Redis with private endpoint and TLS.

param prefix string
param location string
param tags object
param subnetId string
param privateDnsZoneId string
param environment string

var skuName = environment == 'prod' ? 'Standard' : 'Basic'
var capacity = environment == 'prod' ? 1 : 0

resource redisCache 'Microsoft.Cache/redis@2024-11-01' = {
  name: '${prefix}-redis'
  location: location
  tags: tags
  properties: {
    sku: {
      name: skuName
      family: 'C'
      capacity: capacity
    }
    enableNonSslPort: false
    minimumTlsVersion: '1.2'
    publicNetworkAccess: 'Disabled'
  }
}

resource privateEndpoint 'Microsoft.Network/privateEndpoints@2024-05-01' = {
  name: '${prefix}-redis-pe'
  location: location
  tags: tags
  properties: {
    subnet: {
      id: subnetId
    }
    privateLinkServiceConnections: [
      {
        name: '${prefix}-redis-plsc'
        properties: {
          privateLinkServiceId: redisCache.id
          groupIds: ['redisCache']
        }
      }
    ]
  }
}

resource privateDnsZoneGroup 'Microsoft.Network/privateEndpoints/privateDnsZoneGroups@2024-05-01' = {
  parent: privateEndpoint
  name: 'default'
  properties: {
    privateDnsZoneConfigs: [
      {
        name: 'redis'
        properties: {
          privateDnsZoneId: privateDnsZoneId
        }
      }
    ]
  }
}

output hostname string = redisCache.properties.hostName
#disable-next-line outputs-should-not-contain-secrets // consumed by keyvault module
output primaryKey string = redisCache.listKeys().primaryKey
output sslPort int = redisCache.properties.sslPort
