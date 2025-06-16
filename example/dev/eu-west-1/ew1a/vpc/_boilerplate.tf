locals {

  # Work out hiera scope based on directory structure
  pathdirs    = split("/", abspath(path.root))
  stack       = element(local.pathdirs, length(local.pathdirs) - 1)
  group       = element(local.pathdirs, length(local.pathdirs) - 2)
  region      = element(local.pathdirs, length(local.pathdirs) - 3)
  aws_account = element(local.pathdirs, length(local.pathdirs) - 4)

  # NOTE: when region is global, we still need a "real" AWS region for our provider (default: eu-west-1)
  aws_profile = "${local.aws_account}.administrator"
  aws_region  = local.region == "global" ? "eu-west-1" : local.region

  # work out dependencies configs
  remote_state_config = { for key, val in local.dependencies :
    key => zipmap(["stack", "group", "region", "aws_account"], slice(reverse(split("/", abspath(val))), 0, 4))
  }

  # create usable dependencies map to iterate over
  dependency = { for key, _ in local.remote_state_config :
    key => jsondecode(nonsensitive(data.aws_ssm_parameter.this[key].value))
  }

  # output only selected keys/vals, or "all" if nothing specific is selected
  output = { for key in coalescelist(local.outputs,keys(module.this.output)):
             key => module.this.output[key] 
  }

}

# Save outputs to SSM Parameter Store, for other modules to use
resource "aws_ssm_parameter" "tf_output_to_ssm" {
  name  = "/tfoutput/${local.group}/${local.stack}"
  type  = "String"
  value = jsonencode(local.output)
}

# Read output of dependency modules from their SSM Parameter Store paths
data "aws_ssm_parameter" "this" {
  for_each = local.remote_state_config
  name     = "/tfoutput/${each.value.group}/${each.value.stack}"
}

# NOTE: Requires OpenTofu, which  supports backend variables (as long as they are known before init)
terraform {
  backend "s3" {
    bucket  = join("-", [local,aws_account, local.region, "my-very-unique-tfstate-bucket"]) # globally unique but not random
    key     = join("-", [local.group, local.stack, "tfstate"])
    profile = local.aws_profile
    region  = local.aws_region
  }
}

# Hiera module performs data lookup and returns results as output
module "hiera" {
  source      = "../../../../../hiera"
  stack       = local.stack
  group       = local.group
  region      = local.region
  aws_account = local.aws_account
}

# Run the module matching "this dir name" (NOTE: Also requires OpenTofu)
module "this" {
  source = "../../../../../modules/${local.stack}"
  config = jsonencode(local.module_input)
}

output "output" {
  value = local.output
}
