// DNS records in existing bowrain.cloud zone for Bowrain custom domains.

param dnsZoneName string
param dnsZoneResourceGroup string
param environment string
param frontDoorFqdn string
param keycloakAppFqdn string

// Prod uses the zone apex (bowrain.cloud); dev uses the 'dev' subdomain.
var apiSubdomain = environment == 'prod' ? '@' : 'dev'
var authSubdomain = environment == 'prod' ? 'auth' : 'auth.dev'

// CNAME: dev.bowrain.cloud → Front Door endpoint FQDN (dev only).
// The prod apex domain (bowrain.cloud) cannot use a CNAME; it requires
// an ALIAS record to Front Door configured outside of Bicep.
module apiCname 'dns-record.bicep' = if (environment != 'prod') {
  name: 'dns-api-cname'
  scope: resourceGroup(dnsZoneResourceGroup)
  params: {
    dnsZoneName: dnsZoneName
    recordName: apiSubdomain
    targetFqdn: frontDoorFqdn
  }
}

// CNAME: auth[.dev].bowrain.cloud → keycloak container app FQDN.
module authCname 'dns-record.bicep' = {
  name: 'dns-auth-cname'
  scope: resourceGroup(dnsZoneResourceGroup)
  params: {
    dnsZoneName: dnsZoneName
    recordName: authSubdomain
    targetFqdn: keycloakAppFqdn
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
