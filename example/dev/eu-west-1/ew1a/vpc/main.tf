locals {

  dependencies = {
    apex_zones = "../../../global/shared/apex_zones"
  }

  custom_config = {
    apex_zones       = local.dependency.apex_zones.zone_map
  }

  module_input = merge(local.custom_config, jsondecode(module.hiera.json))

  outputs = [
    "name",
    "vpc_id",
    "azs",
    "dns_zones",
    "private_subnets",
    "public_subnets",
    "database_subnets",
  ]

}
