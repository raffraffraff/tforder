locals {

  dependencies = {
    vpc = "../vpc"
  }

  custom_config = {
    cluster_name   = local.dependency.vpc.name
    vpc_id         = local.dependency.vpc.vpc_id
    subnet_ids     = local.dependency.vpc.private_subnets
    region         = local.region
    aws_profile    = local.aws_profile
  }

  module_input = merge(local.custom_config, jsondecode(module.hiera.json))

  outputs = [
    "cluster_oidc_issuer_url",
    "cluster_version",
    "cluster_name",
    "cluster_endpoint"
  ]

}
