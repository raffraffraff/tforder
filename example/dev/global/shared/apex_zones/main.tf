locals {

  dependencies = {
  }

  custom_config = {
  }

  module_input = merge(local.custom_config, jsondecode(module.hiera.json))

  outputs = [
    "zone_map"
  ]

}
