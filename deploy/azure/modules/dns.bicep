// DNS records in existing samarb.ai zone for Bowrain custom domains.

param dnsZoneName string
param dnsZoneResourceGroup string
param environment string
param apiAppFqdn string
param keycloakAppFqdn string

var apiSubdomain = environment == 'prod' ? 'bowrain' : 'bowrain-dev'
var authSubdomain = environment == 'prod' ? 'auth.bowrain' : 'auth.bowrain-dev'

// Reference the existing DNS zone in its resource group.
resource dnsZone 'Microsoft.Network/dnsZones@2023-07-01-preview' existing = {
  name: dnsZoneName
  scope: resourceGroup(dnsZoneResourceGroup)
}

// CNAME: bowrain.samarb.ai → bowrain-api container app FQDN.
module apiCname 'dns-record.bicep' = {
  name: 'dns-api-cname'
  scope: resourceGroup(dnsZoneResourceGroup)
  params: {
    dnsZoneName: dnsZoneName
    recordName: apiSubdomain
    targetFqdn: apiAppFqdn
  }
}

// CNAME: auth.bowrain.samarb.ai → keycloak container app FQDN.
module authCname 'dns-record.bicep' = {
  name: 'dns-auth-cname'
  scope: resourceGroup(dnsZoneResourceGroup)
  params: {
    dnsZoneName: dnsZoneName
    recordName: authSubdomain
    targetFqdn: keycloakAppFqdn
  }
}

// TXT validation record for API custom domain.
module apiTxt 'dns-txt-record.bicep' = {
  name: 'dns-api-txt'
  scope: resourceGroup(dnsZoneResourceGroup)
  params: {
    dnsZoneName: dnsZoneName
    recordName: 'asuid.${apiSubdomain}'
    txtValue: apiAppFqdn
  }
}

// TXT validation record for auth custom domain.
module authTxt 'dns-txt-record.bicep' = {
  name: 'dns-auth-txt'
  scope: resourceGroup(dnsZoneResourceGroup)
  params: {
    dnsZoneName: dnsZoneName
    recordName: 'asuid.${authSubdomain}'
    txtValue: keycloakAppFqdn
  }
}
