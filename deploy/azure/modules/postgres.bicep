// PostgreSQL Flexible Server with PgBouncer, private networking, and Entra ID auth.

param prefix string
param location string
param tags object
param subnetId string
param privateDnsZoneId string
param environment string

@secure()
param administratorLogin string

@secure()
param administratorPassword string

param managedIdentityPrincipalId string

var skuName = environment == 'prod' ? 'Standard_B2ms' : 'Standard_B1ms'
var storageSizeGB = environment == 'prod' ? 128 : 32

resource postgresServer 'Microsoft.DBforPostgreSQL/flexibleServers@2025-08-01' = {
  name: '${prefix}-pg'
  location: location
  tags: tags
  sku: {
    name: skuName
    tier: 'Burstable'
  }
  properties: {
    version: '16'
    administratorLogin: administratorLogin
    administratorLoginPassword: administratorPassword
    storage: {
      storageSizeGB: storageSizeGB
    }
    backup: {
      backupRetentionDays: environment == 'prod' ? 35 : 7
      geoRedundantBackup: environment == 'prod' ? 'Enabled' : 'Disabled'
    }
    highAvailability: {
      mode: 'Disabled'
    }
    network: {
      delegatedSubnetResourceId: subnetId
      privateDnsZoneArmResourceId: privateDnsZoneId
    }
    authConfig: {
      activeDirectoryAuth: 'Enabled'
      passwordAuth: 'Enabled'
    }
  }
}

// Enable PgBouncer connection pooling.
resource pgBouncerConfig 'Microsoft.DBforPostgreSQL/flexibleServers/configurations@2025-08-01' = {
  parent: postgresServer
  name: 'pgbouncer.enabled'
  properties: {
    value: 'true'
    source: 'user-override'
  }
}

// Bowrain application database.
resource bowrainDb 'Microsoft.DBforPostgreSQL/flexibleServers/databases@2025-08-01' = {
  parent: postgresServer
  name: 'bowrain'
  properties: {
    charset: 'UTF8'
    collation: 'en_US.utf8'
  }
}

// Keycloak database.
resource keycloakDb 'Microsoft.DBforPostgreSQL/flexibleServers/databases@2025-08-01' = {
  parent: postgresServer
  name: 'keycloak'
  properties: {
    charset: 'UTF8'
    collation: 'en_US.utf8'
  }
}

// Entra ID admin for managed identity access.
resource entraAdmin 'Microsoft.DBforPostgreSQL/flexibleServers/administrators@2025-08-01' = {
  parent: postgresServer
  name: managedIdentityPrincipalId
  properties: {
    principalType: 'ServicePrincipal'
    principalName: '${prefix}-id'
    tenantId: subscription().tenantId
  }
}

output fqdn string = postgresServer.properties.fullyQualifiedDomainName
output serverId string = postgresServer.id
